package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type expectedEntry struct {
	name     string
	mode     fs.FileMode
	contents []byte
}

func TestPackProducesDeterministicCanonicalMultiEntryArchives(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "tool")
	license := filepath.Join(root, "license")
	notice := filepath.Join(root, "notices")
	executableContents := bytes.Repeat([]byte("deterministic release bytes\n"), 4096)
	licenseContents := []byte("project license\n")
	noticeContents := []byte("dependency notices\n")
	writeTestFile(t, executable, executableContents, executableMode)
	writeTestFile(t, license, licenseContents, supportingFileMode)
	writeTestFile(t, notice, noticeContents, supportingFileMode)

	want := []expectedEntry{
		{name: "LICENSE", mode: supportingFileMode, contents: licenseContents},
		{name: "THIRD_PARTY_NOTICES", mode: supportingFileMode, contents: noticeContents},
		{name: "tool", mode: executableMode, contents: executableContents},
	}
	forward := []inputSpec{
		{path: executable, name: "tool", mode: executableMode},
		{path: notice, name: "THIRD_PARTY_NOTICES", mode: supportingFileMode},
		{path: license, name: "LICENSE", mode: supportingFileMode},
	}
	reverse := []inputSpec{forward[2], forward[1], forward[0]}

	for _, archiveFormat := range []string{"tar.gz", "zip"} {
		t.Run(archiveFormat, func(t *testing.T) {
			first := filepath.Join(root, "first."+archiveFormat)
			second := filepath.Join(root, "second."+archiveFormat)
			if err := pack(archiveFormat, first, forward); err != nil {
				t.Fatal(err)
			}
			if err := pack(archiveFormat, second, reverse); err != nil {
				t.Fatal(err)
			}
			firstBytes := readTestFile(t, first)
			secondBytes := readTestFile(t, second)
			if !bytes.Equal(firstBytes, secondBytes) {
				t.Fatal("equivalent inputs in different orders produced different archive bytes")
			}
			info, err := os.Stat(first)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != archiveOutputMode {
				t.Fatalf("archive mode = %04o, want %04o", got, archiveOutputMode)
			}
			if err := verifyArchive(archiveFormat, first, reverse); err != nil {
				t.Fatalf("verifyArchive() rejected canonical archive: %v", err)
			}

			switch archiveFormat {
			case "tar.gz":
				assertCanonicalTarGzip(t, first, want)
			case "zip":
				assertCanonicalZip(t, first, want)
			}
		})
	}
}

func TestVerifyArchiveRejectsWrongMode(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "tool")
	writeTestFile(t, executable, []byte("binary"), executableMode)
	for _, archiveFormat := range []string{"tar.gz", "zip"} {
		t.Run(archiveFormat, func(t *testing.T) {
			archive := filepath.Join(root, "wrong-mode."+archiveFormat)
			if err := pack(archiveFormat, archive, oneInput(executable, "tool", supportingFileMode)); err != nil {
				t.Fatal(err)
			}
			err := verifyArchive(archiveFormat, archive, oneInput(executable, "tool", executableMode))
			if err == nil || !strings.Contains(err.Error(), "mode") {
				t.Fatalf("verifyArchive() error = %v, want mode mismatch", err)
			}
		})
	}
}

func TestVerifyArchiveRejectsChangedReviewedInput(t *testing.T) {
	for _, archiveFormat := range []string{"tar.gz", "zip"} {
		t.Run(archiveFormat, func(t *testing.T) {
			root := t.TempDir()
			input := filepath.Join(root, "license")
			writeTestFile(t, input, []byte("reviewed\n"), supportingFileMode)
			archive := filepath.Join(root, "license."+archiveFormat)
			if err := pack(archiveFormat, archive, oneInput(input, "LICENSE", supportingFileMode)); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, input, []byte("changed!\n"), supportingFileMode)
			err := verifyArchive(archiveFormat, archive, oneInput(input, "LICENSE", supportingFileMode))
			if err == nil || (!strings.Contains(err.Error(), "size") && !strings.Contains(err.Error(), "content")) {
				t.Fatalf("verifyArchive() error = %v, want content mismatch", err)
			}
		})
	}
}

func TestVerifyArchiveRejectsUnexpectedEntries(t *testing.T) {
	root := t.TempDir()
	license := filepath.Join(root, "license")
	notice := filepath.Join(root, "notices")
	writeTestFile(t, license, []byte("license\n"), supportingFileMode)
	writeTestFile(t, notice, []byte("notices\n"), supportingFileMode)
	all := []inputSpec{
		{path: license, name: "LICENSE", mode: supportingFileMode},
		{path: notice, name: "THIRD_PARTY_NOTICES", mode: supportingFileMode},
	}

	for _, archiveFormat := range []string{"tar.gz", "zip"} {
		t.Run(archiveFormat, func(t *testing.T) {
			archive := filepath.Join(root, "unexpected."+archiveFormat)
			if err := pack(archiveFormat, archive, all); err != nil {
				t.Fatal(err)
			}
			err := verifyArchive(archiveFormat, archive, all[:1])
			if err == nil || (!strings.Contains(err.Error(), "unexpected") && !strings.Contains(err.Error(), "entry count")) {
				t.Fatalf("verifyArchive() error = %v, want unexpected-entry rejection", err)
			}
		})
	}
}

func TestVerifyArchiveRejectsNonCanonicalHeaders(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "tool")
	writeTestFile(t, input, []byte("binary"), executableMode)
	specs := oneInput(input, "tool", executableMode)

	t.Run("tar.gz", func(t *testing.T) {
		archive := filepath.Join(root, "noncanonical.tar.gz")
		writeNonCanonicalTarGzip(t, archive, "tool", []byte("binary"), executableMode)
		err := verifyArchive("tar.gz", archive, specs)
		if err == nil || !strings.Contains(err.Error(), "gzip header") {
			t.Fatalf("verifyArchive() error = %v, want gzip header rejection", err)
		}
	})

	t.Run("zip", func(t *testing.T) {
		archive := filepath.Join(root, "noncanonical.zip")
		writeNonCanonicalZip(t, archive, "tool", []byte("binary"), executableMode)
		err := verifyArchive("zip", archive, specs)
		if err == nil || !strings.Contains(err.Error(), "header") {
			t.Fatalf("verifyArchive() error = %v, want zip header rejection", err)
		}
	})
}

func TestVerifyTarGzipRejectsTrailingData(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "license")
	writeTestFile(t, input, []byte("license\n"), supportingFileMode)
	archive := filepath.Join(root, "trailing.tar.gz")
	specs := oneInput(input, "LICENSE", supportingFileMode)
	if err := pack("tar.gz", archive, specs); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("unexpected")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchive("tar.gz", archive, specs); err == nil || !strings.Contains(err.Error(), "after its canonical gzip member") {
		t.Fatalf("verifyArchive() error = %v, want trailing-data rejection", err)
	}
}

func TestVerifyTarGzipRejectsUncompressedDataAfterTarTerminator(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "license")
	writeTestFile(t, input, []byte("license\n"), supportingFileMode)
	archive := filepath.Join(root, "inner-trailing.tar.gz")
	writeTarGzipWithInnerTrailingData(t, archive, "LICENSE", []byte("license\n"), supportingFileMode)
	err := verifyArchive("tar.gz", archive, oneInput(input, "LICENSE", supportingFileMode))
	if err == nil || !strings.Contains(err.Error(), "after the canonical tar terminator") {
		t.Fatalf("verifyArchive() error = %v, want inner trailing-data rejection", err)
	}
}

func TestVerifyTarGzipRejectsRawPaddingHeaderAndTerminatorDrift(t *testing.T) {
	mutations := map[string]func([]byte) []byte{
		"nonzero entry padding": func(payload []byte) []byte {
			payload[512+len("binary")] = 1
			return payload
		},
		"semantically ignored header byte": func(payload []byte) []byte {
			payload[len("tool")+1] = ' '
			rewriteTestTarChecksum(t, payload[:tarBlockSize])
			return payload
		},
		"one terminator block": func(payload []byte) []byte {
			return payload[:len(payload)-tarBlockSize]
		},
		"no terminator blocks": func(payload []byte) []byte {
			return payload[:len(payload)-2*tarBlockSize]
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			input := filepath.Join(root, "tool")
			archive := filepath.Join(root, "tool.tar.gz")
			writeTestFile(t, input, []byte("binary"), executableMode)
			specs := oneInput(input, "tool", executableMode)
			if err := pack("tar.gz", archive, specs); err != nil {
				t.Fatal(err)
			}
			rewriteTestTarGzipPayload(t, archive, mutate)
			if err := verifyArchive("tar.gz", archive, specs); err == nil {
				t.Fatalf("verifyArchive() accepted %s", name)
			}
		})
	}
}

func TestVerifyZipRejectsNonCanonicalRawLayout(t *testing.T) {
	for _, mutation := range []string{"prefix", "trailing", "local extra", "creator version"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			input := filepath.Join(root, "license")
			archive := filepath.Join(root, "release.zip")
			writeTestFile(t, input, []byte("license\n"), supportingFileMode)
			specs := oneInput(input, "LICENSE", supportingFileMode)
			if err := pack("zip", archive, specs); err != nil {
				t.Fatal(err)
			}
			data := readTestFile(t, archive)
			switch mutation {
			case "prefix":
				data = append([]byte("stub"), data...)
			case "trailing":
				data = append(data, []byte("trailing")...)
			case "local extra":
				nameLength := int(binary.LittleEndian.Uint16(data[26:28]))
				extraOffset := 30 + nameLength
				data[extraOffset] ^= 0x01
			case "creator version":
				endOffset := len(data) - 22
				centralOffset := int(binary.LittleEndian.Uint32(data[endOffset+16 : endOffset+20]))
				data[centralOffset+4] ^= 0x01
			}
			if err := os.WriteFile(archive, data, archiveOutputMode); err != nil {
				t.Fatal(err)
			}
			if err := verifyArchive("zip", archive, specs); err == nil {
				t.Fatalf("verifyArchive() accepted zip %s drift", mutation)
			}
		})
	}
}

func TestVerifyArchiveRejectsSymbolicLinks(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "license")
	writeTestFile(t, input, []byte("reviewed\n"), supportingFileMode)
	archive := filepath.Join(root, "license.zip")
	if err := pack("zip", archive, oneInput(input, "LICENSE", supportingFileMode)); err != nil {
		t.Fatal(err)
	}
	archiveLink := filepath.Join(root, "linked.zip")
	inputLink := filepath.Join(root, "linked-license")
	if err := os.Symlink(archive, archiveLink); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := os.Symlink(input, inputLink); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := verifyArchive("zip", archiveLink, oneInput(input, "LICENSE", supportingFileMode)); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("verifyArchive() accepted symbolic-link archive: %v", err)
	}
	if err := verifyArchive("zip", archive, oneInput(inputLink, "LICENSE", supportingFileMode)); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("verifyArchive() accepted symbolic-link reviewed input: %v", err)
	}
}

func TestPackRefusesExistingOutput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "tool")
	output := filepath.Join(root, "tool.tar.gz")
	writeTestFile(t, input, []byte("binary"), executableMode)
	writeTestFile(t, output, []byte("keep"), 0o600)
	if err := pack("tar.gz", output, oneInput(input, "tool", executableMode)); err == nil || !strings.Contains(err.Error(), "without overwrite") {
		t.Fatalf("pack() error = %v", err)
	}
	if data := readTestFile(t, output); string(data) != "keep" {
		t.Fatalf("existing output changed: %q", data)
	}
}

func TestPackRefusesSymbolicLinkOutputWithoutChangingTarget(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "tool")
	external := filepath.Join(root, "external")
	output := filepath.Join(root, "tool.zip")
	writeTestFile(t, input, []byte("binary"), executableMode)
	writeTestFile(t, external, []byte("keep"), 0o600)
	if err := os.Symlink(external, output); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := pack("zip", output, oneInput(input, "tool", executableMode)); err == nil || !strings.Contains(err.Error(), "without overwrite") {
		t.Fatalf("pack() error = %v", err)
	}
	if data := readTestFile(t, external); string(data) != "keep" {
		t.Fatalf("symbolic-link target changed: %q", data)
	}
}

func TestCreateArchiveRemovesPartialOutputOnLaterEntryFailure(t *testing.T) {
	entries := []archiveEntry{
		{input: strings.NewReader("complete"), name: "a", size: 8, mode: supportingFileMode},
		{input: strings.NewReader("short"), name: "b", size: 100, mode: executableMode},
	}
	for _, archiveFormat := range []string{"tar.gz", "zip"} {
		t.Run(archiveFormat, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "partial."+archiveFormat)
			err := createArchive(archiveFormat, entries, output)
			if err == nil || !strings.Contains(err.Error(), "size changed") {
				t.Fatalf("createArchive() error = %v", err)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("partial output remains: %v", err)
			}
		})
	}
}

func TestPackRejectsUnsafeEntryNamesBeforeCreatingOutput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "tool")
	writeTestFile(t, input, []byte("binary"), executableMode)
	if err := validateEntryName(strings.Repeat("a", 96) + ".exe"); err != nil {
		t.Fatalf("maximum configured Windows executable name was rejected: %v", err)
	}
	unsafeNames := []string{
		"", ".", "..", "../tool", "/absolute/tool", "dir/tool", `dir\tool`,
		"bad\x00name", "bad\nname", string([]byte{0xff}), "CON", "nul.txt", "COM1.log",
		"tool:", "trailing.", "with space", "日本語", "bad\u202ename", strings.Repeat("a", 101),
	}
	for formatIndex, archiveFormat := range []string{"tar.gz", "zip"} {
		for entryIndex, entry := range unsafeNames {
			output := filepath.Join(root, fmt.Sprintf("unsafe-%d-%d.archive", formatIndex, entryIndex))
			if err := pack(archiveFormat, output, oneInput(input, entry, executableMode)); err == nil {
				t.Fatalf("pack(%q) accepted entry %q", archiveFormat, entry)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("pack(%q) created output for entry %q: %v", archiveFormat, entry, err)
			}
		}
	}
	output := filepath.Join(root, "tool.rar")
	if err := pack("rar", output, oneInput(input, "tool", executableMode)); err == nil {
		t.Fatal("pack() accepted an unsupported format")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("pack() created output for an unsupported format: %v", err)
	}
}

func TestPackRejectsDuplicateNamesAndUnsupportedModesBeforeCreatingOutput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "tool")
	writeTestFile(t, input, []byte("binary"), executableMode)
	tests := []struct {
		name  string
		specs []inputSpec
		want  string
	}{
		{
			name: "duplicate",
			specs: []inputSpec{
				{path: input, name: "tool", mode: executableMode},
				{path: input, name: "tool", mode: supportingFileMode},
			},
			want: "duplicate",
		},
		{
			name: "case insensitive duplicate",
			specs: []inputSpec{
				{path: input, name: "LICENSE", mode: supportingFileMode},
				{path: input, name: "license", mode: executableMode},
			},
			want: "case-insensitive filesystem",
		},
		{
			name:  "unsupported mode",
			specs: oneInput(input, "tool", 0o600),
			want:  "unsupported archive entry mode",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(root, strings.ReplaceAll(test.name, " ", "-")+".zip")
			err := pack("zip", output, test.specs)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("pack() error = %v, want text %q", err, test.want)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("pack() created output: %v", err)
			}
		})
	}
}

func TestPackRejectsNonRegularInputs(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "tool.tar.gz")
	if err := pack("tar.gz", output, oneInput(root, "tool", executableMode)); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("pack() directory error = %v", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("pack() created output for a directory: %v", err)
	}

	input := filepath.Join(root, "tool")
	link := filepath.Join(root, "linked-tool")
	writeTestFile(t, input, []byte("binary"), executableMode)
	if err := os.Symlink(input, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := pack("zip", output, oneInput(link, "tool", executableMode)); err == nil || !strings.Contains(err.Error(), "without a symbolic link") {
		t.Fatalf("pack() symbolic-link error = %v", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("pack() created output for a symbolic link: %v", err)
	}
}

func TestCreateArchiveRejectsInvalidEntrySetsBeforeCreatingOutput(t *testing.T) {
	tests := []struct {
		name    string
		entries []archiveEntry
		want    string
	}{
		{name: "empty", want: "at least one"},
		{
			name:    "missing reader",
			entries: []archiveEntry{{name: "tool", size: 1, mode: executableMode}},
			want:    "no input",
		},
		{
			name:    "negative size",
			entries: []archiveEntry{{input: strings.NewReader(""), name: "tool", size: -1, mode: executableMode}},
			want:    "negative size",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "invalid.tar.gz")
			err := createArchive("tar.gz", test.entries, output)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("createArchive() error = %v, want %q", err, test.want)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("createArchive() created invalid output: %v", err)
			}
		})
	}
}

func TestParseInputSpecsAcceptsOnlyCompleteCanonicalTriples(t *testing.T) {
	specs, err := parseInputSpecs([]string{"bin", "tool", "0755", "license", "LICENSE", "0644"})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].mode != executableMode || specs[1].mode != supportingFileMode {
		t.Fatalf("parseInputSpecs() = %#v", specs)
	}
	for _, arguments := range [][]string{
		nil,
		{"bin", "tool"},
		{"bin", "tool", "755"},
		{"bin", "tool", "0600"},
		{"bin", "tool", "0777"},
	} {
		if _, err := parseInputSpecs(arguments); err == nil {
			t.Fatalf("parseInputSpecs(%q) succeeded", arguments)
		}
	}
}

func oneInput(path, name string, mode fs.FileMode) []inputSpec {
	return []inputSpec{{path: path, name: name, mode: mode}}
}

func writeTestFile(t *testing.T, path string, contents []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func writeNonCanonicalTarGzip(t *testing.T, path, name string, contents []byte, mode fs.FileMode) {
	t.Helper()
	output, err := os.Create(path) // #nosec G304 -- the test owns its temporary path.
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(output) // Deliberately leaves the non-canonical zero modification time.
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name, Mode: int64(mode), Size: int64(len(contents)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTarGzipWithInnerTrailingData(t *testing.T, path, name string, contents []byte, mode fs.FileMode) {
	t.Helper()
	output, err := os.Create(path) // #nosec G304 -- the test owns its temporary path.
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter.Header.ModTime = canonicalArchiveTime
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name, Mode: int64(mode), Size: int64(len(contents)), ModTime: canonicalArchiveTime,
		Typeflag: tar.TypeReg, Format: tar.FormatUSTAR,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := gzipWriter.Write([]byte("unexpected")); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func rewriteTestTarGzipPayload(t *testing.T, path string, mutate func([]byte) []byte) {
	t.Helper()
	input, err := os.Open(path) // #nosec G304 -- the test owns its temporary path.
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(input)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	payload = mutate(payload)

	output, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, archiveOutputMode) // #nosec G304 -- the test owns its temporary path.
	if err != nil {
		t.Fatal(err)
	}
	writer, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	writer.Header.ModTime = canonicalArchiveTime
	writer.Header.OS = 255
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func rewriteTestTarChecksum(t *testing.T, header []byte) {
	t.Helper()
	for index := 148; index < 156; index++ {
		header[index] = ' '
	}
	checksum := int64(0)
	for _, value := range header {
		checksum += int64(value)
	}
	if err := formatCanonicalTarOctal(header[148:155], checksum); err != nil {
		t.Fatal(err)
	}
	header[155] = ' '
}

func writeNonCanonicalZip(t *testing.T, path, name string, contents []byte, mode fs.FileMode) {
	t.Helper()
	output, err := os.Create(path) // #nosec G304 -- the test owns its temporary path.
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(output)
	header := &zip.FileHeader{Name: name, Method: zip.Store} // Canonical release entries use Deflate.
	header.SetMode(mode)
	header.SetModTime(canonicalArchiveTime)
	entry, err := zipWriter.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertCanonicalTarGzip(t *testing.T, path string, want []expectedEntry) {
	t.Helper()
	file, err := os.Open(path) // #nosec G304 -- the test owns its temporary path.
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	if !gzipReader.ModTime.Equal(canonicalArchiveTime) || gzipReader.Name != "" || gzipReader.Comment != "" || gzipReader.OS != 255 {
		t.Fatalf("non-canonical gzip header: %+v", gzipReader.Header)
	}
	tarReader := tar.NewReader(gzipReader)
	for _, expected := range want {
		header, err := tarReader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != expected.name || header.Mode != int64(expected.mode) || header.Uid != 0 || header.Gid != 0 ||
			header.Uname != "" || header.Gname != "" || !header.ModTime.Equal(canonicalArchiveTime) ||
			header.Typeflag != tar.TypeReg || header.Format != tar.FormatUSTAR {
			t.Fatalf("non-canonical tar header for %q: %+v", expected.name, header)
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, expected.contents) {
			t.Fatalf("tar entry %q content changed", expected.name)
		}
	}
	if _, err := tarReader.Next(); err != io.EOF {
		t.Fatalf("tar contains another entry: %v", err)
	}
}

func assertCanonicalZip(t *testing.T, path string, want []expectedEntry) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != len(want) {
		t.Fatalf("zip entries = %d, want %d", len(reader.File), len(want))
	}
	for index, expected := range want {
		entry := reader.File[index]
		if entry.Name != expected.name || entry.Mode().Perm() != expected.mode || !entry.Mode().IsRegular() ||
			!entry.Modified.Equal(canonicalArchiveTime) || entry.Method != zip.Deflate {
			t.Fatalf("non-canonical zip header for %q: %+v", expected.name, entry.FileHeader)
		}
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if !bytes.Equal(data, expected.contents) {
			t.Fatalf("zip entry %q content changed", expected.name)
		}
	}
}
