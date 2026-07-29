// Command repoguard detects repository content that is unsafe to publish.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tasuku43/agentic-cli-foundry/tools/internal/projectconfig"
)

type issue struct {
	Path    string
	Line    int
	Message string
}

var (
	bootstrapPlaceholder  = regexp.MustCompile(`__CLI_[A-Z0-9_]+__|TODO_TEMPLATE|CHANGEME|<your[-_ ][^>]+>`)
	japaneseText          = regexp.MustCompile(`[\x{3040}-\x{30ff}\x{3400}-\x{9fff}]`)
	absoluteHome          = regexp.MustCompile(`(?:/Users/[^/\s]+|/home/[^/\s]+|[A-Za-z]:\\Users\\[^\\\s]+)`)
	privateNetwork        = regexp.MustCompile(`(?i)https?://(?:[^/]*\.(?:internal|corp|local)|10\.[0-9.]+|192\.168\.[0-9.]+|172\.(?:1[6-9]|2[0-9]|3[01])\.[0-9.]+)`)
	formulaPlaceholder    = regexp.MustCompile(`@@([A-Z0-9_]+)@@`)
	inlineMarkdownLink    = regexp.MustCompile(`!?\[[^]\n]*\]\(([^)\n]*)\)`)
	referenceMarkdownLink = regexp.MustCompile(`^\s*\[[^]\n]+\]:\s*(\S+)`)
	workChecklistItem     = regexp.MustCompile(`^\s*(?:[-+*]|[0-9]{1,9}[.)])\s+\[([ xX])\]\s+`)
	uriScheme             = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	authorizationSecret   = regexp.MustCompile(`(?i)authorization\s*:\s*(?:bearer|basic)\s+([A-Za-z0-9+/=_-]{8,})`)
	assignmentSecret      = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])["']?(?:api[_-]?key|client[_-]?secret|password|passwd|access[_-]?token|refresh[_-]?token|private[_-]?key)["']?\s*[:=]\s*(?:"([^"\r\n]*)"|'([^'\r\n]*)'|([^# ,}\]\t\r\n]+))`)
	exampleSecret         = regexp.MustCompile(`^(?:example|dummy|fake|test|redacted|placeholder)(?:[-_.][a-z0-9][a-z0-9._-]*)?$`)
	environmentSecret     = regexp.MustCompile(`^(?:\$\{[A-Z][A-Z0-9_]*\}|env\.[A-Z][A-Z0-9_]*)$`)
	secretPatterns        = []struct {
		name string
		re   *regexp.Regexp
	}{
		{"private key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`)},
		{"AWS access key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
		{"GitHub token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`)},
		{"Slack token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{20,}`)},
		{"Google API key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
		{"credential-bearing URL", regexp.MustCompile(`(?i)https?://[^/@\s:]+:[^/@\s]+@`)},
	}
	allowedFormulaPlaceholders = map[string]bool{
		"FORMULA_CLASS": true, "DESCRIPTION": true, "REPOSITORY_URL": true,
		"VERSION": true, "MACOS_ARM64_URL": true, "MACOS_AMD64_URL": true,
		"MACOS_ARM64_SHA256": true, "MACOS_AMD64_SHA256": true, "BINARY_NAME": true,
		"LICENSE_SPDX": true,
	}
)

func main() {
	scope := flag.String("scope", "hygiene", "hygiene, security, or public")
	rootFlag := flag.String("root", ".", "repository root")
	flag.Parse()
	if *scope != "hygiene" && *scope != "security" && *scope != "public" {
		fmt.Fprintf(os.Stderr, "repoguard: invalid scope %q\n", *scope)
		os.Exit(2)
	}
	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		fatal(err)
	}
	issues, err := inspect(root, *scope)
	if err != nil {
		fatal(err)
	}
	if len(issues) != 0 {
		for _, item := range issues {
			position := item.Path
			if item.Line > 0 {
				position = fmt.Sprintf("%s:%d", position, item.Line)
			}
			fmt.Fprintf(os.Stderr, "%s: %s\n", position, item.Message)
		}
		os.Exit(1)
	}
	fmt.Printf("repoguard (%s): OK\n", *scope)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "repoguard: %v\n", err)
	os.Exit(1)
}

func inspect(root, scope string) ([]issue, error) {
	paths, err := repositoryPaths(root)
	if err != nil {
		return nil, err
	}
	if err := validateRepositoryPaths(root, paths); err != nil {
		return nil, err
	}
	config, err := projectconfig.Load(root)
	if err != nil {
		return nil, err
	}
	denylist, err := readDenylist(root, config.PublicGuard.DenylistFile)
	if err != nil {
		return nil, err
	}
	var issues []issue
	shapeIssues, err := checkFilesystemShape(root, config)
	if err != nil {
		return nil, err
	}
	issues = append(issues, shapeIssues...)
	issues = append(issues, checkRequired(root, config, scope)...)
	issues = append(issues, checkLicense(root, config, scope)...)
	issues = append(issues, checkAgentHarness(root)...)
	workIssues, err := checkWorkPackets(root, paths)
	if err != nil {
		return nil, err
	}
	issues = append(issues, workIssues...)
	historicalLocaleExemptions, err := historicalWorkPacketLocaleExemptions(root, paths)
	if err != nil {
		return nil, err
	}
	linkIssues, err := checkMarkdownLinks(root, paths)
	if err != nil {
		return nil, err
	}
	issues = append(issues, linkIssues...)
	for _, relative := range paths {
		issues = append(issues, checkPath(relative)...)
		if config.Profile == "ready" && relative != "tools/internal/projectconfig/defaults.go" {
			if identity := remainingTemplateIdentity(relative, config.Project); identity != "" {
				issues = append(issues, issue{Path: relative, Message: fmt.Sprintf("template identity %q remains in path after bootstrap", identity)})
			}
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative))) // #nosec G304 -- git and fallback paths are validated as local repository paths.
		if err != nil {
			return nil, err
		}
		if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
			continue
		}
		issues = append(issues, checkTextWithLocaleExemption(
			relative,
			string(data),
			config,
			denylist,
			scope,
			workPacketLocaleExempt(relative, historicalLocaleExemptions),
		)...)
	}
	if scope == "public" && config.Profile == "ready" {
		for _, problem := range projectconfig.ReadyProblems(config.Project) {
			issues = append(issues, issue{Path: ".harness/project.json", Message: problem})
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Line != issues[j].Line {
			return issues[i].Line < issues[j].Line
		}
		return issues[i].Message < issues[j].Message
	})
	return issues, nil
}

type workMetadataValue struct {
	Value string
	Line  int
}

var acceptedWorkStatuses = map[string]bool{
	"Draft":      true,
	"Accepted":   true,
	"Active":     true,
	"Complete":   true,
	"Superseded": true,
}

func checkWorkPackets(root string, repositoryPaths []string) ([]issue, error) {
	available := make(map[string]bool, len(repositoryPaths))
	var goals []string
	for _, relative := range repositoryPaths {
		available[relative] = true
		parts := strings.Split(relative, "/")
		if len(parts) == 4 && parts[0] == "docs" && parts[1] == "work" && parts[3] == "goal.md" {
			goals = append(goals, relative)
		}
	}
	sort.Strings(goals)

	var issues []issue
	successorEdges := make(map[string]string)
	for _, goalPath := range goals {
		data, err := readRegularRepositoryFile(root, goalPath)
		if err != nil {
			return nil, err
		}
		text := string(data)
		statuses := workMetadata(text, "Status")
		if len(statuses) != 1 {
			issues = append(issues, issue{Path: goalPath, Message: "work goal must declare exactly one Status metadata field"})
			continue
		}
		status := statuses[0]
		if !acceptedWorkStatuses[status.Value] {
			issues = append(issues, issue{Path: goalPath, Line: status.Line, Message: "work goal Status must be one of Draft, Accepted, Active, Complete, or Superseded"})
			continue
		}

		successors := workMetadata(text, "Successor")
		if len(successors) > 1 {
			issues = append(issues, issue{Path: goalPath, Message: "work goal must not declare Successor more than once"})
		}
		if status.Value == "Complete" {
			issues = append(issues, checkCompletedWorkPacket(root, goalPath, text, available)...)
		}
		if status.Value == "Superseded" {
			target, successorIssues := checkSupersededWorkPacket(root, goalPath, successors, available)
			issues = append(issues, successorIssues...)
			if len(successorIssues) == 0 {
				successorEdges[goalPath] = target
			}
		}
	}
	issues = append(issues, checkWorkSuccessorCycles(successorEdges)...)
	return issues, nil
}

// historicalWorkPacketLocaleExemptions returns only packet roots whose goal
// explicitly declares a terminal historical status. Malformed, missing,
// Draft, Accepted, and Active metadata fail closed and receive no exemption.
func historicalWorkPacketLocaleExemptions(root string, repositoryPaths []string) (map[string]bool, error) {
	exemptions := make(map[string]bool)
	for _, relative := range repositoryPaths {
		parts := strings.Split(relative, "/")
		if len(parts) != 4 || parts[0] != "docs" || parts[1] != "work" || parts[2] == "_template" || parts[3] != "goal.md" {
			continue
		}
		data, err := readRegularRepositoryFile(root, relative)
		if err != nil {
			return nil, err
		}
		statuses := workMetadata(string(data), "Status")
		if len(statuses) == 1 && (statuses[0].Value == "Complete" || statuses[0].Value == "Superseded") {
			exemptions[pathpkg.Dir(relative)] = true
		}
	}
	return exemptions, nil
}

func workPacketLocaleExempt(relative string, exemptions map[string]bool) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) < 4 || parts[0] != "docs" || parts[1] != "work" {
		return false
	}
	return exemptions[pathpkg.Join(parts[0], parts[1], parts[2])]
}

func checkCompletedWorkPacket(root, goalPath, goalText string, available map[string]bool) []issue {
	var issues []issue
	found, total, unchecked := workChecklist(goalText, "## Acceptance criteria")
	if !found || total == 0 {
		issues = append(issues, issue{Path: goalPath, Message: "Complete work goal must contain acceptance criteria checkboxes"})
	} else {
		for _, line := range unchecked {
			issues = append(issues, issue{Path: goalPath, Line: line, Message: "Complete work goal has an unchecked acceptance criterion"})
		}
	}

	tasksPath := pathpkg.Join(pathpkg.Dir(goalPath), "tasks.md")
	if !available[tasksPath] {
		return append(issues, issue{Path: tasksPath, Message: "Complete work packet must include tasks.md"})
	}
	data, err := readRegularRepositoryFile(root, tasksPath)
	if err != nil {
		return append(issues, issue{Path: tasksPath, Message: err.Error()})
	}
	_, total, unchecked = workChecklist(string(data), "")
	if total == 0 {
		issues = append(issues, issue{Path: tasksPath, Message: "Complete work packet tasks.md must contain task checkboxes"})
	}
	for _, line := range unchecked {
		issues = append(issues, issue{Path: tasksPath, Line: line, Message: "Complete work packet has an unchecked task"})
	}
	return issues
}

func checkSupersededWorkPacket(root, goalPath string, successors []workMetadataValue, available map[string]bool) (string, []issue) {
	if len(successors) != 1 || strings.EqualFold(successors[0].Value, "none") || successors[0].Value == "" {
		return "", []issue{{Path: goalPath, Message: "Superseded work goal must declare one explicit Successor path"}}
	}
	metadata := successors[0]
	raw := workTrimASCIIHorizontal(metadata.Value)
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\\?#") || strings.Contains(raw, "](") || strings.HasPrefix(raw, "/") || uriScheme.MatchString(raw) || pathpkg.Clean(raw) != raw {
		return "", []issue{{Path: goalPath, Line: metadata.Line, Message: "Superseded work goal Successor must be one canonical raw relative path"}}
	}
	resolved := pathpkg.Clean(pathpkg.Join(pathpkg.Dir(goalPath), raw))
	parts := strings.Split(resolved, "/")
	if resolved == goalPath || len(parts) != 4 || parts[0] != "docs" || parts[1] != "work" || parts[2] == "_template" || parts[3] != "goal.md" {
		return "", []issue{{Path: goalPath, Line: metadata.Line, Message: "Superseded work goal Successor must target another non-template docs/work/<name>/goal.md"}}
	}
	if !available[resolved] {
		return "", []issue{{Path: goalPath, Line: metadata.Line, Message: fmt.Sprintf("Superseded work goal Successor %q is not a repository work goal", resolved)}}
	}
	if _, err := readRegularRepositoryFile(root, resolved); err != nil {
		return "", []issue{{Path: goalPath, Line: metadata.Line, Message: fmt.Sprintf("Superseded work goal Successor is not a regular repository file: %v", err)}}
	}
	return resolved, nil
}

func checkWorkSuccessorCycles(edges map[string]string) []issue {
	starts := make([]string, 0, len(edges))
	for start := range edges {
		starts = append(starts, start)
	}
	sort.Strings(starts)
	reported := make(map[string]bool)
	var issues []issue
	for _, start := range starts {
		positions := make(map[string]int)
		var chain []string
		current := start
		for {
			if position, exists := positions[current]; exists {
				cycle := append([]string(nil), chain[position:]...)
				sortedCycle := append([]string(nil), cycle...)
				sort.Strings(sortedCycle)
				key := strings.Join(sortedCycle, "\x00")
				if !reported[key] {
					reported[key] = true
					issues = append(issues, issue{
						Path:    sortedCycle[0],
						Message: "Superseded work goal Successor chain contains a cycle: " + strings.Join(cycle, " -> ") + " -> " + current,
					})
				}
				break
			}
			positions[current] = len(chain)
			chain = append(chain, current)
			next, exists := edges[current]
			if !exists {
				break
			}
			current = next
		}
	}
	return issues
}

func workMetadata(text, name string) []workMetadataValue {
	scanner := workMarkdownScanner{}
	foundH1 := false
	inBlock := false
	var values []workMetadataValue
	for index, raw := range strings.Split(text, "\n") {
		line, visible := scanner.visible(raw)
		if !visible {
			if foundH1 {
				break
			}
			continue
		}
		if !foundH1 {
			if workH1(line) {
				foundH1 = true
			}
			continue
		}
		if !inBlock && workASCIIBlankLine(line) {
			continue
		}
		key, value, metadata := workMetadataLine(line)
		if !metadata {
			break
		}
		inBlock = true
		if key == name {
			values = append(values, workMetadataValue{Value: value, Line: index + 1})
		}
	}
	return values
}

func workH1(line string) bool {
	rest, ok := workFenceIndent(line)
	return ok && (rest == "#" || strings.HasPrefix(rest, "# "))
}

func workMetadataLine(line string) (string, string, bool) {
	if !strings.HasPrefix(line, "- ") {
		return "", "", false
	}
	rest := strings.TrimPrefix(line, "- ")
	delimiter := strings.IndexByte(rest, ':')
	if delimiter <= 0 {
		return "", "", false
	}
	key := rest[:delimiter]
	if !workMetadataKey(key) {
		return "", "", false
	}
	return key, workTrimASCIIHorizontal(rest[delimiter+1:]), true
}

func workMetadataKey(key string) bool {
	if key == "" || key[0] < 'A' || (key[0] > 'Z' && key[0] < 'a') || key[0] > 'z' {
		return false
	}
	for _, value := range []byte(key) {
		if (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') || value == ' ' || value == '-' || value == '_' {
			continue
		}
		return false
	}
	return true
}

func workChecklist(text, section string) (bool, int, []int) {
	found := section == ""
	inside := found
	scanner := workMarkdownScanner{}
	total := 0
	var unchecked []int
	for index, raw := range strings.Split(text, "\n") {
		line, visible := scanner.visible(raw)
		if !visible {
			continue
		}
		headingLine := strings.TrimRight(line, " \t")
		if section != "" {
			if headingLine == section {
				found = true
				inside = true
				continue
			}
			if strings.HasPrefix(headingLine, "## ") {
				inside = false
				continue
			}
		}
		if !inside {
			continue
		}
		if !scanner.listItem {
			continue
		}
		match := workChecklistItem.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		total++
		if match[1] == " " {
			unchecked = append(unchecked, index+1)
		}
	}
	return found, total, unchecked
}

type workMarkdownScanner struct {
	inHTMLComment bool
	inlineCode    int
	fenceMarker   byte
	fenceLength   int
	fenceBase     int
	container     int
	listItem      bool
}

func (s *workMarkdownScanner) visible(raw string) (string, bool) {
	s.listItem = false
	raw = strings.TrimSuffix(raw, "\r")
	if s.fenceMarker != 0 {
		line, ok := workRemoveContainerIndent(raw, s.fenceBase)
		if ok && workFenceClose(line, s.fenceMarker, s.fenceLength) {
			s.fenceMarker = 0
			s.fenceLength = 0
			s.fenceBase = 0
		}
		return "", false
	}

	if !s.inHTMLComment && workASCIIBlankLine(raw) {
		s.inlineCode = 0
		return raw, true
	}
	leadingRaw := workLeadingColumns(raw)
	commentBase := 0
	if s.container > 0 && leadingRaw >= s.container {
		commentBase = s.container
	}
	line := raw
	if leadingRaw-commentBase < 4 {
		line = s.withoutHTMLComments(raw)
	}
	if workASCIIBlankLine(line) {
		return line, true
	}
	leading := workLeadingColumns(line)
	if s.container > 0 && leading < s.container {
		s.container = 0
	}
	candidate := line
	base := 0
	if s.container > 0 && leading >= s.container {
		if stripped, ok := workRemoveContainerIndent(line, s.container); ok {
			candidate = stripped
			base = s.container
		}
	}
	if marker, length, opens := workFenceOpen(candidate); opens {
		s.inlineCode = 0
		s.fenceMarker = marker
		s.fenceLength = length
		s.fenceBase = base
		return "", false
	}
	if indent, listItem := workListContentIndent(line, s.container); listItem {
		s.container = indent
		s.listItem = true
	}
	return line, true
}

func (s *workMarkdownScanner) withoutHTMLComments(line string) string {
	var visible strings.Builder
	rest := line
	for {
		if s.inHTMLComment {
			end := strings.Index(rest, "-->")
			if end < 0 {
				workWriteCommentMask(&visible, rest)
				return visible.String()
			}
			workWriteCommentMask(&visible, rest[:end+3])
			rest = rest[end+3:]
			s.inHTMLComment = false
			continue
		}
		start, codeTicks := workHTMLCommentStart(rest, s.inlineCode)
		s.inlineCode = codeTicks
		if start < 0 {
			visible.WriteString(rest)
			return visible.String()
		}
		visible.WriteString(rest[:start])
		rest = rest[start:]
		s.inHTMLComment = true
	}
}

func workHTMLCommentStart(line string, codeTicks int) (int, int) {
	for index := 0; index < len(line); {
		if codeTicks == 0 && strings.HasPrefix(line[index:], "<!--") {
			if !workEscapedAt(line, index) {
				return index, codeTicks
			}
			index += len("<!--")
			continue
		}
		if line[index] != '`' || (codeTicks == 0 && workEscapedAt(line, index)) {
			index++
			continue
		}
		run := workBacktickRun(line, index)
		if codeTicks == 0 {
			codeTicks = run
		} else if run == codeTicks {
			codeTicks = 0
		}
		index += run
	}
	return -1, codeTicks
}

func workEscapedAt(line string, index int) bool {
	backslashes := 0
	for index > 0 && line[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 == 1
}

func workBacktickRun(line string, start int) int {
	length := 0
	for start+length < len(line) && line[start+length] == '`' {
		length++
	}
	return length
}

func workWriteCommentMask(visible *strings.Builder, value string) {
	for index := 0; index < len(value); index++ {
		if value[index] == '\t' {
			visible.WriteByte('\t')
		} else {
			visible.WriteByte(' ')
		}
	}
}

func workASCIIBlankLine(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] != ' ' && value[index] != '\t' {
			return false
		}
	}
	return true
}

func workTrimASCIIHorizontal(value string) string {
	return strings.Trim(value, " \t")
}

func workListContentIndent(line string, container int) (int, bool) {
	if container > 0 {
		if indent, ok := workListContentIndentAt(line, container); ok {
			return indent, true
		}
	}
	return workListContentIndentAt(line, 0)
}

func workListContentIndentAt(line string, base int) (int, bool) {
	rest, ok := workRemoveContainerIndent(line, base)
	if !ok {
		return 0, false
	}
	marker, column := workLeadingIndent(rest, base)
	if column-base > 3 || marker == len(rest) {
		return 0, false
	}
	if rest[marker] == '-' || rest[marker] == '+' || rest[marker] == '*' {
		marker++
		column++
	} else {
		start := marker
		for marker < len(rest) && marker-start < 9 && rest[marker] >= '0' && rest[marker] <= '9' {
			marker++
			column++
		}
		if marker == start || marker >= len(rest) || (rest[marker] != '.' && rest[marker] != ')') {
			return 0, false
		}
		marker++
		column++
	}
	spacingStart := column
	for marker < len(rest) && (rest[marker] == ' ' || rest[marker] == '\t') && column-spacingStart < 4 {
		next := column + 1
		if rest[marker] == '\t' {
			next = workNextTabStop(column)
		}
		if next-spacingStart > 4 {
			break
		}
		column = next
		marker++
	}
	if column == spacingStart {
		return 0, false
	}
	return column, true
}

func workLeadingColumns(line string) int {
	_, column := workLeadingIndent(line, 0)
	return column
}

func workLeadingIndent(line string, startColumn int) (int, int) {
	index := 0
	column := startColumn
	for index < len(line) {
		switch line[index] {
		case ' ':
			column++
		case '\t':
			column = workNextTabStop(column)
		default:
			return index, column
		}
		index++
	}
	return index, column
}

func workNextTabStop(column int) int {
	return column + 4 - column%4
}

func workRemoveContainerIndent(line string, indent int) (string, bool) {
	if indent == 0 {
		return line, true
	}
	index := 0
	column := 0
	for index < len(line) && column < indent {
		switch line[index] {
		case ' ':
			column++
		case '\t':
			column = workNextTabStop(column)
		default:
			return "", false
		}
		index++
	}
	if column < indent {
		return "", false
	}
	leadingBytes, endColumn := workLeadingIndent(line[index:], column)
	return strings.Repeat(" ", endColumn-indent) + line[index+leadingBytes:], true
}

func workFenceOpen(line string) (byte, int, bool) {
	rest, ok := workFenceIndent(line)
	if !ok || len(rest) < 3 || (rest[0] != '`' && rest[0] != '~') {
		return 0, 0, false
	}
	marker := rest[0]
	length := 0
	for length < len(rest) && rest[length] == marker {
		length++
	}
	if length < 3 || (marker == '`' && strings.ContainsRune(rest[length:], '`')) {
		return 0, 0, false
	}
	return marker, length, true
}

func workFenceClose(line string, marker byte, minimum int) bool {
	rest, ok := workFenceIndent(line)
	if !ok {
		return false
	}
	length := 0
	for length < len(rest) && rest[length] == marker {
		length++
	}
	return length >= minimum && workASCIIFenceSpace(rest[length:])
}

func workASCIIFenceSpace(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] != ' ' && value[index] != '\t' {
			return false
		}
	}
	return true
}

func workFenceIndent(line string) (string, bool) {
	index, columns := workLeadingIndent(line, 0)
	if columns > 3 {
		return "", false
	}
	return line[index:], true
}

func readRegularRepositoryFile(root, relative string) ([]byte, error) {
	if err := validateRepositoryPaths(root, []string{relative}); err != nil {
		return nil, err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	data, err := os.ReadFile(path) // #nosec G304 -- relative is validated as a local regular repository path above.
	if err != nil {
		return nil, fmt.Errorf("read repository path %q: %w", relative, err)
	}
	return data, nil
}

func checkLicense(root string, config projectconfig.Config, scope string) []issue {
	if scope != "public" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(root, "LICENSE")) // #nosec G304 -- LICENSE is a fixed file below the selected repository root.
	if err != nil {
		return nil
	}
	text := string(data)
	valid := false
	switch config.Project.LicenseSPDX {
	case "MIT":
		valid = strings.Contains(text, "MIT License") && strings.Contains(text, "Permission is hereby granted")
	case "Apache-2.0":
		valid = strings.Contains(text, "Apache License") && strings.Contains(text, "Version 2.0")
	default:
		valid = strings.Contains(text, config.Project.LicenseSPDX)
	}
	if !valid {
		return []issue{{Path: "LICENSE", Message: "content does not match project.license_spdx; choose and update the license deliberately"}}
	}
	return nil
}

func repositoryPaths(root string) ([]string, error) {
	command := exec.Command("git", "ls-files", "-co", "--exclude-standard", "-z")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return nil, fmt.Errorf("git ls-files: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for _, raw := range bytes.Split(output, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		relative := string(raw)
		if !filepath.IsLocal(relative) {
			return nil, fmt.Errorf("git returned a non-local path %q", relative)
		}
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			if os.IsNotExist(err) {
				// Git keeps tracked working-tree deletions in its cached path set.
				// Bootstrap renames intentionally create that state before staging.
				continue
			}
			return nil, fmt.Errorf("inspect git path %q: %w", relative, err)
		}
		paths = append(paths, filepath.ToSlash(relative))
	}
	sort.Strings(paths)
	return paths, nil
}

// validateRepositoryPaths rejects links and special files before repoguard
// reads any repository-controlled content. A link in any path component is
// rejected so an apparently local path cannot redirect a read outside root.
func validateRepositoryPaths(root string, paths []string) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository root is a symbolic link: %s", root)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("repository root is not a directory: %s", root)
	}
	for _, relative := range paths {
		if !filepath.IsLocal(relative) || filepath.IsAbs(relative) {
			return fmt.Errorf("repository path is not local: %q", relative)
		}
		parts := strings.Split(filepath.Clean(filepath.FromSlash(relative)), string(filepath.Separator))
		current := root
		for index, part := range parts {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil {
				return fmt.Errorf("inspect repository path %q: %w", relative, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("repository path is a symbolic link: %s", relative)
			}
			if index < len(parts)-1 && !info.IsDir() {
				return fmt.Errorf("repository path component is not a directory: %s", relative)
			}
			if index == len(parts)-1 && !info.Mode().IsRegular() {
				return fmt.Errorf("repository path is not a regular file: %s", relative)
			}
		}
	}
	return nil
}

func checkRequired(root string, config projectconfig.Config, scope string) []issue {
	if scope != "public" {
		return nil
	}
	var issues []issue
	for _, path := range config.PublicGuard.Required {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			issues = append(issues, issue{Path: path, Message: "required public repository path is missing"})
		}
	}
	return issues
}

func checkAgentHarness(root string) []issue {
	paths := []string{
		".agents/skills/bootstrap-derived-cli/SKILL.md",
		".agents/skills/bootstrap-derived-cli/agents/openai.yaml",
		".agents/skills/add-capability/SKILL.md",
	}
	var issues []issue
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			issues = append(issues, issue{Path: path, Message: "required Codex harness file is missing"})
		}
	}
	return issues
}

func checkFilesystemShape(root string, config projectconfig.Config) ([]issue, error) {
	var issues []issue
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "bin" || entry.Name() == "dist") {
			return filepath.SkipDir
		}
		if strings.EqualFold(entry.Name(), ".claude") {
			issues = append(issues, issue{Path: relative, Message: "Claude-specific harness paths are outside this Codex-only template"})
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		if entry.Type()&os.ModeSymlink != 0 {
			issues = append(issues, issue{Path: relative, Message: "symbolic links are not allowed in the public working tree"})
			return nil
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			issues = append(issues, issue{Path: relative, Message: "special files are not allowed in the public working tree"})
			return nil
		}
		if !entry.IsDir() {
			issues = append(issues, checkWorkingTreeArtifact(relative, config)...)
		}
		return nil
	})
	return issues, err
}

func checkWorkingTreeArtifact(path string, config projectconfig.Config) []issue {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))
	if strings.EqualFold(base, "claude.md") {
		return []issue{{Path: path, Message: "Claude-specific instructions are outside this Codex-only template"}}
	}
	if strings.HasSuffix(lower, ".bootstrap.tmp") || strings.HasSuffix(lower, ".bootstrap.orig") {
		return []issue{{Path: path, Message: "interrupted bootstrap residue must not be published"}}
	}
	if filepath.ToSlash(path) == config.Project.BinaryName || filepath.ToSlash(path) == config.Project.BinaryName+".exe" {
		return []issue{{Path: path, Message: "root build artifact must not be published; use bin/ or a temporary release directory"}}
	}
	return nil
}

func checkPath(path string) []issue {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))
	forbiddenBase := map[string]bool{
		".env": true, ".ds_store": true, "credentials.json": true, "secrets.json": true,
		"id_rsa": true, "id_ed25519": true,
	}
	if forbiddenBase[base] || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx") {
		return []issue{{Path: path, Message: "sensitive or local-only path must not be published"}}
	}
	return nil
}

func checkMarkdownLinks(root string, repositoryPaths []string) ([]issue, error) {
	publishable := make(map[string]bool, len(repositoryPaths))
	for _, relative := range repositoryPaths {
		publishable[relative] = true
	}
	var issues []issue
	for _, source := range repositoryPaths {
		if !strings.HasSuffix(strings.ToLower(source), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(source))) // #nosec G304 -- source was validated as a local regular repository path.
		if err != nil {
			return nil, err
		}
		inFence := false
		for index, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			var destinations []string
			for _, match := range inlineMarkdownLink.FindAllStringSubmatch(line, -1) {
				destinations = append(destinations, match[1])
			}
			if match := referenceMarkdownLink.FindStringSubmatch(line); match != nil {
				destinations = append(destinations, match[1])
			}
			for _, raw := range destinations {
				destination := markdownDestination(raw)
				if destination == "" || strings.HasPrefix(destination, "#") || strings.HasPrefix(destination, "//") || uriScheme.MatchString(destination) {
					continue
				}
				local := destination
				if delimiter := strings.IndexAny(local, "?#"); delimiter >= 0 {
					local = local[:delimiter]
				}
				if local == "" {
					continue
				}
				if problem := validateMarkdownTarget(root, source, local, publishable); problem != "" {
					issues = append(issues, issue{Path: source, Line: index + 1, Message: problem})
				}
			}
		}
	}
	return issues, nil
}

func markdownDestination(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "<") {
		if end := strings.Index(trimmed, ">"); end > 0 {
			return trimmed[1:end]
		}
		return trimmed
	}
	if fields := strings.Fields(trimmed); len(fields) != 0 {
		return fields[0]
	}
	return ""
}

func validateMarkdownTarget(root, source, local string, publishable map[string]bool) string {
	if strings.HasPrefix(local, "/") || strings.Contains(local, `\`) {
		return fmt.Sprintf("local Markdown link %q must use a repository-relative slash path", local)
	}
	cleaned := pathpkg.Clean(local)
	canonical := cleaned
	if strings.HasSuffix(local, "/") && cleaned != "." {
		canonical += "/"
	}
	if local != canonical {
		return fmt.Sprintf("local Markdown link %q is not canonical", local)
	}
	resolved := pathpkg.Clean(pathpkg.Join(pathpkg.Dir(source), cleaned))
	if resolved == ".." || strings.HasPrefix(resolved, "../") || pathpkg.IsAbs(resolved) {
		return fmt.Sprintf("local Markdown link %q escapes the repository", local)
	}
	if !publishable[resolved] {
		return fmt.Sprintf("local Markdown link %q does not target a publishable regular file", local)
	}
	current := root
	for _, component := range strings.Split(resolved, "/") {
		current = filepath.Join(current, filepath.FromSlash(component))
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Sprintf("local Markdown link %q cannot be resolved: %v", local, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Sprintf("local Markdown link %q resolves through a symbolic link", local)
		}
	}
	info, err := os.Lstat(current)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Sprintf("local Markdown link %q does not target a regular file", local)
	}
	return ""
}

func checkText(path, text string, config projectconfig.Config, denylist []string, scope string) []issue {
	return checkTextWithLocaleExemption(path, text, config, denylist, scope, false)
}

func checkTextWithLocaleExemption(path, text string, config projectconfig.Config, denylist []string, scope string, documentationLocaleExempt bool) []issue {
	var issues []issue
	scannerSource := path == "tools/repoguard/main.go"
	lines := strings.Split(text, "\n")
	checkDocumentationLocale := strings.HasSuffix(strings.ToLower(path), ".md") &&
		!documentationLocaleExempt &&
		isEnglishDocumentationLocale(config.PublicGuard.DocumentationLocale)
	documentationLines := lines
	documentationVisible := make([]bool, len(lines))
	if checkDocumentationLocale {
		documentationLines, documentationVisible = projectDocumentationLocale(lines)
	}
	for index, line := range lines {
		lineNumber := index + 1
		documentationLine := line
		visible := true
		if checkDocumentationLocale {
			documentationLine = documentationLines[index]
			visible = documentationVisible[index]
		}
		if config.Profile == "ready" && path != "tools/internal/projectconfig/defaults.go" {
			if identity := remainingTemplateIdentity(line, config.Project); identity != "" {
				issues = append(issues, issue{Path: path, Line: lineNumber, Message: fmt.Sprintf("template identity %q remains after bootstrap", identity)})
			}
		}
		if config.Profile == "ready" && !scannerSource && bootstrapPlaceholder.MatchString(line) {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "unresolved bootstrap placeholder"})
		}
		if checkDocumentationLocale && visible && japaneseText.MatchString(documentationLine) {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "documentation contains Japanese text while public_guard.documentation_locale is English"})
		}
		if !scannerSource && absoluteHome.MatchString(line) {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "machine-specific home directory path"})
		}
		if !scannerSource && privateNetwork.MatchString(line) {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "private hostname or network address"})
		}
		for _, term := range denylist {
			if path != filepath.ToSlash(config.PublicGuard.DenylistFile) && strings.Contains(strings.ToLower(line), strings.ToLower(term)) {
				issues = append(issues, issue{Path: path, Line: lineNumber, Message: fmt.Sprintf("custom denylist term %q", term)})
			}
		}
		if !scannerSource {
			issues = append(issues, checkFormulaPlaceholder(path, line, lineNumber)...)
		}
		if !scannerSource && (scope == "security" || scope == "public") {
			issues = append(issues, checkSecrets(path, line, lineNumber)...)
		}
	}
	return issues
}

// projectDocumentationLocale masks fenced blocks, HTML comments, block quotes,
// bounded inline-code spans, and parsed link destinations while preserving one
// result per source line. Blank lines, quotes, and fences bound inline parsing,
// so an unmatched delimiter cannot hide later trusted prose.
func projectDocumentationLocale(lines []string) ([]string, []bool) {
	projected := make([]string, len(lines))
	visible := make([]bool, len(lines))
	scanner := workMarkdownScanner{}
	for index, line := range lines {
		projected[index], visible[index] = scanner.visible(line)
		if visible[index] && strings.HasPrefix(strings.TrimLeft(projected[index], " \t"), ">") {
			visible[index] = false
		}
	}
	for start := 0; start < len(lines); {
		if !visible[start] || workASCIIBlankLine(projected[start]) {
			start++
			continue
		}
		end := start + 1
		for end < len(lines) && visible[end] && !workASCIIBlankLine(projected[end]) {
			end++
		}
		segment := strings.Join(projected[start:end], "\n")
		maskedSegment := maskMarkdownInlineCode(segment)
		maskedLines := strings.Split(maskedSegment, "\n")
		for offset, line := range maskedLines {
			masked := []byte(line)
			maskMarkdownLinkDestinations(line, masked)
			projected[start+offset] = string(masked)
		}
		start = end
	}
	return projected, visible
}

func maskMarkdownLocaleNonProse(line string) string {
	projected, _ := projectDocumentationLocale([]string{line})
	return projected[0]
}

func maskMarkdownInlineCode(text string) string {
	masked := []byte(text)
	for index := 0; index < len(text); {
		if text[index] != '`' || workEscapedAt(text, index) {
			index++
			continue
		}
		run := workBacktickRun(text, index)
		closing := matchingBacktickRun(text, index+run, run)
		if closing < 0 {
			index += run
			continue
		}
		maskMarkdownBytes(masked, index, closing+run)
		index = closing + run
	}
	return string(masked)
}

func maskMarkdownLinkDestinations(line string, masked []byte) {
	maskMarkdownReferenceDestination(line, masked)
	for index := 0; index < len(line); {
		if line[index] != '[' || workEscapedAt(line, index) {
			index++
			continue
		}
		labelEnd := matchingMarkdownBracket(line, index)
		if labelEnd < 0 || labelEnd+1 >= len(line) || line[labelEnd+1] != '(' {
			index++
			continue
		}
		start, end, closeIndex, ok := markdownInlineDestination(line, labelEnd+1)
		if !ok {
			index++
			continue
		}
		maskMarkdownBytes(masked, start, end)
		index = closeIndex + 1
	}
}

func maskMarkdownReferenceDestination(line string, masked []byte) {
	index := 0
	for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	if index >= len(line) || line[index] != '[' || workEscapedAt(line, index) {
		return
	}
	labelEnd := matchingMarkdownBracket(line, index)
	if labelEnd < 0 || labelEnd+1 >= len(line) || line[labelEnd+1] != ':' {
		return
	}
	start, end, ok := markdownStandaloneDestination(line, labelEnd+2)
	if ok {
		maskMarkdownBytes(masked, start, end)
	}
}

func matchingMarkdownBracket(line string, start int) int {
	depth := 0
	for index := start; index < len(line); index++ {
		if workEscapedAt(line, index) {
			continue
		}
		switch line[index] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func markdownInlineDestination(line string, open int) (int, int, int, bool) {
	index := skipMarkdownSpace(line, open+1)
	if index >= len(line) {
		return 0, 0, 0, false
	}
	if line[index] == '<' {
		end := markdownUnescapedByte(line, index+1, '>')
		if end < 0 {
			return 0, 0, 0, false
		}
		closeIndex, ok := markdownLinkTail(line, end+1)
		return index + 1, end, closeIndex, ok
	}
	start := index
	depth := 0
	for index < len(line) {
		if workEscapedAt(line, index) {
			index++
			continue
		}
		switch line[index] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return start, index, index, true
			}
			depth--
		case ' ', '\t':
			if depth != 0 {
				return 0, 0, 0, false
			}
			closeIndex, ok := markdownLinkTail(line, index)
			return start, index, closeIndex, ok
		}
		index++
	}
	return 0, 0, 0, false
}

func markdownLinkTail(line string, start int) (int, bool) {
	index := skipMarkdownSpace(line, start)
	if index < len(line) && line[index] == ')' {
		return index, true
	}
	if index >= len(line) || (line[index] != '\'' && line[index] != '"' && line[index] != '(') {
		return 0, false
	}
	opening := line[index]
	closing := opening
	if opening == '(' {
		closing = ')'
	}
	titleEnd := markdownUnescapedByte(line, index+1, closing)
	if titleEnd < 0 {
		return 0, false
	}
	index = skipMarkdownSpace(line, titleEnd+1)
	return index, index < len(line) && line[index] == ')'
}

func markdownStandaloneDestination(line string, start int) (int, int, bool) {
	index := skipMarkdownSpace(line, start)
	if index >= len(line) {
		return 0, 0, false
	}
	if line[index] == '<' {
		end := markdownUnescapedByte(line, index+1, '>')
		return index + 1, end, end >= 0
	}
	begin := index
	depth := 0
	for index < len(line) && line[index] != ' ' && line[index] != '\t' {
		if workEscapedAt(line, index) {
			index++
			continue
		}
		if line[index] == '(' {
			depth++
		} else if line[index] == ')' {
			if depth == 0 {
				return 0, 0, false
			}
			depth--
		}
		index++
	}
	return begin, index, depth == 0
}

func skipMarkdownSpace(line string, start int) int {
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	return start
}

func markdownUnescapedByte(line string, start int, target byte) int {
	for index := start; index < len(line); index++ {
		if line[index] == target && !workEscapedAt(line, index) {
			return index
		}
	}
	return -1
}

func matchingBacktickRun(line string, start, length int) int {
	for index := start; index < len(line); {
		if line[index] != '`' || workEscapedAt(line, index) {
			index++
			continue
		}
		run := workBacktickRun(line, index)
		if run == length {
			return index
		}
		index += run
	}
	return -1
}

func maskMarkdownBytes(masked []byte, start, end int) {
	for index := start; index < end; index++ {
		if masked[index] != '\t' && masked[index] != '\n' && masked[index] != '\r' {
			masked[index] = ' '
		}
	}
}

func isEnglishDocumentationLocale(locale string) bool {
	return locale == "en" || strings.HasPrefix(locale, "en-") || locale == "eng" || strings.HasPrefix(locale, "eng-")
}

func checkFormulaPlaceholder(path, line string, lineNumber int) []issue {
	matches := formulaPlaceholder.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil
	}
	validPath := (strings.HasPrefix(filepath.ToSlash(path), "Formula/") && strings.HasSuffix(path, ".rb.template")) ||
		path == "scripts/render-formula.sh" || path == "scripts/lint-release.sh"
	var issues []issue
	for _, match := range matches {
		if !validPath || !allowedFormulaPlaceholders[match[1]] {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "unknown or misplaced release-time placeholder " + match[0]})
		}
	}
	return issues
}

func checkSecrets(path, line string, lineNumber int) []issue {
	var issues []issue
	for _, pattern := range secretPatterns {
		if pattern.re.MatchString(line) {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "secret-like content: " + pattern.name})
		}
	}
	for _, match := range authorizationSecret.FindAllStringSubmatch(line, -1) {
		if !safeExampleSecret(match[1]) {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "secret-like content: authorization header"})
		}
	}
	for _, match := range assignmentSecret.FindAllStringSubmatch(line, -1) {
		value := ""
		for _, candidate := range match[1:] {
			if candidate != "" {
				value = candidate
				break
			}
		}
		if !safeExampleSecret(value) {
			issues = append(issues, issue{Path: path, Line: lineNumber, Message: "secret-like value assigned in source"})
		}
	}
	return issues
}

func safeExampleSecret(value string) bool {
	trimmed := strings.TrimSpace(strings.Trim(value, `"'`))
	lower := strings.ToLower(trimmed)
	if lower == "" || lower == "none" || lower == "null" || lower == "[redacted]" {
		return true
	}
	return exampleSecret.MatchString(lower) || environmentSecret.MatchString(trimmed)
}

func remainingTemplateIdentity(line string, target projectconfig.Project) string {
	defaults := projectconfig.Defaults
	type identityReplacement struct {
		from string
		to   string
	}
	values := []identityReplacement{
		{"https://github.com/" + defaults.GitHubOwner + "/" + defaults.GitHubRepository, "https://github.com/" + target.GitHubOwner + "/" + target.GitHubRepository},
		{defaults.GoModule, target.GoModule},
		{defaults.GitHubOwner + "/" + defaults.GitHubRepository, target.GitHubOwner + "/" + target.GitHubRepository},
		{defaults.Description, target.Description},
		{defaults.SecurityContact, target.SecurityContact},
		{defaults.Name, target.Name},
		{defaults.FormulaClass, target.FormulaClass},
		{defaults.BinaryName, target.BinaryName},
		{defaults.GitHubRepository, target.GitHubRepository},
	}
	sort.SliceStable(values, func(i, j int) bool { return len(values[i].to) > len(values[j].to) })
	withoutTargetIdentity := line
	for _, value := range values {
		if value.to != "" && value.to != value.from {
			withoutTargetIdentity = strings.ReplaceAll(withoutTargetIdentity, value.to, "")
		}
	}
	sort.SliceStable(values, func(i, j int) bool { return len(values[i].from) > len(values[j].from) })
	for _, value := range values {
		if strings.Contains(withoutTargetIdentity, value.from) {
			return value.from
		}
	}
	return ""
}

func readDenylist(root, relative string) ([]string, error) {
	if !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("denylist path is not local: %q", relative)
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	file, err := os.Open(path) // #nosec G304 -- relative is validated and scoped to the selected repository root.
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var terms []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		term := strings.TrimSpace(scanner.Text())
		if term != "" && !strings.HasPrefix(term, "#") {
			terms = append(terms, term)
		}
	}
	return terms, scanner.Err()
}
