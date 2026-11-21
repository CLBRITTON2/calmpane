package ptyagent

// oscScanState is where withoutNonASCIIInOSC stopped in Claude's output, which an OSC string can span reads of.
type oscScanState int

const (
	outsideOSC oscScanState = iota
	afterEscOutsideOSC
	insideOSC
)

// withoutNonASCIIInOSC drops the bytes of 0x80 and above inside OSC strings. The ansi parser vt uses ends an OSC string
// at 0x9C, the C1 string terminator, even inside a UTF-8 character, so the rest of a title such as "✳ Claude Code"
// lands on the screen as text. Claude's terminal queries are ASCII and pass whole.
func withoutNonASCIIInOSC(output []byte, state oscScanState) ([]byte, oscScanState) {
	parsableOutput := make([]byte, 0, len(output))
	for _, outputByte := range output {
		switch {
		case state == insideOSC && outputByte >= 0x80:
			continue
		case state == insideOSC && (outputByte == 0x07 || outputByte == 0x18 || outputByte == 0x1a):
			state = outsideOSC
		case outputByte == 0x1b:
			state = afterEscOutsideOSC
		case state == afterEscOutsideOSC && outputByte == ']':
			state = insideOSC
		case state == afterEscOutsideOSC:
			state = outsideOSC
		}
		parsableOutput = append(parsableOutput, outputByte)
	}
	return parsableOutput, state
}
