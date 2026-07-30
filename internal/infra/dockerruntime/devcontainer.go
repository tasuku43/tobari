package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const maxDevContainerBytes = 256 * 1024

// ReadDevContainer reads one explicit image-based definition below a Tobari root.
func (r *Runtime) ReadDevContainer(
	ctx context.Context, root, value string,
) (tobari.DevContainerConfig, error) {
	if err := ctx.Err(); err != nil {
		return tobari.DevContainerConfig{}, err
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return tobari.DevContainerConfig{}, fmt.Errorf("Tobari root must be canonical and absolute")
	}
	if value == "" {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container path is required")
	}
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("make Dev Container path absolute: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("resolve Dev Container path: %w", err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container path must be inside the selected root")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("inspect Dev Container path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container path must be a regular file")
	}
	if info.Size() > maxDevContainerBytes {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container file exceeds %d bytes", maxDevContainerBytes)
	}
	file, err := os.Open(resolved) // #nosec G304 -- canonical path is constrained below the selected root.
	if err != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("open Dev Container file: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxDevContainerBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("read Dev Container file: %w", readErr)
	}
	if closeErr != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("close Dev Container file: %w", closeErr)
	}
	if len(data) > maxDevContainerBytes {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container file exceeds %d bytes", maxDevContainerBytes)
	}
	if err := ctx.Err(); err != nil {
		return tobari.DevContainerConfig{}, err
	}
	return parseDevContainer(data)
}

func parseDevContainer(data []byte) (tobari.DevContainerConfig, error) {
	withoutComments, err := stripJSONComments(data)
	if err != nil {
		return tobari.DevContainerConfig{}, err
	}
	normalized := stripJSONTrailingCommas(withoutComments)
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	token, err := decoder.Token()
	if err != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("decode Dev Container object: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container document must be an object")
	}
	properties := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return tobari.DevContainerConfig{}, fmt.Errorf("decode Dev Container property: %w", err)
		}
		name, ok := token.(string)
		if !ok {
			return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container property name is invalid")
		}
		if _, duplicate := properties[name]; duplicate {
			return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container property %q is duplicated", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return tobari.DevContainerConfig{}, fmt.Errorf("decode Dev Container property %q: %w", name, err)
		}
		properties[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return tobari.DevContainerConfig{}, fmt.Errorf("close Dev Container object: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container document contains trailing data")
	}
	rawImage, exists := properties["image"]
	var image string
	if exists {
		if err := json.Unmarshal(rawImage, &image); err != nil {
			return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container image must be a string")
		}
	}
	if exists && image == "" {
		return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container image must be a string")
	}
	for _, name := range []string{"$schema", "name"} {
		if raw, present := properties[name]; present {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container %s must be a string", name)
			}
		}
	}
	if raw, present := properties["customizations"]; present {
		var value map[string]json.RawMessage
		if err := json.Unmarshal(raw, &value); err != nil || value == nil {
			return tobari.DevContainerConfig{}, fmt.Errorf("Dev Container customizations must be an object")
		}
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return tobari.DevContainerConfig{Image: image, Properties: names}, nil
}

func stripJSONComments(data []byte) ([]byte, error) {
	output := append([]byte{}, data...)
	inString, escaped := false, false
	for index := 0; index < len(output); index++ {
		current := output[index]
		if inString {
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == '"' {
				inString = false
			}
			continue
		}
		if current == '"' {
			inString = true
			continue
		}
		if current != '/' || index+1 >= len(output) {
			continue
		}
		switch output[index+1] {
		case '/':
			output[index], output[index+1] = ' ', ' '
			index += 2
			for index < len(output) && output[index] != '\n' && output[index] != '\r' {
				output[index] = ' '
				index++
			}
			index--
		case '*':
			output[index], output[index+1] = ' ', ' '
			index += 2
			closed := false
			for index < len(output) {
				if index+1 < len(output) && output[index] == '*' && output[index+1] == '/' {
					output[index], output[index+1] = ' ', ' '
					index++
					closed = true
					break
				}
				if output[index] != '\n' && output[index] != '\r' {
					output[index] = ' '
				}
				index++
			}
			if !closed {
				return nil, fmt.Errorf("Dev Container block comment is not closed")
			}
		}
	}
	return output, nil
}

func stripJSONTrailingCommas(data []byte) []byte {
	output := append([]byte{}, data...)
	inString, escaped := false, false
	for index := 0; index < len(output); index++ {
		current := output[index]
		if inString {
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == '"' {
				inString = false
			}
			continue
		}
		if current == '"' {
			inString = true
			continue
		}
		if current != ',' {
			continue
		}
		next := index + 1
		for next < len(output) {
			switch output[next] {
			case ' ', '\t', '\r', '\n':
				next++
			default:
				goto found
			}
		}
	found:
		if next < len(output) && (output[next] == '}' || output[next] == ']') {
			output[index] = ' '
		}
	}
	return output
}
