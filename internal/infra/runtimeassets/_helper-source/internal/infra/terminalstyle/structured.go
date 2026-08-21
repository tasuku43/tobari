package terminalstyle

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// MaxStructuredCandidateBytes bounds the amount of untrusted child output
// held while deciding whether it is a structured document. A larger or
// incomplete candidate is passed through without styling.
const MaxStructuredCandidateBytes = 256 * 1024

const (
	structuredReset       = "\x1b[0m"
	structuredPunctuation = "\x1b[38;5;250m"
	structuredKey         = "\x1b[38;5;81m"
	structuredString      = "\x1b[38;5;114m"
	structuredNumber      = "\x1b[38;5;214m"
	structuredKeyword     = "\x1b[38;5;177m"
	structuredComment     = "\x1b[38;5;244m"
)

// StructuredWriter is a bounded, display-only stream projection. It emits
// original bytes in their original order and may add only Tobari-owned SGR
// spans around recognized JSON/YAML tokens.
type StructuredWriter struct {
	dst         io.Writer
	enabled     bool
	pending     []byte
	yamlPending []byte
	failed      bool
}

// NewStructuredWriter creates a writer. When enabled is false, writes are
// forwarded immediately and no buffering occurs.
func NewStructuredWriter(dst io.Writer, enabled bool) *StructuredWriter {
	return &StructuredWriter{dst: dst, enabled: enabled}
}

func (w *StructuredWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.dst == nil {
		return 0, errors.New("structured output destination is nil")
	}
	if !w.enabled {
		return w.dst.Write(p)
	}
	if w.failed {
		return 0, errors.New("structured output writer is closed after a write failure")
	}
	w.pending = append(w.pending, p...)
	if err := w.process(); err != nil {
		w.failed = true
		return 0, err
	}
	return len(p), nil
}

// Flush emits every undecided byte. It is called after the child process
// returns so an incomplete candidate cannot disappear from the terminal.
func (w *StructuredWriter) Flush() error {
	if !w.enabled || w.failed {
		if w.failed {
			return errors.New("structured output writer is closed after a write failure")
		}
		return nil
	}
	if w.yamlPending != nil {
		if len(w.pending) > 0 && likelyYAMLLine(w.pending) {
			w.yamlPending = append(w.yamlPending, w.pending...)
			w.pending = nil
		}
		if err := w.emitStructuredOrRaw(w.yamlPending); err != nil {
			w.failed = true
			return err
		}
		w.yamlPending = nil
	}
	if len(w.pending) > 0 {
		if err := w.emitStructuredOrRaw(w.pending); err != nil {
			w.failed = true
			return err
		}
		w.pending = nil
	}
	return nil
}

func (w *StructuredWriter) process() error {
	for len(w.pending) > 0 {
		if containsTerminalControl(w.pending) {
			if w.yamlPending != nil {
				if err := w.emit(w.yamlPending); err != nil {
					return err
				}
				w.yamlPending = nil
			}
			if err := w.emit(w.pending); err != nil {
				return err
			}
			w.pending = nil
			continue
		}

		if w.yamlPending != nil {
			lineEnd := bytes.IndexByte(w.pending, '\n')
			if lineEnd < 0 {
				if len(w.yamlPending)+len(w.pending) > MaxStructuredCandidateBytes {
					if err := w.emit(w.yamlPending); err != nil {
						return err
					}
					w.yamlPending = nil
					if err := w.emit(w.pending); err != nil {
						return err
					}
					w.pending = nil
					continue
				}
				if !likelyYAMLLine(w.pending) {
					if err := w.emitStructuredOrRaw(w.yamlPending); err != nil {
						return err
					}
					w.yamlPending = nil
					if err := w.emit(w.pending); err != nil {
						return err
					}
					w.pending = nil
				}
				return nil
			}
			line := append([]byte(nil), w.pending[:lineEnd+1]...)
			w.pending = w.pending[lineEnd+1:]
			if likelyYAMLLine(line) {
				w.yamlPending = append(w.yamlPending, line...)
				if len(w.yamlPending) > MaxStructuredCandidateBytes {
					if err := w.emit(w.yamlPending); err != nil {
						return err
					}
					w.yamlPending = nil
				}
				continue
			}
			if err := w.emitStructuredOrRaw(w.yamlPending); err != nil {
				return err
			}
			w.yamlPending = nil
			w.pending = append(line, w.pending...)
			continue
		}

		start := firstNonSpace(w.pending)
		if start >= 0 && (w.pending[start] == '{' || w.pending[start] == '[') {
			jsonInput := w.pending[start:]
			if len(jsonInput) > MaxStructuredCandidateBytes {
				// A truncated probe is enough to decide that a candidate which
				// has not closed within the bound must pass through. This keeps
				// the JSON decoder from retaining an unbounded RawMessage.
				jsonInput = jsonInput[:MaxStructuredCandidateBytes]
			}
			status, consumed, raw := decodeJSONPrefix(jsonInput)
			switch status {
			case jsonIncomplete:
				if len(w.pending) > MaxStructuredCandidateBytes {
					return w.emitAndClearPending()
				}
				return nil
			case jsonComplete:
				styled := colorizeJSON(raw)
				if err := w.emit(w.pending[:start]); err != nil {
					return err
				}
				if err := w.emit(styled); err != nil {
					return err
				}
				w.pending = w.pending[start+consumed:]
				continue
			case jsonInvalid:
				// Do not hold malformed JSON forever. A newline gives us a
				// natural output boundary; otherwise flush the bounded chunk.
				if lineEnd := bytes.IndexByte(w.pending, '\n'); lineEnd >= 0 {
					if err := w.emit(w.pending[:lineEnd+1]); err != nil {
						return err
					}
					w.pending = w.pending[lineEnd+1:]
					continue
				}
				return w.emitAndClearPending()
			}
		}

		lineEnd := bytes.IndexByte(w.pending, '\n')
		if lineEnd < 0 {
			if len(w.pending) > MaxStructuredCandidateBytes {
				return w.emitAndClearPending()
			}
			// A non-JSON partial line is ordinary interactive output until a
			// complete line proves otherwise. Emitting it now keeps shell
			// prompts and progress indicators responsive.
			return w.emitAndClearPending()
		}
		line := append([]byte(nil), w.pending[:lineEnd+1]...)
		w.pending = w.pending[lineEnd+1:]
		if likelyYAMLStart(line) {
			w.yamlPending = line
			continue
		}
		if err := w.emitStructuredOrRaw(line); err != nil {
			return err
		}
	}
	return nil
}

func (w *StructuredWriter) emitAndClearPending() error {
	if err := w.emit(w.pending); err != nil {
		return err
	}
	w.pending = nil
	return nil
}

func (w *StructuredWriter) emitStructuredOrRaw(data []byte) error {
	if styled, ok := ColorizeStructured(data); ok {
		return w.emit(styled)
	}
	return w.emit(data)
}

func (w *StructuredWriter) emit(data []byte) error {
	return writeAll(w.dst, data)
}

// ColorizeStructured returns a color-only projection when data is one
// complete, bounded, structurally useful JSON/YAML document. The original
// slice is returned unchanged when no safe projection applies.
func ColorizeStructured(data []byte) ([]byte, bool) {
	if len(data) == 0 || len(data) > MaxStructuredCandidateBytes || containsTerminalControl(data) {
		return data, false
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed) {
		return colorizeJSON(data), true
	}
	if validStructuredYAML(data) {
		return colorizeYAML(data), true
	}
	return data, false
}

type jsonDecodeStatus uint8

const (
	jsonInvalid jsonDecodeStatus = iota
	jsonIncomplete
	jsonComplete
)

func decodeJSONPrefix(data []byte) (jsonDecodeStatus, int, []byte) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		if isIncompleteJSONError(err) {
			return jsonIncomplete, 0, nil
		}
		return jsonInvalid, 0, nil
	}
	if len(raw) == 0 || (raw[0] != '{' && raw[0] != '[') || !json.Valid(raw) {
		return jsonInvalid, 0, nil
	}
	consumed := int(decoder.InputOffset())
	if consumed < len(raw) {
		consumed = len(raw)
	}
	return jsonComplete, consumed, raw
}

func isIncompleteJSONError(err error) bool {
	return errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "unexpected end of JSON input")
}

func colorizeJSON(data []byte) []byte {
	var output bytes.Buffer
	output.Grow(len(data) + len(data)/8)
	for index := 0; index < len(data); {
		current := data[index]
		switch {
		case current == ' ' || current == '\t' || current == '\r' || current == '\n':
			output.WriteByte(current)
			index++
		case current == '"':
			end := scanJSONString(data, index)
			if end <= index {
				output.WriteByte(current)
				index++
				continue
			}
			next := end
			for next < len(data) && (data[next] == ' ' || data[next] == '\t' || data[next] == '\r' || data[next] == '\n') {
				next++
			}
			style := structuredString
			if next < len(data) && data[next] == ':' {
				style = structuredKey
			}
			writeStyled(&output, style, data[index:end])
			index = end
		case isJSONPunctuation(current):
			writeStyled(&output, structuredPunctuation, data[index:index+1])
			index++
		case isJSONNumberStart(current):
			end := scanJSONAtom(data, index)
			writeStyled(&output, structuredNumber, data[index:end])
			index = end
		case hasJSONKeywordAt(data, index, "true") || hasJSONKeywordAt(data, index, "false") || hasJSONKeywordAt(data, index, "null"):
			end := scanJSONAtom(data, index)
			writeStyled(&output, structuredKeyword, data[index:end])
			index = end
		default:
			output.WriteByte(current)
			index++
		}
	}
	return output.Bytes()
}

func scanJSONString(data []byte, start int) int {
	for index := start + 1; index < len(data); index++ {
		if data[index] == '\\' {
			index++
			continue
		}
		if data[index] == '"' {
			return index + 1
		}
	}
	return start
}

func scanJSONAtom(data []byte, start int) int {
	for index := start; index < len(data); index++ {
		switch data[index] {
		case ' ', '\t', '\r', '\n', '{', '}', '[', ']', ',', ':':
			return index
		}
	}
	return len(data)
}

func isJSONNumberStart(value byte) bool {
	return value == '-' || value >= '0' && value <= '9'
}

func isJSONPunctuation(value byte) bool {
	switch value {
	case '{', '}', '[', ']', ',', ':':
		return true
	default:
		return false
	}
}

func hasJSONKeywordAt(data []byte, index int, keyword string) bool {
	if !bytes.HasPrefix(data[index:], []byte(keyword)) {
		return false
	}
	end := index + len(keyword)
	return end == len(data) || isJSONDelimiter(data[end])
}

func isJSONDelimiter(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n' || value == '{' || value == '}' || value == '[' || value == ']' || value == ',' || value == ':'
}

func validStructuredYAML(data []byte) bool {
	if !hasYAMLStructure(data) {
		return false
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil || document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return false
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode && root.Kind != yaml.SequenceNode {
		return false
	}
	if !safeYAMLNode(root) {
		return false
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return false
	}
	return true
}

func safeYAMLNode(node *yaml.Node) bool {
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return false
	}
	if node.Kind == yaml.ScalarNode {
		switch node.Tag {
		case "!!str", "!!int", "!!float", "!!bool", "!!null", "!!timestamp", "!!binary":
		default:
			return false
		}
	}
	for _, child := range node.Content {
		if !safeYAMLNode(child) {
			return false
		}
	}
	return true
}

func hasYAMLStructure(data []byte) bool {
	lines := bytes.Split(data, []byte{'\n'})
	nonEmpty := 0
	nested := false
	sequence := false
	documentMarker := false
	for _, line := range lines {
		trimmed := bytes.TrimSpace(bytes.TrimSuffix(line, []byte{'\r'}))
		if len(trimmed) == 0 || trimmed[0] == '#' {
			continue
		}
		nonEmpty++
		if bytes.Equal(trimmed, []byte("---")) || bytes.Equal(trimmed, []byte("...")) {
			documentMarker = true
		}
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			nested = true
		}
		if trimmed[0] == '-' && (len(trimmed) == 1 || trimmed[1] == ' ' || trimmed[1] == '\t') {
			sequence = true
		}
	}
	return nonEmpty >= 2 && (nested || sequence || documentMarker)
}

func colorizeYAML(data []byte) []byte {
	var output bytes.Buffer
	output.Grow(len(data) + len(data)/8)
	for start := 0; start < len(data); {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			output.Write(colorizeYAMLLine(data[start:]))
			break
		}
		end += start + 1
		output.Write(colorizeYAMLLine(data[start:end]))
		start = end
	}
	return output.Bytes()
}

func colorizeYAMLLine(line []byte) []byte {
	contentEnd := len(line)
	if contentEnd > 0 && line[contentEnd-1] == '\n' {
		contentEnd--
	}
	if contentEnd > 0 && line[contentEnd-1] == '\r' {
		contentEnd--
	}
	var output bytes.Buffer
	output.Grow(len(line) + len(line)/8)
	index := 0
	for index < contentEnd && (line[index] == ' ' || line[index] == '\t') {
		output.WriteByte(line[index])
		index++
	}
	if index < contentEnd && (bytes.Equal(line[index:contentEnd], []byte("---")) || bytes.Equal(line[index:contentEnd], []byte("..."))) {
		writeStyled(&output, structuredPunctuation, line[index:contentEnd])
		index = contentEnd
	}
	if index < contentEnd && line[index] == '-' && (index+1 == contentEnd || line[index+1] == ' ' || line[index+1] == '\t') {
		writeStyled(&output, structuredPunctuation, line[index:index+1])
		index++
		for index < contentEnd && (line[index] == ' ' || line[index] == '\t') {
			output.WriteByte(line[index])
			index++
		}
	}
	if colon := yamlMappingColon(line, index, contentEnd); colon >= 0 {
		writeStyled(&output, structuredKey, line[index:colon])
		writeStyled(&output, structuredPunctuation, line[colon:colon+1])
		index = colon + 1
	}
	colorizeYAMLValue(&output, line, index, contentEnd)
	if contentEnd < len(line) {
		output.Write(line[contentEnd:])
	}
	return output.Bytes()
}

func yamlMappingColon(line []byte, start, end int) int {
	inSingle, inDouble, escaped := false, false, false
	for index := start; index < end; index++ {
		value := line[index]
		if inDouble {
			if escaped {
				escaped = false
			} else if value == '\\' {
				escaped = true
			} else if value == '"' {
				inDouble = false
			}
			continue
		}
		if inSingle {
			if value == '\'' {
				if index+1 < end && line[index+1] == '\'' {
					index++
				} else {
					inSingle = false
				}
			}
			continue
		}
		switch value {
		case '"':
			inDouble = true
		case '\'':
			inSingle = true
		case ':':
			if index+1 == end || line[index+1] == ' ' || line[index+1] == '\t' || line[index+1] == '#' {
				return index
			}
		}
	}
	return -1
}

func colorizeYAMLValue(output *bytes.Buffer, line []byte, start, end int) {
	for index := start; index < end; {
		if line[index] == ' ' || line[index] == '\t' {
			output.WriteByte(line[index])
			index++
			continue
		}
		if line[index] == '#' {
			writeStyled(output, structuredComment, line[index:end])
			return
		}
		if line[index] == '"' || line[index] == '\'' {
			quote := line[index]
			valueEnd := index + 1
			for valueEnd < end {
				if line[valueEnd] == quote && (quote == '\'' || valueEnd == index+1 || line[valueEnd-1] != '\\') {
					valueEnd++
					break
				}
				valueEnd++
			}
			writeStyled(output, structuredString, line[index:valueEnd])
			index = valueEnd
			continue
		}
		valueEnd := index
		for valueEnd < end && line[valueEnd] != ' ' && line[valueEnd] != '\t' {
			valueEnd++
		}
		token := line[index:valueEnd]
		style := structuredString
		if isYAMLKeyword(token) {
			style = structuredKeyword
		} else if isYAMLNumber(token) {
			style = structuredNumber
		}
		writeStyled(output, style, token)
		index = valueEnd
	}
}

func isYAMLKeyword(token []byte) bool {
	switch strings.ToLower(string(token)) {
	case "true", "false", "null", "~":
		return true
	default:
		return false
	}
}

func isYAMLNumber(token []byte) bool {
	if len(token) == 0 {
		return false
	}
	if _, err := strconv.ParseFloat(string(token), 64); err == nil {
		return true
	}
	return false
}

func likelyYAMLStart(line []byte) bool {
	trimmed := strings.TrimSpace(string(bytes.TrimSuffix(line, []byte{'\r', '\n'})))
	if trimmed == "---" || strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
		return true
	}
	return yamlLineHasMapping(trimmed)
}

func likelyYAMLLine(line []byte) bool {
	trimmedBytes := bytes.TrimSpace(bytes.TrimSuffix(line, []byte{'\r', '\n'}))
	if len(trimmedBytes) == 0 || trimmedBytes[0] == '#' || bytes.Equal(trimmedBytes, []byte("---")) || bytes.Equal(trimmedBytes, []byte("...")) {
		return true
	}
	if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
		return true
	}
	return bytes.HasPrefix(trimmedBytes, []byte("- ")) || bytes.Equal(trimmedBytes, []byte("-")) || yamlLineHasMapping(string(trimmedBytes))
}

func yamlLineHasMapping(line string) bool {
	colon := yamlMappingColon([]byte(line), 0, len(line))
	return colon > 0
}

func firstNonSpace(data []byte) int {
	for index, value := range data {
		if value != ' ' && value != '\t' && value != '\r' && value != '\n' {
			return index
		}
	}
	return -1
}

func containsTerminalControl(data []byte) bool {
	if !utf8.Valid(data) {
		return true
	}
	for index, value := range data {
		switch {
		case value == 0x1b || value == 0x7f:
			return true
		case value < 0x20 && value != '\t' && value != '\n' && value != '\r':
			return true
		case value == 0xc2 && index+1 < len(data) && data[index+1] >= 0x80 && data[index+1] <= 0x9f:
			return true
		}
	}
	return false
}

func writeStyled(output *bytes.Buffer, style string, value []byte) {
	if len(value) == 0 {
		return
	}
	output.WriteString(style)
	output.Write(value)
	output.WriteString(structuredReset)
}

func writeAll(output io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := output.Write(data)
		if written < 0 || written > len(data) {
			return io.ErrShortWrite
		}
		data = data[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
