package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
)

const (
	zipLocalHeaderSignature            = 0x04034b50
	zipDataDescriptorSignature         = 0x08074b50
	zipCentralHeaderSignature          = 0x02014b50
	zipEndSignature                    = 0x06054b50
	zipVersion20                       = 20
	zipUnixCreator20                   = 3<<8 | zipVersion20
	zipDataDescriptorFlag              = 0x0008
	zipDOSDate19800101                 = 0x0021
	tarBlockSize                       = 512
	canonicalArchiveUnixSeconds uint32 = 315532800
)

// verifyArchive deliberately reopens both the completed archive and each
// reviewed input. It does not trust successful writer calls as proof of the
// artifact that will be published.
func verifyArchive(archiveFormat, archivePath string, specs []inputSpec) error {
	if err := validateArchiveFormat(archiveFormat); err != nil {
		return err
	}
	if err := validateInputSpecs(specs); err != nil {
		return err
	}
	orderedSpecs := append([]inputSpec(nil), specs...)
	// Go string comparison supplies the same locale-independent byte order as
	// archive creation.
	sort.Slice(orderedSpecs, func(left, right int) bool {
		return orderedSpecs[left].name < orderedSpecs[right].name
	})

	archive, archiveInfo, err := openVerifiedRegularFile(archivePath, "archive")
	if err != nil {
		return err
	}
	defer archive.Close()

	switch archiveFormat {
	case "tar.gz":
		return verifyTarGzipArchive(archive, orderedSpecs)
	case "zip":
		return verifyZipArchive(archive, archiveInfo.Size(), orderedSpecs)
	default:
		return fmt.Errorf("unsupported archive format %q", archiveFormat)
	}
}

func openVerifiedRegularFile(path, description string) (*os.File, os.FileInfo, error) {
	info, err := os.Lstat(path) // #nosec G703 -- release verification intentionally accepts one explicit local path.
	if err != nil {
		return nil, nil, fmt.Errorf("inspect %s: %w", description, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s must be a regular file reached without a symbolic link", description)
	}
	file, err := os.Open(path) // #nosec G304 G703 -- the explicit file was type-checked immediately above.
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", description, err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("inspect opened %s: %w", description, err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%s changed while it was being opened", description)
	}
	return file, openedInfo, nil
}

func verifyTarGzipArchive(archive *os.File, specs []inputSpec) error {
	if err := verifyCanonicalGzipHeader(archive); err != nil {
		return err
	}
	bufferedArchive := bufio.NewReader(archive)
	gzipReader, err := gzip.NewReader(bufferedArchive)
	if err != nil {
		return fmt.Errorf("open gzip archive: %w", err)
	}
	gzipReader.Multistream(false)
	closed := false
	defer func() {
		if !closed {
			_ = gzipReader.Close()
		}
	}()
	if !gzipReader.ModTime.Equal(canonicalArchiveTime) || gzipReader.OS != 255 ||
		gzipReader.Name != "" || gzipReader.Comment != "" || len(gzipReader.Extra) != 0 {
		return errors.New("gzip header is not canonical")
	}

	for _, spec := range specs {
		header := make([]byte, tarBlockSize)
		if _, err := io.ReadFull(gzipReader, header); err != nil {
			return fmt.Errorf("read tar header for %q: %w", spec.name, err)
		}
		if name := string(bytes.TrimRight(header[0:100], "\x00")); name != spec.name {
			return fmt.Errorf("tar entry order or name = %q, want %q", name, spec.name)
		}
		mode, err := parseCanonicalTarOctal(header[100:108])
		if err != nil {
			return fmt.Errorf("tar entry %q mode is not canonical: %w", spec.name, err)
		}
		if mode != int64(spec.mode.Perm()) {
			return fmt.Errorf("tar entry %q mode = %04o, want %04o", spec.name, mode, spec.mode.Perm())
		}
		size, err := parseCanonicalTarOctal(header[124:136])
		if err != nil {
			return fmt.Errorf("tar entry %q size is not canonical: %w", spec.name, err)
		}
		expectedHeader, err := canonicalTarHeader(spec, size)
		if err != nil {
			return fmt.Errorf("build canonical tar header for %q: %w", spec.name, err)
		}
		if !bytes.Equal(header, expectedHeader) {
			return fmt.Errorf("tar entry %q raw header is not canonical", spec.name)
		}
		if err := verifyEntryContent(io.LimitReader(gzipReader, size), size, spec); err != nil {
			return err
		}
		paddingSize := tarPadding(size)
		if paddingSize != 0 {
			padding := make([]byte, paddingSize)
			if _, err := io.ReadFull(gzipReader, padding); err != nil {
				return fmt.Errorf("read tar padding for %q: %w", spec.name, err)
			}
			if !allZero(padding) {
				return fmt.Errorf("tar entry %q padding is not canonical zero data", spec.name)
			}
		}
	}
	for terminator := 0; terminator < 2; terminator++ {
		block := make([]byte, tarBlockSize)
		if _, err := io.ReadFull(gzipReader, block); err != nil {
			return fmt.Errorf("read canonical tar terminator block %d: %w", terminator+1, err)
		}
		if !allZero(block) {
			return fmt.Errorf("tar archive contains unexpected entry or noncanonical terminator data at block %d", terminator+1)
		}
	}
	var trailing [1]byte
	read, err := gzipReader.Read(trailing[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("finish reading gzip archive: %w", err)
	}
	if read != 0 || !errors.Is(err, io.EOF) {
		return errors.New("tar.gz archive contains uncompressed data after the canonical tar terminator")
	}
	if err := gzipReader.Close(); err != nil {
		return fmt.Errorf("close gzip archive: %w", err)
	}
	closed = true
	if _, err := bufferedArchive.ReadByte(); err != io.EOF {
		if err != nil {
			return fmt.Errorf("inspect bytes after gzip member: %w", err)
		}
		return errors.New("tar.gz archive contains data after its canonical gzip member")
	}
	return nil
}

func canonicalTarHeader(spec inputSpec, size int64) ([]byte, error) {
	if err := validateEntryName(spec.name); err != nil {
		return nil, err
	}
	if err := validateEntryMode(spec.mode); err != nil {
		return nil, err
	}
	header := make([]byte, tarBlockSize)
	copy(header[0:100], spec.name)
	for _, field := range []struct {
		bytes []byte
		value int64
	}{
		{bytes: header[100:108], value: int64(spec.mode.Perm())},
		{bytes: header[108:116], value: 0},
		{bytes: header[116:124], value: 0},
		{bytes: header[124:136], value: size},
		{bytes: header[136:148], value: canonicalArchiveTime.Unix()},
		{bytes: header[329:337], value: 0},
		{bytes: header[337:345], value: 0},
	} {
		if err := formatCanonicalTarOctal(field.bytes, field.value); err != nil {
			return nil, err
		}
	}
	header[156] = '0'
	copy(header[257:263], "ustar\x00")
	copy(header[263:265], "00")
	for index := 148; index < 156; index++ {
		header[index] = ' '
	}
	checksum := int64(0)
	for _, value := range header {
		checksum += int64(value)
	}
	if err := formatCanonicalTarOctal(header[148:155], checksum); err != nil {
		return nil, err
	}
	header[155] = ' '
	return header, nil
}

func formatCanonicalTarOctal(field []byte, value int64) error {
	if len(field) < 2 || value < 0 {
		return errors.New("tar numeric value is outside the canonical octal grammar")
	}
	encoded := strconv.FormatInt(value, 8)
	if len(encoded) > len(field)-1 {
		return errors.New("tar numeric value exceeds its canonical octal field")
	}
	for index := range field[:len(field)-1] {
		field[index] = '0'
	}
	copy(field[len(field)-1-len(encoded):len(field)-1], encoded)
	field[len(field)-1] = 0
	return nil
}

func parseCanonicalTarOctal(field []byte) (int64, error) {
	if len(field) < 2 || field[len(field)-1] != 0 {
		return 0, errors.New("octal field must end in NUL")
	}
	for _, value := range field[:len(field)-1] {
		if value < '0' || value > '7' {
			return 0, errors.New("octal field contains a non-octal byte")
		}
	}
	value, err := strconv.ParseInt(string(field[:len(field)-1]), 8, 64)
	if err != nil {
		return 0, errors.New("octal field overflows")
	}
	return value, nil
}

func tarPadding(size int64) int {
	return int(-size & (tarBlockSize - 1))
}

func allZero(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
}

func verifyZipArchive(archive *os.File, archiveSize int64, specs []inputSpec) error {
	zipReader, err := zip.NewReader(archive, archiveSize)
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	if len(zipReader.File) != len(specs) {
		return fmt.Errorf("zip entry count = %d, want %d", len(zipReader.File), len(specs))
	}
	if zipReader.Comment != "" {
		return errors.New("zip archive comment is not canonical")
	}
	for index, spec := range specs {
		entry := zipReader.File[index]
		if entry.Name != spec.name {
			return fmt.Errorf("zip entry order or name = %q, want %q", entry.Name, spec.name)
		}
		if !entry.Mode().IsRegular() || entry.Mode().Perm() != spec.mode {
			return fmt.Errorf("zip entry %q mode = %04o, want regular %04o", spec.name, entry.Mode().Perm(), spec.mode)
		}
		if entry.Method != zip.Deflate || !entry.Modified.Equal(canonicalArchiveTime) ||
			entry.Comment != "" || entry.NonUTF8 || entry.Flags&0x8 == 0 || entry.Flags&^uint16(0x808) != 0 {
			return fmt.Errorf("zip entry %q header is not canonical", spec.name)
		}
		content, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", spec.name, err)
		}
		if entry.UncompressedSize64 > math.MaxInt64 {
			_ = content.Close()
			return fmt.Errorf("zip entry %q is too large to verify", spec.name)
		}
		// #nosec G115 -- the explicit MaxInt64 bound above makes this conversion exact.
		verifyErr := verifyEntryContent(content, int64(entry.UncompressedSize64), spec)
		closeErr := content.Close()
		if verifyErr != nil {
			return verifyErr
		}
		if closeErr != nil {
			return fmt.Errorf("close zip entry %q: %w", spec.name, closeErr)
		}
	}
	return verifyCanonicalZipLayout(archive, archiveSize, zipReader.File, specs)
}

func verifyCanonicalGzipHeader(archive *os.File) error {
	header, err := readAtExact(archive, 0, 10)
	if err != nil {
		return fmt.Errorf("read gzip header: %w", err)
	}
	if header[0] != 0x1f || header[1] != 0x8b || header[2] != 8 || header[3] != 0 ||
		binary.LittleEndian.Uint32(header[4:8]) != canonicalArchiveUnixSeconds ||
		header[8] != 2 || header[9] != 255 {
		return errors.New("gzip header is not canonical at the raw byte boundary")
	}
	return nil
}

func verifyCanonicalZipLayout(archive *os.File, archiveSize int64, files []*zip.File, specs []inputSpec) error {
	if archiveSize < 22 || len(files) != len(specs) || len(specs) > math.MaxUint16 {
		return errors.New("zip archive size or entry count is outside the canonical format")
	}
	endOffset := archiveSize - 22
	end, err := readAtExact(archive, endOffset, 22)
	if err != nil {
		return fmt.Errorf("read zip end record: %w", err)
	}
	if binary.LittleEndian.Uint32(end[0:4]) != zipEndSignature ||
		binary.LittleEndian.Uint16(end[4:6]) != 0 || binary.LittleEndian.Uint16(end[6:8]) != 0 ||
		int(binary.LittleEndian.Uint16(end[8:10])) != len(specs) ||
		int(binary.LittleEndian.Uint16(end[10:12])) != len(specs) ||
		binary.LittleEndian.Uint16(end[20:22]) != 0 {
		return errors.New("zip end record is not canonical")
	}
	centralSize := int64(binary.LittleEndian.Uint32(end[12:16]))
	centralOffset := int64(binary.LittleEndian.Uint32(end[16:20]))
	if centralOffset < 0 || centralSize < 0 || centralOffset+centralSize != endOffset {
		return errors.New("zip central directory does not end at the canonical end record")
	}

	localOffset := int64(0)
	localOffsets := make([]uint32, len(specs))
	compressedSizes := make([]uint32, len(specs))
	uncompressedSizes := make([]uint32, len(specs))
	for index, spec := range specs {
		entry := files[index]
		if entry.CompressedSize64 > math.MaxUint32 || entry.UncompressedSize64 > math.MaxUint32 || localOffset > math.MaxUint32 {
			return fmt.Errorf("zip entry %q requires non-canonical ZIP64 metadata", spec.name)
		}
		localOffsets[index] = uint32(localOffset)
		compressedSizes[index] = uint32(entry.CompressedSize64)
		uncompressedSizes[index] = uint32(entry.UncompressedSize64)
		header, err := readAtExact(archive, localOffset, 30)
		if err != nil {
			return fmt.Errorf("read local zip header for %q: %w", spec.name, err)
		}
		if binary.LittleEndian.Uint32(header[0:4]) != zipLocalHeaderSignature ||
			binary.LittleEndian.Uint16(header[4:6]) != zipVersion20 ||
			binary.LittleEndian.Uint16(header[6:8]) != zipDataDescriptorFlag ||
			binary.LittleEndian.Uint16(header[8:10]) != zip.Deflate ||
			binary.LittleEndian.Uint16(header[10:12]) != 0 ||
			binary.LittleEndian.Uint16(header[12:14]) != zipDOSDate19800101 ||
			binary.LittleEndian.Uint32(header[14:18]) != 0 ||
			binary.LittleEndian.Uint32(header[18:22]) != 0 ||
			binary.LittleEndian.Uint32(header[22:26]) != 0 {
			return fmt.Errorf("zip entry %q local header is not canonical", spec.name)
		}
		nameLength := int(binary.LittleEndian.Uint16(header[26:28]))
		extraLength := int(binary.LittleEndian.Uint16(header[28:30]))
		nameAndExtra, err := readAtExact(archive, localOffset+30, nameLength+extraLength)
		if err != nil {
			return fmt.Errorf("read local zip name for %q: %w", spec.name, err)
		}
		if string(nameAndExtra[:nameLength]) != spec.name || !bytes.Equal(nameAndExtra[nameLength:], canonicalZipExtra()) {
			return fmt.Errorf("zip entry %q local name or extra field is not canonical", spec.name)
		}
		dataOffset := localOffset + 30 + int64(nameLength+extraLength)
		projectedDataOffset, err := entry.DataOffset()
		if err != nil || projectedDataOffset != dataOffset {
			return fmt.Errorf("zip entry %q data offset is not canonical", spec.name)
		}
		descriptorOffset := dataOffset + int64(entry.CompressedSize64)
		descriptor, err := readAtExact(archive, descriptorOffset, 16)
		if err != nil {
			return fmt.Errorf("read zip data descriptor for %q: %w", spec.name, err)
		}
		if binary.LittleEndian.Uint32(descriptor[0:4]) != zipDataDescriptorSignature ||
			binary.LittleEndian.Uint32(descriptor[4:8]) != entry.CRC32 ||
			binary.LittleEndian.Uint32(descriptor[8:12]) != compressedSizes[index] ||
			binary.LittleEndian.Uint32(descriptor[12:16]) != uncompressedSizes[index] {
			return fmt.Errorf("zip entry %q data descriptor is not canonical", spec.name)
		}
		localOffset = descriptorOffset + 16
	}
	if localOffset != centralOffset {
		return errors.New("zip local entry sequence does not meet the central directory")
	}

	position := centralOffset
	for index, spec := range specs {
		entry := files[index]
		header, err := readAtExact(archive, position, 46)
		if err != nil {
			return fmt.Errorf("read central zip header for %q: %w", spec.name, err)
		}
		externalMode := uint32(0o100000|spec.mode.Perm()) << 16
		if binary.LittleEndian.Uint32(header[0:4]) != zipCentralHeaderSignature ||
			binary.LittleEndian.Uint16(header[4:6]) != zipUnixCreator20 ||
			binary.LittleEndian.Uint16(header[6:8]) != zipVersion20 ||
			binary.LittleEndian.Uint16(header[8:10]) != zipDataDescriptorFlag ||
			binary.LittleEndian.Uint16(header[10:12]) != zip.Deflate ||
			binary.LittleEndian.Uint16(header[12:14]) != 0 ||
			binary.LittleEndian.Uint16(header[14:16]) != zipDOSDate19800101 ||
			binary.LittleEndian.Uint32(header[16:20]) != entry.CRC32 ||
			binary.LittleEndian.Uint32(header[20:24]) != compressedSizes[index] ||
			binary.LittleEndian.Uint32(header[24:28]) != uncompressedSizes[index] ||
			binary.LittleEndian.Uint16(header[32:34]) != 0 ||
			binary.LittleEndian.Uint16(header[34:36]) != 0 ||
			binary.LittleEndian.Uint16(header[36:38]) != 0 ||
			binary.LittleEndian.Uint32(header[38:42]) != externalMode ||
			binary.LittleEndian.Uint32(header[42:46]) != localOffsets[index] {
			return fmt.Errorf("zip entry %q central header is not canonical", spec.name)
		}
		nameLength := int(binary.LittleEndian.Uint16(header[28:30]))
		extraLength := int(binary.LittleEndian.Uint16(header[30:32]))
		nameAndExtra, err := readAtExact(archive, position+46, nameLength+extraLength)
		if err != nil {
			return fmt.Errorf("read central zip name for %q: %w", spec.name, err)
		}
		if string(nameAndExtra[:nameLength]) != spec.name || !bytes.Equal(nameAndExtra[nameLength:], canonicalZipExtra()) {
			return fmt.Errorf("zip entry %q central name or extra field is not canonical", spec.name)
		}
		position += 46 + int64(nameLength+extraLength)
	}
	if position != centralOffset+centralSize {
		return errors.New("zip central directory size is not canonical")
	}
	return nil
}

func canonicalZipExtra() []byte {
	extra := make([]byte, 9)
	binary.LittleEndian.PutUint16(extra[0:2], 0x5455)
	binary.LittleEndian.PutUint16(extra[2:4], 5)
	extra[4] = 1
	binary.LittleEndian.PutUint32(extra[5:9], canonicalArchiveUnixSeconds)
	return extra
}

func readAtExact(reader io.ReaderAt, offset int64, size int) ([]byte, error) {
	if offset < 0 || size < 0 {
		return nil, errors.New("negative archive offset or size")
	}
	buffer := make([]byte, size)
	read, err := reader.ReadAt(buffer, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if read != size {
		return nil, io.ErrUnexpectedEOF
	}
	return buffer, nil
}

func verifyEntryContent(actual io.Reader, actualSize int64, spec inputSpec) error {
	expected, expectedInfo, err := openVerifiedRegularFile(spec.path, fmt.Sprintf("expected input %q", spec.name))
	if err != nil {
		return err
	}
	defer expected.Close()
	if actualSize != expectedInfo.Size() {
		return fmt.Errorf("archive entry %q size = %d, want %d", spec.name, actualSize, expectedInfo.Size())
	}

	actualBuffer := make([]byte, deterministicBuffer)
	expectedBuffer := make([]byte, deterministicBuffer)
	for {
		actualRead, actualErr := io.ReadFull(actual, actualBuffer)
		expectedRead, expectedErr := io.ReadFull(expected, expectedBuffer)
		if actualRead != expectedRead || !bytes.Equal(actualBuffer[:actualRead], expectedBuffer[:expectedRead]) {
			return fmt.Errorf("archive entry %q content differs from its reviewed input", spec.name)
		}
		actualDone := actualErr == io.EOF || actualErr == io.ErrUnexpectedEOF
		expectedDone := expectedErr == io.EOF || expectedErr == io.ErrUnexpectedEOF
		if actualDone || expectedDone {
			if actualDone && expectedDone {
				return nil
			}
			return fmt.Errorf("archive entry %q content length changed during verification", spec.name)
		}
		if actualErr != nil {
			return fmt.Errorf("read archive entry %q: %w", spec.name, actualErr)
		}
		if expectedErr != nil {
			return fmt.Errorf("read expected input %q: %w", spec.name, expectedErr)
		}
	}
}
