// Command bootstrap applies validated, exact template substitutions once.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tasuku43/agentic-cli-foundry/tools/internal/projectconfig"
)

type replacement struct {
	From string
	To   string
}

type contentUpdate struct {
	Relative  string
	Path      string
	Temporary string
	Backup    string
	Original  []byte
	Updated   []byte
	Mode      os.FileMode
}

type pathRename struct {
	FromRelative string
	ToRelative   string
	Source       string
	Target       string
	Mode         os.FileMode
}

type bootstrapPlan struct {
	Updates []contentUpdate
	Renames []pathRename
}

var commitRename = os.Rename

func main() {
	dryRun := flag.Bool("dry-run", false, "print changes without writing files")
	rootFlag := flag.String("root", ".", "repository root")
	flag.Parse()
	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		fatal(err)
	}
	if err := rejectRepositorySymlinks(root); err != nil {
		fatal(err)
	}
	config, err := projectconfig.Load(root)
	if err != nil {
		fatal(err)
	}
	if config.Profile != "template" {
		fatal(fmt.Errorf("profile is %q; bootstrap only runs once from the template profile", config.Profile))
	}
	if problems := projectconfig.ReadyProblems(config.Project); len(problems) != 0 {
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "bootstrap:", problem)
		}
		os.Exit(1)
	}

	changed, renames, err := applyConfigured(root, config, *dryRun)
	if err != nil {
		fatal(err)
	}
	for _, path := range changed {
		fmt.Println("update", path)
	}
	for _, rename := range renames {
		fmt.Printf("rename %s -> %s\n", rename[0], rename[1])
	}
	if *dryRun {
		fmt.Printf("bootstrap dry-run: %d updates, %d renames\n", len(changed), len(renames))
		return
	}
	fmt.Printf("bootstrap: identity ready (%d updates, %d renames)\n", len(changed), len(renames))
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
	os.Exit(1)
}

func replacements(target projectconfig.Project) []replacement {
	defaults := projectconfig.Defaults
	values := []replacement{
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
	sort.SliceStable(values, func(i, j int) bool { return len(values[i].From) > len(values[j].From) })
	return values
}

func apply(root string, replacements []replacement, dryRun bool) ([]string, [][2]string, error) {
	plan, err := buildPlan(root, replacements)
	if err != nil {
		return nil, nil, err
	}
	changed, renames := planSummary(plan)
	if dryRun {
		return changed, renames, nil
	}
	if err := executePlan(root, plan); err != nil {
		return nil, nil, err
	}
	return changed, renames, nil
}

func applyConfigured(root string, config projectconfig.Config, dryRun bool) ([]string, [][2]string, error) {
	plan, err := buildPlan(root, replacements(config.Project))
	if err != nil {
		return nil, nil, err
	}
	plan, err = addReadyProfileUpdate(root, plan, config)
	if err != nil {
		return nil, nil, err
	}
	changed, renames := planSummary(plan)
	if dryRun {
		return changed, renames, nil
	}
	if err := executePlan(root, plan); err != nil {
		return nil, nil, err
	}
	return changed, renames, nil
}

func addReadyProfileUpdate(root string, plan bootstrapPlan, config projectconfig.Config) (bootstrapPlan, error) {
	config.Profile = "ready"
	if err := config.Validate(); err != nil {
		return bootstrapPlan{}, err
	}
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return bootstrapPlan{}, err
	}
	updated = append(updated, '\n')
	relative := ".harness/project.json"
	path := filepath.Join(root, filepath.FromSlash(relative))
	for index := range plan.Updates {
		if plan.Updates[index].Relative == relative {
			plan.Updates[index].Updated = updated
			return plan, nil
		}
	}
	original, err := os.ReadFile(path) // #nosec G304 -- path is the fixed project profile below the selected repository root.
	if err != nil {
		return bootstrapPlan{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return bootstrapPlan{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return bootstrapPlan{}, fmt.Errorf("bootstrap only accepts a regular project profile: %s", relative)
	}
	update, err := newContentUpdate(relative, path, original, updated, info.Mode().Perm())
	if err != nil {
		return bootstrapPlan{}, err
	}
	plan.Updates = append(plan.Updates, update)
	sort.Slice(plan.Updates, func(i, j int) bool {
		return plan.Updates[i].Relative < plan.Updates[j].Relative
	})
	return plan, nil
}

func buildPlan(root string, replacements []replacement) (bootstrapPlan, error) {
	var replacementPairs []string
	for _, item := range replacements {
		if item.From != item.To {
			replacementPairs = append(replacementPairs, item.From, item.To)
		}
	}
	replacer := strings.NewReplacer(replacementPairs...)
	pathReplacer := newPathReplacer(replacements)
	var plan bootstrapPlan
	err := walkBootstrapTree(root, func(path string, entry os.DirEntry) error {
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		updatedBase := pathReplacer.Replace(filepath.Base(path))
		if updatedBase != filepath.Base(path) {
			target := filepath.Join(filepath.Dir(path), updatedBase)
			toRelative, err := filepath.Rel(root, target)
			if err != nil || !filepath.IsLocal(toRelative) {
				return fmt.Errorf("rename target leaves repository: %s", target)
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			plan.Renames = append(plan.Renames, pathRename{
				FromRelative: relative,
				ToRelative:   filepath.ToSlash(toRelative),
				Source:       path,
				Target:       target,
				Mode:         info.Mode(),
			})
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(relative, "tools/internal/projectconfig/") {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 -- path was returned by WalkDir below the selected repository root.
		if err != nil {
			return err
		}
		if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
			return nil
		}
		updatedText := replacer.Replace(string(data))
		if updatedText == string(data) {
			return nil
		}
		updated := []byte(updatedText)
		if filepath.Ext(relative) == ".go" {
			updated, err = format.Source(updated)
			if err != nil {
				return fmt.Errorf("format updated Go source %s: %w", relative, err)
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("bootstrap only accepts regular files: %s", relative)
		}
		update, err := newContentUpdate(relative, path, data, updated, info.Mode().Perm())
		if err != nil {
			return err
		}
		plan.Updates = append(plan.Updates, update)
		return nil
	})
	if err != nil {
		return bootstrapPlan{}, err
	}
	sort.Slice(plan.Updates, func(i, j int) bool {
		return plan.Updates[i].Relative < plan.Updates[j].Relative
	})
	sort.Slice(plan.Renames, func(i, j int) bool {
		iDepth := strings.Count(plan.Renames[i].Source, string(filepath.Separator))
		jDepth := strings.Count(plan.Renames[j].Source, string(filepath.Separator))
		if iDepth != jDepth {
			return iDepth > jDepth
		}
		return plan.Renames[i].FromRelative < plan.Renames[j].FromRelative
	})
	seenTargets := make(map[string]string, len(plan.Renames))
	for _, rename := range plan.Renames {
		if previous, exists := seenTargets[rename.Target]; exists {
			return bootstrapPlan{}, fmt.Errorf("rename target %s is produced by both %s and %s", rename.ToRelative, previous, rename.FromRelative)
		}
		seenTargets[rename.Target] = rename.FromRelative
		if _, err := os.Lstat(rename.Target); err == nil {
			return bootstrapPlan{}, fmt.Errorf("rename target already exists: %s", rename.Target)
		} else if !os.IsNotExist(err) {
			return bootstrapPlan{}, err
		}
	}
	return plan, nil
}

func newContentUpdate(relative, path string, original, updated []byte, mode os.FileMode) (contentUpdate, error) {
	update := contentUpdate{
		Relative:  relative,
		Path:      path,
		Temporary: path + ".bootstrap.tmp",
		Backup:    path + ".bootstrap.orig",
		Original:  append([]byte(nil), original...),
		Updated:   append([]byte(nil), updated...),
		Mode:      mode,
	}
	for _, reserved := range []string{update.Temporary, update.Backup} {
		if _, err := os.Lstat(reserved); err == nil {
			return contentUpdate{}, fmt.Errorf("bootstrap reserved path already exists: %s", reserved)
		} else if !os.IsNotExist(err) {
			return contentUpdate{}, err
		}
	}
	return update, nil
}

func newPathReplacer(replacements []replacement) *strings.Replacer {
	allowed := map[string]bool{
		projectconfig.Defaults.BinaryName:       true,
		projectconfig.Defaults.GitHubRepository: true,
	}
	var pairs []string
	for _, item := range replacements {
		if allowed[item.From] && item.From != item.To {
			pairs = append(pairs, item.From, item.To)
		}
	}
	return strings.NewReplacer(pairs...)
}

func walkBootstrapTree(root string, visit func(string, os.DirEntry) error) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository root is a symbolic link: %s", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("repository root is not a directory: %s", root)
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != root && entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && path != root && (entry.Name() == "bin" || entry.Name() == "dist") {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			relative, _ := filepath.Rel(root, path)
			return fmt.Errorf("symbolic link is not allowed during bootstrap: %s", filepath.ToSlash(relative))
		}
		return visit(path, entry)
	})
}

func rejectRepositorySymlinks(root string) error {
	return walkBootstrapTree(root, func(string, os.DirEntry) error { return nil })
}

func planSummary(plan bootstrapPlan) ([]string, [][2]string) {
	changed := make([]string, 0, len(plan.Updates))
	for _, update := range plan.Updates {
		changed = append(changed, update.Relative)
	}
	renames := make([][2]string, 0, len(plan.Renames))
	for _, rename := range plan.Renames {
		renames = append(renames, [2]string{rename.FromRelative, rename.ToRelative})
	}
	return changed, renames
}

func executePlan(root string, plan bootstrapPlan) error {
	staged := make([]string, 0, len(plan.Updates))
	defer func() {
		for _, path := range staged {
			_ = os.Remove(path)
		}
	}()
	for _, update := range plan.Updates {
		if err := stageContentUpdate(update); err != nil {
			return err
		}
		staged = append(staged, update.Temporary)
	}
	if err := validatePlanCurrent(root, plan); err != nil {
		return err
	}
	committedUpdates := make([]contentUpdate, 0, len(plan.Updates))
	committedRenames := make([]pathRename, 0, len(plan.Renames))
	rollback := func(cause error) error {
		var rollbackErrors []string
		for index := len(committedRenames) - 1; index >= 0; index-- {
			rename := committedRenames[index]
			if err := os.Rename(rename.Target, rename.Source); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Sprintf("restore rename %s: %v", rename.FromRelative, err))
			}
		}
		for index := len(committedUpdates) - 1; index >= 0; index-- {
			update := committedUpdates[index]
			if err := os.Remove(update.Path); err != nil && !os.IsNotExist(err) {
				rollbackErrors = append(rollbackErrors, fmt.Sprintf("remove partial update %s: %v", update.Relative, err))
			}
			if err := os.Rename(update.Backup, update.Path); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Sprintf("restore update %s: %v", update.Relative, err))
			}
		}
		if len(rollbackErrors) != 0 {
			return fmt.Errorf("%w; bootstrap rollback also failed: %s", cause, strings.Join(rollbackErrors, "; "))
		}
		return cause
	}
	for _, update := range plan.Updates {
		if err := commitRename(update.Path, update.Backup); err != nil {
			return rollback(fmt.Errorf("back up update %s: %w", update.Relative, err))
		}
		committedUpdates = append(committedUpdates, update)
		if err := commitRename(update.Temporary, update.Path); err != nil {
			return rollback(fmt.Errorf("commit update %s: %w", update.Relative, err))
		}
	}
	for _, rename := range plan.Renames {
		if err := commitRename(rename.Source, rename.Target); err != nil {
			return rollback(fmt.Errorf("rename %s to %s: %w", rename.FromRelative, rename.ToRelative, err))
		}
		committedRenames = append(committedRenames, rename)
	}
	for _, update := range committedUpdates {
		backup := pathAfterRenames(update.Backup, committedRenames)
		if err := os.Remove(backup); err != nil {
			return fmt.Errorf("remove bootstrap backup for %s: %w", update.Relative, err)
		}
	}
	return nil
}

func pathAfterRenames(path string, renames []pathRename) string {
	current := path
	for _, rename := range renames {
		prefix := rename.Source + string(filepath.Separator)
		if strings.HasPrefix(current, prefix) {
			current = filepath.Join(rename.Target, strings.TrimPrefix(current, prefix))
		}
	}
	return current
}

func stageContentUpdate(update contentUpdate) error {
	file, err := os.OpenFile(update.Temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, update.Mode) // #nosec G304 -- the path is preflighted below the selected repository root.
	if err != nil {
		return fmt.Errorf("stage update %s: %w", update.Relative, err)
	}
	written, writeErr := file.Write(update.Updated)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(update.Temporary)
		return fmt.Errorf("stage update %s: %w", update.Relative, writeErr)
	}
	if written != len(update.Updated) {
		_ = os.Remove(update.Temporary)
		return fmt.Errorf("stage update %s: short write", update.Relative)
	}
	if closeErr != nil {
		_ = os.Remove(update.Temporary)
		return fmt.Errorf("stage update %s: %w", update.Relative, closeErr)
	}
	if err := os.Chmod(update.Temporary, update.Mode); err != nil {
		_ = os.Remove(update.Temporary)
		return fmt.Errorf("stage update %s: %w", update.Relative, err)
	}
	return nil
}

func validatePlanCurrent(root string, plan bootstrapPlan) error {
	if err := rejectRepositorySymlinks(root); err != nil {
		return err
	}
	for _, update := range plan.Updates {
		info, err := os.Lstat(update.Path)
		if err != nil {
			return fmt.Errorf("revalidate update %s: %w", update.Relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != update.Mode {
			return fmt.Errorf("repository file type or mode changed after bootstrap preflight: %s", update.Relative)
		}
		data, err := os.ReadFile(update.Path) // #nosec G304 -- the path came from the preflight repository walk.
		if err != nil {
			return fmt.Errorf("revalidate update %s: %w", update.Relative, err)
		}
		if !bytes.Equal(data, update.Original) {
			return fmt.Errorf("repository changed after bootstrap preflight: %s", update.Relative)
		}
		temporaryInfo, err := os.Lstat(update.Temporary)
		if err != nil {
			return fmt.Errorf("staged update disappeared after bootstrap preflight: %s", update.Relative)
		}
		if temporaryInfo.Mode()&os.ModeSymlink != 0 || !temporaryInfo.Mode().IsRegular() {
			return fmt.Errorf("staged update is not a regular file: %s", update.Relative)
		}
		if _, err := os.Lstat(update.Backup); err == nil {
			return fmt.Errorf("bootstrap backup path appeared after preflight: %s", update.Relative)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, rename := range plan.Renames {
		info, err := os.Lstat(rename.Source)
		if err != nil {
			return fmt.Errorf("rename source changed after bootstrap preflight: %s", rename.FromRelative)
		}
		if info.Mode() != rename.Mode {
			return fmt.Errorf("rename source type or mode changed after bootstrap preflight: %s", rename.FromRelative)
		}
		if _, err := os.Lstat(rename.Target); err == nil {
			return fmt.Errorf("rename target appeared after bootstrap preflight: %s", rename.ToRelative)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
