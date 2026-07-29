// Command archivepack creates and verifies byte-for-byte reproducible release archives.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	executableMode      = 0o755
	supportingFileMode  = 0o644
	archiveOutputMode   = 0o644
	privateOutputMode   = 0o600
	deterministicBuffer = 32 * 1024
)

var canonicalArchiveTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

type inputSpec struct {
	path string
	name string
	mode fs.FileMode
}

type archiveEntry struct {
	input io.Reader
	name  string
	size  int64
	mode  fs.FileMode
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "verify" {
		if len(os.Args) < 7 || (len(os.Args)-4)%3 != 0 {
			fatal(errors.New("usage: archivepack verify <tar.gz|zip> <archive-file> (<input-file> <entry-name> <0755|0644>)+"))
		}
		specs, err := parseInputSpecs(os.Args[4:])
		if err != nil {
			fatal(err)
		}
		if err := verifyArchive(os.Args[2], os.Args[3], specs); err != nil {
			fatal(err)
		}
		return
	}

	if len(os.Args) < 6 || (len(os.Args)-3)%3 != 0 {
		fatal(errors.New("usage: archivepack <tar.gz|zip> <output-file> (<input-file> <entry-name> <0755|0644>)+"))
	}
	specs, err := parseInputSpecs(os.Args[3:])
	if err != nil {
		fatal(err)
	}
	if err := pack(os.Args[1], os.Args[2], specs); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "archivepack: %v\n", err)
	os.Exit(1)
}

func parseInputSpecs(arguments []string) ([]inputSpec, error) {
	if len(arguments) == 0 || len(arguments)%3 != 0 {
		return nil, errors.New("archive entries must be input-file, entry-name, and mode triples")
	}
	specs := make([]inputSpec, 0, len(arguments)/3)
	for argument := 0; argument < len(arguments); argument += 3 {
		mode, err := parseArchiveMode(arguments[argument+2])
		if err != nil {
			return nil, err
		}
		specs = append(specs, inputSpec{
			path: arguments[argument],
			name: arguments[argument+1],
			mode: mode,
		})
	}
	return specs, nil
}

func parseArchiveMode(value string) (fs.FileMode, error) {
	switch value {
	case "0755":
		return executableMode, nil
	case "0644":
		return supportingFileMode, nil
	default:
		return 0, fmt.Errorf("unsupported archive entry mode %q: use 0755 or 0644", value)
	}
}

func pack(archiveFormat, outputPath string, specs []inputSpec) (returnErr error) {
	if err := validateArchiveFormat(archiveFormat); err != nil {
		return err
	}
	if err := validateInputSpecs(specs); err != nil {
		return err
	}

	entries := make([]archiveEntry, 0, len(specs))
	inputs := make([]*os.File, 0, len(specs))
	createdOutput := false
	defer func() {
		for _, input := range inputs {
			if closeErr := input.Close(); returnErr == nil && closeErr != nil {
				returnErr = fmt.Errorf("close input: %w", closeErr)
			}
		}
		if returnErr != nil && createdOutput {
			_ = os.Remove(outputPath) // #nosec G703 -- cleanup targets only the exact archive this call successfully created.
		}
	}()

	for _, spec := range specs {
		info, err := os.Lstat(spec.path) // #nosec G703 -- archivepack intentionally accepts explicit local release inputs, not paths relative to a trusted root.
		if err != nil {
			return fmt.Errorf("inspect input %q: %w", spec.name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("input %q must be a regular file reached without a symbolic link", spec.name)
		}
		input, err := os.Open(spec.path) // #nosec G304 G703 -- the explicit input was type-checked above; no trusted base-directory boundary is claimed.
		if err != nil {
			return fmt.Errorf("open input %q: %w", spec.name, err)
		}
		inputs = append(inputs, input)
		openedInfo, err := input.Stat()
		if err != nil {
			return fmt.Errorf("inspect opened input %q: %w", spec.name, err)
		}
		if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
			return fmt.Errorf("input %q changed while it was being opened", spec.name)
		}
		entries = append(entries, archiveEntry{
			input: input,
			name:  spec.name,
			size:  openedInfo.Size(),
			mode:  spec.mode,
		})
	}

	if err := createArchive(archiveFormat, entries, outputPath); err != nil {
		return err
	}
	createdOutput = true
	return nil
}

func validateArchiveFormat(archiveFormat string) error {
	if archiveFormat != "tar.gz" && archiveFormat != "zip" {
		return fmt.Errorf("unsupported archive format %q", archiveFormat)
	}
	return nil
}

func validateInputSpecs(specs []inputSpec) error {
	if len(specs) == 0 {
		return errors.New("archive must contain at least one entry")
	}
	seen := make(map[string]struct{}, len(specs))
	seenFolded := make(map[string]string, len(specs))
	for _, spec := range specs {
		if err := validateEntryName(spec.name); err != nil {
			return err
		}
		if err := validateEntryMode(spec.mode); err != nil {
			return fmt.Errorf("entry %q: %w", spec.name, err)
		}
		if _, duplicate := seen[spec.name]; duplicate {
			return fmt.Errorf("duplicate archive entry name %q", spec.name)
		}
		folded := strings.ToUpper(spec.name)
		if previous, duplicate := seenFolded[folded]; duplicate {
			return fmt.Errorf("archive entry names %q and %q collide on a case-insensitive filesystem", previous, spec.name)
		}
		seen[spec.name] = struct{}{}
		seenFolded[folded] = spec.name
	}
	return nil
}

func canonicalEntries(entries []archiveEntry) ([]archiveEntry, error) {
	if len(entries) == 0 {
		return nil, errors.New("archive must contain at least one entry")
	}
	ordered := append([]archiveEntry(nil), entries...)
	// Go string comparison is a bytewise lexicographic order independent of locale.
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].name < ordered[right].name
	})
	seenFolded := make(map[string]string, len(ordered))
	for index, entry := range ordered {
		if err := validateEntryName(entry.name); err != nil {
			return nil, err
		}
		if err := validateEntryMode(entry.mode); err != nil {
			return nil, fmt.Errorf("entry %q: %w", entry.name, err)
		}
		if entry.input == nil {
			return nil, fmt.Errorf("entry %q has no input", entry.name)
		}
		if entry.size < 0 {
			return nil, fmt.Errorf("entry %q has a negative size", entry.name)
		}
		if index != 0 && entry.name == ordered[index-1].name {
			return nil, fmt.Errorf("duplicate archive entry name %q", entry.name)
		}
		folded := strings.ToUpper(entry.name)
		if previous, duplicate := seenFolded[folded]; duplicate {
			return nil, fmt.Errorf("archive entry names %q and %q collide on a case-insensitive filesystem", previous, entry.name)
		}
		seenFolded[folded] = entry.name
	}
	return ordered, nil
}

func validateEntryMode(mode fs.FileMode) error {
	if mode != executableMode && mode != supportingFileMode {
		return fmt.Errorf("unsupported archive entry mode %04o: use 0755 or 0644", mode)
	}
	return nil
}

func validateEntryName(name string) error {
	if !utf8.ValidString(name) {
		return errors.New("archive entry name must be valid UTF-8")
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00<>\"|?*") ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("entry name %q is not a safe archive basename", name)
	}
	if len(name) > 100 {
		return fmt.Errorf("entry name %q exceeds the portable USTAR basename limit of 100 bytes", name)
	}
	for _, character := range []byte(name) {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("entry name %q must use portable printable ASCII without spaces", name)
		}
	}
	stem, _, _ := strings.Cut(name, ".")
	upperStem := strings.ToUpper(stem)
	if upperStem == "CON" || upperStem == "PRN" || upperStem == "AUX" || upperStem == "NUL" ||
		(len(upperStem) == 4 && upperStem[3] >= '1' && upperStem[3] <= '9' &&
			(strings.HasPrefix(upperStem, "COM") || strings.HasPrefix(upperStem, "LPT"))) {
		return fmt.Errorf("entry name %q uses a reserved Windows device basename", name)
	}
	return nil
}

func createArchive(archiveFormat string, entries []archiveEntry, outputPath string) (returnErr error) {
	if err := validateArchiveFormat(archiveFormat); err != nil {
		return err
	}
	orderedEntries, err := canonicalEntries(entries)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateOutputMode) // #nosec G304 G703 -- the caller supplies an explicit staging output and O_EXCL prevents overwrite or symlink traversal.
	if err != nil {
		return fmt.Errorf("create output without overwrite: %w", err)
	}
	success := false
	defer func() {
		if closeErr := output.Close(); returnErr == nil && closeErr != nil {
			returnErr = fmt.Errorf("close output: %w", closeErr)
		}
		if returnErr != nil || !success {
			_ = os.Remove(outputPath) // #nosec G703 -- cleanup targets only the exact staging path this call created with O_EXCL.
		}
	}()

	switch archiveFormat {
	case "tar.gz":
		returnErr = writeTarGzip(output, orderedEntries)
	case "zip":
		returnErr = writeZip(output, orderedEntries)
	}
	if returnErr != nil {
		return returnErr
	}
	if err := output.Chmod(archiveOutputMode); err != nil {
		return fmt.Errorf("set completed archive permissions: %w", err)
	}
	success = true
	return nil
}

func writeTarGzip(output io.Writer, entries []archiveEntry) error {
	gzipWriter, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	gzipWriter.Header.ModTime = canonicalArchiveTime
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	tarClosed := false
	gzipClosed := false
	defer func() {
		if !tarClosed {
			_ = tarWriter.Close()
		}
		if !gzipClosed {
			_ = gzipWriter.Close()
		}
	}()

	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     int64(entry.mode),
			Uid:      0,
			Gid:      0,
			Size:     entry.size,
			ModTime:  canonicalArchiveTime,
			Typeflag: tar.TypeReg,
			Format:   tar.FormatUSTAR,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header for %q: %w", entry.name, err)
		}
		written, err := copyDeterministic(tarWriter, entry.input)
		if err != nil {
			return fmt.Errorf("write tar entry %q: %w", entry.name, err)
		}
		if written != entry.size {
			return fmt.Errorf("input %q size changed while archiving: wrote %d bytes, expected %d", entry.name, written, entry.size)
		}
	}
	tarClosed = true
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}
	gzipClosed = true
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	return nil
}

func writeZip(output io.Writer, entries []archiveEntry) error {
	zipWriter := zip.NewWriter(output)
	zipWriter.RegisterCompressor(zip.Deflate, func(destination io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(destination, flate.BestCompression)
	})
	closed := false
	defer func() {
		if !closed {
			_ = zipWriter.Close()
		}
	}()

	for _, archiveEntry := range entries {
		header := &zip.FileHeader{Name: archiveEntry.name, Method: zip.Deflate}
		header.SetMode(archiveEntry.mode)
		header.SetModTime(canonicalArchiveTime)
		entry, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("write zip header for %q: %w", archiveEntry.name, err)
		}
		written, err := copyDeterministic(entry, archiveEntry.input)
		if err != nil {
			return fmt.Errorf("write zip entry %q: %w", archiveEntry.name, err)
		}
		if written != archiveEntry.size {
			return fmt.Errorf("input %q size changed while archiving: wrote %d bytes, expected %d", archiveEntry.name, written, archiveEntry.size)
		}
	}
	closed = true
	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}
	return nil
}

func copyDeterministic(destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, deterministicBuffer)
	var total int64
	for {
		read, readErr := io.ReadFull(source, buffer)
		if read != 0 {
			written, writeErr := destination.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		switch readErr {
		case nil:
			continue
		case io.EOF, io.ErrUnexpectedEOF:
			return total, nil
		default:
			return total, readErr
		}
	}
}
