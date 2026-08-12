package credentialhost

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	maxClaudeTerminalBytes = 128 << 10
	maxClaudeVisibleBytes  = 64 << 10
	maxClaudeTokenBytes    = 16 << 10
	maxClaudeTerminalRows  = 2048
	maxClaudeTerminalCells = maxClaudeTerminalBytes
	maxClaudeTerminalWork  = maxClaudeTerminalBytes
	maxClaudeHistoryBytes  = maxClaudeTerminalBytes
	// The fixed private PTY is deliberately wider than the maximum accepted
	// token so the terminal, rather than parser inference, never inserts a
	// visual wrap into the opaque candidate.
	claudeTerminalColumns = 32767

	claudeSetupIntro       = "This will guide you through long-lived (1-year) auth token setup for your Claude account. Claude subscription required."
	claudeSetupSuccessLine = "✓ Long-lived authentication token created successfully!"
	claudeSetupTokenMarker = "Your OAuth token (valid for 1 year):" // #nosec G101 -- fixed public Claude UI text, not a credential.
	claudeSetupFooter      = "Store this token securely. You won't be able to see it again."
	claudeSetupUsage       = "Use this token by setting: export CLAUDE_CODE_OAUTH_TOKEN=<token>"
)

// claudeSetupOutputParser validates and removes a deliberately small set of
// terminal control sequences emitted by the pinned Ink UI. It retains the
// complete bounded transcript until Finish validates the exact success frame,
// then writes only fixed non-secret text. No provider-controlled bytes cross
// the visible boundary before the token and every raw occurrence are known.
type claudeSetupOutputParser struct {
	mu sync.Mutex

	visible      io.Writer
	rawBytes     int
	visibleBytes int
	clean        []byte
	escape       []byte
	utf8Pending  []byte
	failure      error
	finished     bool
	terminal     claudeTerminalState
}

func newClaudeSetupOutputParser(visible io.Writer) *claudeSetupOutputParser {
	if visible == nil {
		visible = io.Discard
	}
	return &claudeSetupOutputParser{visible: visible}
}

func (p *claudeSetupOutputParser) Write(content []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return 0, ErrClaudeOutputFraming
	}
	if p.failure != nil {
		return 0, p.failure
	}
	if len(content) > maxClaudeTerminalBytes-p.rawBytes {
		p.failLocked(ErrClaudeOutputLimit)
		return 0, p.failure
	}
	p.rawBytes += len(content)
	for index, current := range content {
		if err := p.consumeByteLocked(current); err != nil {
			p.failLocked(err)
			return index, p.failure
		}
	}
	return len(content), nil
}

func (p *claudeSetupOutputParser) consumeByteLocked(current byte) error {
	if len(p.escape) != 0 {
		p.escape = append(p.escape, current)
		if len(p.escape) > 32 {
			return ErrClaudeOutputFraming
		}
		if len(p.escape) == 2 && current != '[' {
			return ErrClaudeOutputFraming
		}
		if len(p.escape) == 2 {
			return nil
		}
		if current >= 0x40 && current <= 0x7e {
			if !validClaudeCSI(p.escape) {
				return ErrClaudeOutputFraming
			}
			if err := p.terminal.applyCSI(p.escape); err != nil {
				return err
			}
			clear(p.escape)
			p.escape = nil
			return nil
		}
		if current < 0x20 || current > 0x3f {
			return ErrClaudeOutputFraming
		}
		return nil
	}
	if current == 0x1b {
		if len(p.utf8Pending) != 0 {
			return ErrClaudeOutputFraming
		}
		p.escape = append(p.escape, current)
		return nil
	}
	if current < utf8.RuneSelf {
		if len(p.utf8Pending) != 0 || (current < 0x20 && current != '\n' && current != '\r') || current == 0x7f {
			return ErrClaudeOutputFraming
		}
		return p.emitTextLocked([]byte{current})
	}
	p.utf8Pending = append(p.utf8Pending, current)
	if !utf8.FullRune(p.utf8Pending) {
		if len(p.utf8Pending) >= utf8.UTFMax {
			return ErrClaudeOutputFraming
		}
		return nil
	}
	decoded, size := utf8.DecodeRune(p.utf8Pending)
	if decoded == utf8.RuneError && size == 1 || size != len(p.utf8Pending) ||
		unicode.IsControl(decoded) || unicode.Is(unicode.Cf, decoded) ||
		decoded == '\u2028' || decoded == '\u2029' {
		return ErrClaudeOutputFraming
	}
	encoded := append([]byte(nil), p.utf8Pending...)
	clear(p.utf8Pending)
	p.utf8Pending = nil
	return p.emitTextLocked(encoded)
}

func (p *claudeSetupOutputParser) emitTextLocked(content []byte) error {
	decoded, _ := utf8.DecodeRune(content)
	if err := p.terminal.write(decoded); err != nil {
		return err
	}
	p.clean = append(p.clean, content...)
	return nil
}

func (p *claudeSetupOutputParser) writeVisibleLocked(content []byte) error {
	if len(content) > maxClaudeVisibleBytes-p.visibleBytes {
		return ErrClaudeOutputLimit
	}
	written, err := p.visible.Write(content)
	if err != nil || written != len(content) {
		return ErrClaudeVisibleOutput
	}
	p.visibleBytes += written
	return nil
}

func (p *claudeSetupOutputParser) Finish() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return nil, ErrClaudeOutputFraming
	}
	p.finished = true
	defer func() {
		clear(p.clean)
		p.clean = nil
		clear(p.escape)
		p.escape = nil
		clear(p.utf8Pending)
		p.utf8Pending = nil
		p.terminal.clear()
	}()
	if p.failure != nil {
		return nil, p.failure
	}
	if len(p.escape) != 0 || len(p.utf8Pending) != 0 || !utf8.Valid(p.clean) {
		return nil, ErrClaudeOutputFraming
	}
	terminalOutput := p.terminal.render()
	defer clear(terminalOutput)
	token, showIntro, err := parseClaudeSetupTranscript(terminalOutput)
	if err != nil {
		return nil, err
	}
	if err := validateClaudeRawTranscript(p.clean, token); err != nil {
		clear(token)
		return nil, err
	}
	if err := validateClaudeTerminalHistory(p.terminal.history, token); err != nil {
		clear(token)
		return nil, err
	}
	visible := make([]byte, 0, len(claudeSetupIntro)+len(claudeSetupSuccessLine)+2)
	if showIntro && !bytes.Contains([]byte(claudeSetupIntro), token) {
		visible = append(visible, claudeSetupIntro...)
		visible = append(visible, '\n')
	}
	if !bytes.Contains([]byte(claudeSetupSuccessLine), token) {
		visible = append(visible, claudeSetupSuccessLine...)
		visible = append(visible, '\n')
	}
	if err := p.writeVisibleLocked(visible); err != nil {
		clear(token)
		return nil, err
	}
	return token, nil
}

func (p *claudeSetupOutputParser) failLocked(err error) {
	if err == nil || p.failure != nil {
		return
	}
	switch {
	case errors.Is(err, ErrClaudeOutputLimit):
		p.failure = ErrClaudeOutputLimit
	case errors.Is(err, ErrClaudeVisibleOutput):
		p.failure = ErrClaudeVisibleOutput
	default:
		p.failure = ErrClaudeOutputFraming
	}
}

func validClaudeCSI(sequence []byte) bool {
	if len(sequence) < 3 || sequence[0] != 0x1b || sequence[1] != '[' {
		return false
	}
	parameters := sequence[2 : len(sequence)-1]
	final := sequence[len(sequence)-1]
	switch final {
	case 'm':
		return claudeCSICharacters(parameters, "0123456789;:")
	case 'K', 'J':
		return len(parameters) == 0 || bytes.Equal(parameters, []byte("0")) ||
			bytes.Equal(parameters, []byte("1")) || bytes.Equal(parameters, []byte("2")) ||
			(final == 'J' && bytes.Equal(parameters, []byte("3")))
	case 'A', 'B', 'C', 'D', 'E', 'F', 'G':
		return len(parameters) == 0 || claudeCSIDecimal(parameters)
	case 'H', 'f':
		if len(parameters) == 0 {
			return true
		}
		if bytes.Count(parameters, []byte(";")) > 1 {
			return false
		}
		for _, part := range bytes.Split(parameters, []byte(";")) {
			if len(part) != 0 && !claudeCSIDecimal(part) {
				return false
			}
		}
		return true
	case 'h', 'l':
		return bytes.Equal(parameters, []byte("?25")) ||
			bytes.Equal(parameters, []byte("?2004")) ||
			bytes.Equal(parameters, []byte("?2026"))
	default:
		return false
	}
}

// claudeTerminalState models only the fixed cursor and erase operations that
// Ink 2.1.220 uses to redraw its screen. Applying those operations, instead of
// merely stripping their bytes, keeps the final success block unambiguous when
// an earlier spinner or URL frame occupied the same terminal rows.
type claudeTerminalState struct {
	rows           [][]rune
	row            int
	column         int
	allocatedCells int
	workUnits      int
	history        []byte
}

func (s *claudeTerminalState) write(value rune) error {
	switch value {
	case '\r':
		s.column = 0
		return nil
	case '\n':
		s.row++
		s.column = 0
		return s.ensureRow(s.row)
	}
	if err := s.ensureRow(s.row); err != nil {
		return err
	}
	if s.column >= claudeTerminalColumns {
		s.row++
		s.column = 0
		if err := s.ensureRow(s.row); err != nil {
			return err
		}
	}
	row := s.rows[s.row]
	if additional := s.column + 1 - len(row); additional > 0 {
		if additional > maxClaudeTerminalCells-s.allocatedCells {
			return ErrClaudeOutputLimit
		}
		if err := s.consumeWork(additional); err != nil {
			return err
		}
		s.allocatedCells += additional
		row = append(row, make([]rune, additional)...)
		for index := len(row) - additional; index < len(row); index++ {
			row[index] = ' '
		}
	}
	if row[s.column] != ' ' && row[s.column] != value {
		if err := s.capture(); err != nil {
			return err
		}
	}
	row[s.column] = value
	s.rows[s.row] = row
	s.column++
	return nil
}

func (s *claudeTerminalState) applyCSI(sequence []byte) error {
	parameters := sequence[2 : len(sequence)-1]
	final := sequence[len(sequence)-1]
	switch final {
	case 'm', 'h', 'l':
		return nil
	case 'A':
		s.row -= claudeCSIAmount(parameters)
		if s.row < 0 {
			s.row = 0
		}
	case 'B':
		s.row += claudeCSIAmount(parameters)
	case 'C':
		s.column += claudeCSIAmount(parameters)
		if s.column >= claudeTerminalColumns {
			s.column = claudeTerminalColumns - 1
		}
	case 'D':
		s.column -= claudeCSIAmount(parameters)
		if s.column < 0 {
			s.column = 0
		}
	case 'E':
		s.row += claudeCSIAmount(parameters)
		s.column = 0
	case 'F':
		s.row -= claudeCSIAmount(parameters)
		if s.row < 0 {
			s.row = 0
		}
		s.column = 0
	case 'G':
		s.column = claudeCSIPosition(parameters, 0, claudeTerminalColumns-1)
	case 'H', 'f':
		parts := bytes.Split(parameters, []byte(";"))
		if len(parts) == 0 {
			s.row, s.column = 0, 0
		} else {
			s.row = claudeCSIPosition(parts[0], 0, maxClaudeTerminalRows-1)
			if len(parts) == 2 {
				s.column = claudeCSIPosition(parts[1], 0, claudeTerminalColumns-1)
			} else {
				s.column = 0
			}
		}
	case 'K':
		if err := s.ensureRow(s.row); err != nil {
			return err
		}
		mode := claudeCSIValue(parameters)
		switch mode {
		case 0:
			if s.column < len(s.rows[s.row]) {
				tail := s.rows[s.row][s.column:]
				if err := s.consumeWork(len(tail)); err != nil {
					return err
				}
				if claudeRunesVisible(tail) {
					if err := s.capture(); err != nil {
						return err
					}
				}
				clear(tail)
				s.rows[s.row] = s.rows[s.row][:s.column]
			}
		case 1:
			row := s.rows[s.row]
			through := s.column
			if through >= len(row) {
				through = len(row) - 1
			}
			if err := s.consumeWork(through + 1); err != nil {
				return err
			}
			if through >= 0 && claudeRunesVisible(row[:through+1]) {
				if err := s.capture(); err != nil {
					return err
				}
			}
			for index := 0; index <= through; index++ {
				row[index] = ' '
			}
		case 2:
			if err := s.consumeWork(len(s.rows[s.row])); err != nil {
				return err
			}
			if claudeRunesVisible(s.rows[s.row]) {
				if err := s.capture(); err != nil {
					return err
				}
			}
			clear(s.rows[s.row])
			s.rows[s.row] = nil
		}
	case 'J':
		if err := s.eraseDisplay(claudeCSIValue(parameters)); err != nil {
			return err
		}
	}
	return s.ensureRow(s.row)
}

func (s *claudeTerminalState) eraseDisplay(mode int) error {
	switch mode {
	case 0:
		if s.row < len(s.rows) {
			work := len(s.rows) - s.row
			visible := false
			if s.column < len(s.rows[s.row]) {
				tail := s.rows[s.row][s.column:]
				work += len(tail)
				visible = claudeRunesVisible(tail)
			}
			for index := s.row + 1; index < len(s.rows); index++ {
				work += len(s.rows[index])
				visible = visible || claudeRunesVisible(s.rows[index])
			}
			if err := s.consumeWork(work); err != nil {
				return err
			}
			if visible {
				if err := s.capture(); err != nil {
					return err
				}
			}
			if s.column < len(s.rows[s.row]) {
				clear(s.rows[s.row][s.column:])
				s.rows[s.row] = s.rows[s.row][:s.column]
			}
			for index := s.row + 1; index < len(s.rows); index++ {
				clear(s.rows[index])
				s.rows[index] = nil
			}
		}
	case 1:
		through := s.row
		if through >= len(s.rows) {
			through = len(s.rows) - 1
		}
		work := through + 1
		visible := false
		for index := 0; index < through; index++ {
			work += len(s.rows[index])
			visible = visible || claudeRunesVisible(s.rows[index])
		}
		if through >= 0 {
			column := s.column
			if column >= len(s.rows[through]) {
				column = len(s.rows[through]) - 1
			}
			work += column + 1
			if column >= 0 {
				visible = visible || claudeRunesVisible(s.rows[through][:column+1])
			}
		}
		if err := s.consumeWork(work); err != nil {
			return err
		}
		if visible {
			if err := s.capture(); err != nil {
				return err
			}
		}
		for index := 0; index < through; index++ {
			clear(s.rows[index])
			s.rows[index] = nil
		}
		if through >= 0 {
			row := s.rows[through]
			column := s.column
			if column >= len(row) {
				column = len(row) - 1
			}
			for index := 0; index <= column; index++ {
				row[index] = ' '
			}
		}
	case 2, 3:
		work := len(s.rows)
		visible := false
		for index := range s.rows {
			work += len(s.rows[index])
			visible = visible || claudeRunesVisible(s.rows[index])
		}
		if err := s.consumeWork(work); err != nil {
			return err
		}
		if visible {
			if err := s.capture(); err != nil {
				return err
			}
		}
		s.clearDisplay()
	}
	return nil
}

func (s *claudeTerminalState) ensureRow(index int) error {
	if index < 0 || index >= maxClaudeTerminalRows {
		return ErrClaudeOutputLimit
	}
	if additional := index + 1 - len(s.rows); additional > 0 {
		if err := s.consumeWork(additional); err != nil {
			return err
		}
	}
	for len(s.rows) <= index {
		s.rows = append(s.rows, nil)
	}
	return nil
}

func (s *claudeTerminalState) render() []byte {
	last := len(s.rows) - 1
	for last >= 0 && claudeTerminalLineEnd(s.rows[last]) == 0 {
		last--
	}
	if last < 0 {
		return nil
	}
	var rendered bytes.Buffer
	for index := 0; index <= last; index++ {
		end := claudeTerminalLineEnd(s.rows[index])
		for _, value := range s.rows[index][:end] {
			rendered.WriteRune(value)
		}
		if index != last {
			rendered.WriteByte('\n')
		}
	}
	return rendered.Bytes()
}

func claudeTerminalLineEnd(line []rune) int {
	end := len(line)
	for end > 0 && line[end-1] == ' ' {
		end--
	}
	return end
}

func (s *claudeTerminalState) clear() {
	s.clearDisplay()
	s.rows = nil
	s.allocatedCells = 0
	s.workUnits = 0
	clear(s.history)
	s.history = nil
}

func (s *claudeTerminalState) clearDisplay() {
	for index := range s.rows {
		clear(s.rows[index])
		s.rows[index] = nil
	}
	s.row = 0
	s.column = 0
}

func (s *claudeTerminalState) consumeWork(units int) error {
	if units < 0 || units > maxClaudeTerminalWork-s.workUnits {
		return ErrClaudeOutputLimit
	}
	s.workUnits += units
	return nil
}

func (s *claudeTerminalState) capture() error {
	work := len(s.rows)
	for index := range s.rows {
		work += len(s.rows[index])
	}
	if err := s.consumeWork(work); err != nil {
		return err
	}
	rendered := s.render()
	defer clear(rendered)
	if len(rendered) == 0 {
		return nil
	}
	if len(rendered)+1 > maxClaudeHistoryBytes-len(s.history) {
		return ErrClaudeOutputLimit
	}
	s.history = append(s.history, rendered...)
	s.history = append(s.history, 0)
	return nil
}

func claudeRunesVisible(values []rune) bool {
	for _, value := range values {
		if value != ' ' {
			return true
		}
	}
	return false
}

func claudeCSIAmount(parameters []byte) int {
	value := claudeCSIValue(parameters)
	if value <= 0 {
		return 1
	}
	return value
}

func claudeCSIPosition(parameters []byte, minimum, maximum int) int {
	value := claudeCSIValue(parameters)
	if value <= 0 {
		value = 1
	}
	value--
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func claudeCSIValue(parameters []byte) int {
	value := 0
	for _, current := range parameters {
		value = value*10 + int(current-'0')
	}
	return value
}

func claudeCSICharacters(value []byte, allowed string) bool {
	for _, current := range value {
		if !bytes.ContainsRune([]byte(allowed), rune(current)) {
			return false
		}
	}
	return true
}

func claudeCSIDecimal(value []byte) bool {
	if len(value) == 0 || len(value) > 5 {
		return false
	}
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func parseClaudeSetupToken(output []byte) ([]byte, error) {
	token, _, err := parseClaudeSetupTranscript(output)
	return token, err
}

func parseClaudeSetupTranscript(output []byte) ([]byte, bool, error) {
	normalized := normalizeClaudeLines(output)
	defer clear(normalized)
	lines := bytes.Split(normalized, []byte{'\n'})
	nonempty := make([][]byte, 0, 6)
	for _, line := range lines {
		trimmed := bytes.Trim(line, " ")
		if len(trimmed) != 0 {
			nonempty = append(nonempty, trimmed)
		}
	}
	showIntro := len(nonempty) == 6 && bytes.Equal(nonempty[0], []byte(claudeSetupIntro))
	offset := 0
	if showIntro {
		offset = 1
	}
	if len(nonempty) != offset+5 ||
		!bytes.Equal(nonempty[offset], []byte(claudeSetupSuccessLine)) ||
		!bytes.Equal(nonempty[offset+1], []byte(claudeSetupTokenMarker)) ||
		!validClaudeSetupToken(nonempty[offset+2]) ||
		!bytes.Equal(nonempty[offset+3], []byte(claudeSetupFooter)) ||
		!bytes.Equal(nonempty[offset+4], []byte(claudeSetupUsage)) {
		return nil, false, ErrClaudeTokenCapture
	}
	return append([]byte(nil), nonempty[offset+2]...), showIntro, nil
}

type claudeRawLine struct {
	value []byte
	start int
	end   int
}

// validateClaudeRawTranscript treats the token as a field in the pinned
// terminal protocol, not as an arbitrary substring. This distinguishes a
// single success slot from an erased earlier success frame while avoiding
// false rejection when an opaque token happens to occur inside fixed public
// UI text.
func validateClaudeRawTranscript(output, finalToken []byte) error {
	normalized := normalizeClaudeLines(output)
	defer clear(normalized)
	lines := claudeRawNonemptyLines(normalized)
	candidates := make([]claudeRawLine, 0, 2)
	for index, line := range lines {
		if !bytes.Equal(line.value, []byte(claudeSetupTokenMarker)) {
			continue
		}
		if index == 0 || index+3 >= len(lines) ||
			!bytes.HasSuffix(lines[index-1].value, []byte(claudeSetupSuccessLine)) ||
			!validClaudeSetupToken(lines[index+1].value) ||
			!bytes.Equal(lines[index+2].value, []byte(claudeSetupFooter)) ||
			!bytes.Equal(lines[index+3].value, []byte(claudeSetupUsage)) {
			continue
		}
		candidates = append(candidates, lines[index+1])
	}
	if bytes.Count(normalized, []byte(claudeSetupTokenMarker)) != 1 ||
		len(candidates) != 1 || !bytes.Equal(candidates[0].value, finalToken) {
		return ErrClaudeTokenCapture
	}

	residual := append([]byte(nil), normalized...)
	defer clear(residual)
	for _, fixed := range []string{
		claudeSetupIntro,
		claudeSetupSuccessLine,
		claudeSetupTokenMarker,
		claudeSetupFooter,
		claudeSetupUsage,
	} {
		maskClaudeRawOccurrences(residual, []byte(fixed))
	}
	clear(residual[candidates[0].start:candidates[0].end])
	if bytes.Contains(residual, finalToken) {
		return ErrClaudeTokenCapture
	}
	return nil
}

func validateClaudeTerminalHistory(history, finalToken []byte) error {
	for start := 0; start < len(history); {
		end := bytes.IndexByte(history[start:], 0)
		if end < 0 {
			return ErrClaudeOutputFraming
		}
		end += start
		snapshot := history[start:end]
		if claudeSnapshotHasSuccessCandidate(snapshot) || claudeSnapshotContainsToken(snapshot, finalToken) {
			return ErrClaudeTokenCapture
		}
		start = end + 1
	}
	return nil
}

func claudeSnapshotHasSuccessCandidate(snapshot []byte) bool {
	normalized := normalizeClaudeLines(snapshot)
	defer clear(normalized)
	lines := claudeRawNonemptyLines(normalized)
	for index, line := range lines {
		if bytes.Equal(line.value, []byte(claudeSetupTokenMarker)) && index > 0 && index+3 < len(lines) &&
			bytes.HasSuffix(lines[index-1].value, []byte(claudeSetupSuccessLine)) &&
			validClaudeSetupToken(lines[index+1].value) &&
			bytes.Equal(lines[index+2].value, []byte(claudeSetupFooter)) &&
			bytes.Equal(lines[index+3].value, []byte(claudeSetupUsage)) {
			return true
		}
	}
	return false
}

func claudeSnapshotContainsToken(snapshot, token []byte) bool {
	residual := append([]byte(nil), snapshot...)
	defer clear(residual)
	for _, fixed := range []string{
		claudeSetupIntro,
		claudeSetupSuccessLine,
		claudeSetupTokenMarker,
		claudeSetupFooter,
		claudeSetupUsage,
	} {
		maskClaudeRawOccurrences(residual, []byte(fixed))
	}
	return bytes.Contains(residual, token)
}

func claudeRawNonemptyLines(normalized []byte) []claudeRawLine {
	lines := make([]claudeRawLine, 0, 8)
	for start := 0; start <= len(normalized); {
		end := bytes.IndexByte(normalized[start:], '\n')
		if end < 0 {
			end = len(normalized)
		} else {
			end += start
		}
		trimmedStart, trimmedEnd := start, end
		for trimmedStart < trimmedEnd && normalized[trimmedStart] == ' ' {
			trimmedStart++
		}
		for trimmedEnd > trimmedStart && normalized[trimmedEnd-1] == ' ' {
			trimmedEnd--
		}
		if trimmedStart != trimmedEnd {
			lines = append(lines, claudeRawLine{
				value: normalized[trimmedStart:trimmedEnd],
				start: trimmedStart,
				end:   trimmedEnd,
			})
		}
		if end == len(normalized) {
			break
		}
		start = end + 1
	}
	return lines
}

func maskClaudeRawOccurrences(content, fixed []byte) {
	for offset := 0; offset <= len(content)-len(fixed); {
		index := bytes.Index(content[offset:], fixed)
		if index < 0 {
			return
		}
		index += offset
		clear(content[index : index+len(fixed)])
		offset = index + len(fixed)
	}
}

func normalizeClaudeLines(output []byte) []byte {
	normalized := make([]byte, 0, len(output))
	for index := 0; index < len(output); index++ {
		if output[index] == '\r' {
			normalized = append(normalized, '\n')
			if index+1 < len(output) && output[index+1] == '\n' {
				index++
			}
			continue
		}
		normalized = append(normalized, output[index])
	}
	return normalized
}

func validClaudeSetupToken(token []byte) bool {
	if len(token) < 8 || len(token) > maxClaudeTokenBytes {
		return false
	}
	for _, current := range token {
		if current < 0x21 || current > 0x7e {
			return false
		}
	}
	return true
}
