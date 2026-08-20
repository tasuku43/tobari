package terminalstyle

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestColorizeStructuredJSONPreservesVisibleBytes(t *testing.T) {
	input := []byte("{\n  \"error\": \"policy_denied\",\n  \"status\": 403,\n  \"retry\": false,\n  \"empty\": [],\n  \"nothing\": null\n}\n")

	got, ok := ColorizeStructured(input)
	if !ok {
		t.Fatal("ColorizeStructured() did not recognize JSON")
	}
	if visible := stripSGR(got); !bytes.Equal(visible, input) {
		t.Fatalf("visible JSON changed:\n got: %q\nwant: %q", visible, input)
	}
	for _, style := range []string{
		structuredPunctuation,
		structuredKey,
		structuredString,
		structuredNumber,
		structuredKeyword,
	} {
		if !bytes.Contains(got, []byte(style)) {
			t.Fatalf("JSON output is missing style %q: %q", style, got)
		}
	}
}

func TestStructuredWriterColorizesFragmentedJSONAndFlushesPendingBytes(t *testing.T) {
	input := []byte("{\n  \"ok\": true,\n  \"message\": \"done\"\n}\nplain output\n")
	var output bytes.Buffer
	writer := NewStructuredWriter(&output, true)
	for _, chunk := range [][]byte{
		input[:7],
		input[7:18],
		input[18:31],
		input[31 : len(input)-4],
		input[len(input)-4:],
	} {
		written, err := writer.Write(chunk)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if written != len(chunk) {
			t.Fatalf("Write() bytes = %d, want %d", written, len(chunk))
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if visible := stripSGR(output.Bytes()); !bytes.Equal(visible, input) {
		t.Fatalf("visible fragmented output changed:\n got: %q\nwant: %q", visible, input)
	}
	if !bytes.Contains(output.Bytes(), []byte(structuredKey)) {
		t.Fatalf("fragmented JSON was not colored: %q", output.Bytes())
	}
}

func TestColorizeStructuredYAMLPreservesVisibleBytes(t *testing.T) {
	input := []byte("name: demo\nitems:\n  - id: 3\n    enabled: true\n    label: \"ok\"\n# synthetic fixture\n")

	got, ok := ColorizeStructured(input)
	if !ok {
		t.Fatal("ColorizeStructured() did not recognize structured YAML")
	}
	if visible := stripSGR(got); !bytes.Equal(visible, input) {
		t.Fatalf("visible YAML changed:\n got: %q\nwant: %q", visible, input)
	}
	for _, style := range []string{
		structuredPunctuation,
		structuredKey,
		structuredString,
		structuredNumber,
		structuredKeyword,
		structuredComment,
	} {
		if !bytes.Contains(got, []byte(style)) {
			t.Fatalf("YAML output is missing style %q: %q", style, got)
		}
	}
}

func TestColorizeStructuredRejectsAmbiguousOrUnsafeYAML(t *testing.T) {
	tests := map[string][]byte{
		"ordinary colon log": []byte("request: failed\nnext line\n"),
		"one line mapping":   []byte("request: failed\n"),
		"alias":              []byte("base: &base\n  name: demo\ncopy: *base\n"),
		"custom tag":         []byte("value: !secret demo\nnested:\n  ok: true\n"),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := ColorizeStructured(input)
			if ok {
				t.Fatalf("ColorizeStructured() accepted unsafe or ambiguous YAML: %q", got)
			}
			if !bytes.Equal(got, input) {
				t.Fatalf("rejected YAML changed: got %q, want %q", got, input)
			}
		})
	}
}

func TestColorizeStructuredRejectsControlsButAcceptsEscapedControlText(t *testing.T) {
	escaped := []byte("{\"message\":\"\\u001b[2J\"}\n")
	styled, ok := ColorizeStructured(escaped)
	if !ok || !bytes.Contains(styled, []byte(structuredString)) {
		t.Fatalf("escaped control text was not safely colored: %q", styled)
	}
	if visible := stripSGR(styled); !bytes.Equal(visible, escaped) {
		t.Fatalf("escaped control text changed: got %q, want %q", visible, escaped)
	}

	for name, actual := range map[string][]byte{
		"escape": []byte("{\"message\":\"ok\"}\x1b[2J\n"),
		"c1":     []byte("{\"message\":\"ok\"}\x9b2J\n"),
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := ColorizeStructured(actual)
			if ok || !bytes.Equal(got, actual) {
				t.Fatalf("actual terminal control was not passed through unchanged: ok=%t got=%q", ok, got)
			}
		})
	}
}

func TestStructuredWriterPassesThroughIncompleteAndOversizedCandidates(t *testing.T) {
	incomplete := []byte("{\"ok\": true\n")
	oversized := append([]byte("{\"message\":\""), bytes.Repeat([]byte{'x'}, MaxStructuredCandidateBytes)...)
	oversized = append(oversized, []byte("\"}\n")...)

	for name, input := range map[string][]byte{
		"incomplete": incomplete,
		"oversized":  oversized,
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			writer := NewStructuredWriter(&output, true)
			if _, err := writer.Write(input); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if err := writer.Flush(); err != nil {
				t.Fatalf("Flush() error = %v", err)
			}
			if !bytes.Equal(output.Bytes(), input) {
				t.Fatalf("pass-through changed bytes: got %d, want %d", output.Len(), len(input))
			}
			if bytes.Contains(output.Bytes(), []byte("\x1b[")) {
				t.Fatal("pass-through unexpectedly added ANSI")
			}
		})
	}
}

func TestStructuredWriterReportsDestinationFailure(t *testing.T) {
	writer := NewStructuredWriter(failingWriter{}, true)
	if _, err := writer.Write([]byte("{\"ok\":true}\n")); !errors.Is(err, errWriteFailed) {
		t.Fatalf("Write() error = %v, want %v", err, errWriteFailed)
	}
	if _, err := writer.Write([]byte("later\n")); err == nil {
		t.Fatal("Write() after destination failure unexpectedly succeeded")
	}
}

var errWriteFailed = errors.New("synthetic destination failure")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

func stripSGR(data []byte) []byte {
	var output bytes.Buffer
	for index := 0; index < len(data); {
		if data[index] == 0x1b && index+2 < len(data) && data[index+1] == '[' {
			if end := bytes.IndexByte(data[index+2:], 'm'); end >= 0 {
				index += end + 3
				continue
			}
		}
		output.WriteByte(data[index])
		index++
	}
	return output.Bytes()
}

func TestStructuredWriterDisabledIsImmediatePassThrough(t *testing.T) {
	var output bytes.Buffer
	writer := NewStructuredWriter(&output, false)
	input := []byte("{\"ok\":true}\n")
	if _, err := writer.Write(input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[") || !bytes.Equal(output.Bytes(), input) {
		t.Fatalf("disabled writer changed output: %q", output.Bytes())
	}
}

func TestStructuredWriterDoesNotHoldInteractivePromptText(t *testing.T) {
	var output bytes.Buffer
	writer := NewStructuredWriter(&output, true)
	if _, err := writer.Write([]byte("tobari:~/work$ ")); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "tobari:~/work$ " {
		t.Fatalf("prompt was buffered: got %q", got)
	}

	if _, err := writer.Write([]byte("items:\n  - one\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("tobari$ ")); err != nil {
		t.Fatal(err)
	}
	if visible := stripSGR(output.Bytes()); !bytes.Contains(visible, []byte("tobari$ ")) {
		t.Fatalf("prompt after YAML was buffered: %q", visible)
	}
}
