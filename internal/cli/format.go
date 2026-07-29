package cli

import (
	"fmt"
	"strings"
	"unicode"
)

type successFormat string

const (
	successFormatTSV  successFormat = "tsv"
	successFormatJSON successFormat = "json"
)

func parseSuccessFormat(value string) (successFormat, error) {
	switch successFormat(value) {
	case successFormatTSV:
		return successFormatTSV, nil
	case successFormatJSON:
		return successFormatJSON, nil
	default:
		return successFormatTSV, fmt.Errorf("--format must be tsv or json")
	}
}

// safeExternalText makes structural runes visible without interpreting the
// remaining text. Backslashes are escaped first so a literal sequence such as
// \n stays distinguishable from a projected newline. Opaque IDs bypass this
// projection and must instead pass their domain validator byte-for-byte.
func safeExternalText(value string) string {
	var output strings.Builder
	for _, r := range value {
		writeExternalRune(&output, r, true)
	}
	return output.String()
}

func escapeTSVCell(value string) string {
	var output strings.Builder
	for _, r := range value {
		writeExternalRune(&output, r, true)
	}
	return output.String()
}

func writeExternalRune(output *strings.Builder, r rune, escapeBackslash bool) {
	if escapeBackslash && r == '\\' {
		output.WriteString("\\\\")
		return
	}
	if r == '\u2028' || r == '\u2029' {
		fmt.Fprintf(output, "\\u%04X", r)
		return
	}
	if unicode.Is(unicode.C, r) {
		switch r {
		case '\t':
			output.WriteString("\\t")
		case '\r':
			output.WriteString("\\r")
		case '\n':
			output.WriteString("\\n")
		default:
			if r <= 0xffff {
				fmt.Fprintf(output, "\\u%04X", r)
			} else {
				fmt.Fprintf(output, "\\U%08X", r)
			}
		}
		return
	}
	output.WriteRune(r)
}
