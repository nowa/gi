package ansiutil

type streamState uint8

const (
	streamText streamState = iota
	streamEscape
	streamEscapeIntermediate
	streamCSI
	streamOSC
	streamOSCEscape
	streamControlString
	streamControlStringEscape
)

// StreamStripper removes ANSI escape sequences while preserving ordinary
// bytes exactly, including UTF-8 sequences split across writes.
type StreamStripper struct {
	state streamState
}

func (s *StreamStripper) Write(input []byte) []byte {
	output := make([]byte, 0, len(input))
	for _, value := range input {
		switch s.state {
		case streamText:
			switch value {
			case 0x1b:
				s.state = streamEscape
			default:
				output = append(output, value)
			}
		case streamEscape:
			switch value {
			case '[':
				s.state = streamCSI
			case ']':
				s.state = streamOSC
			case 'P', 'X', '^', '_':
				s.state = streamControlString
			default:
				if value >= 0x20 && value <= 0x2f {
					s.state = streamEscapeIntermediate
				} else {
					s.state = streamText
				}
			}
		case streamEscapeIntermediate:
			if value >= 0x30 && value <= 0x7e {
				s.state = streamText
			}
		case streamCSI:
			if value >= 0x40 && value <= 0x7e {
				s.state = streamText
			}
		case streamOSC:
			switch value {
			case 0x07:
				s.state = streamText
			case 0x1b:
				s.state = streamOSCEscape
			}
		case streamOSCEscape:
			if value == '\\' {
				s.state = streamText
			} else if value != 0x1b {
				s.state = streamOSC
			}
		case streamControlString:
			if value == 0x1b {
				s.state = streamControlStringEscape
			}
		case streamControlStringEscape:
			if value == '\\' {
				s.state = streamText
			} else if value != 0x1b {
				s.state = streamControlString
			}
		}
	}
	return output
}
