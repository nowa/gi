package gitui

import "strings"

func c1Prefix(data string) (byte, int, bool) {
	if data == "" {
		return 0, 0, false
	}
	if len(data) >= 2 && data[0] == 0xc2 && data[1] >= 0x80 && data[1] <= 0x9f {
		return data[1], 2, true
	}
	if data[0] >= 0x80 && data[0] <= 0x9f {
		return data[0], 1, true
	}
	return 0, 0, false
}

func controlStringTerminator(data string, start int, allowBEL bool) (int, int, bool) {
	payloadEnd := -1
	sequenceEnd := 0
	consider := func(candidatePayloadEnd, candidateSequenceEnd int) {
		if candidateSequenceEnd <= 0 {
			return
		}
		if sequenceEnd == 0 || candidateSequenceEnd < sequenceEnd || (candidateSequenceEnd == sequenceEnd && candidatePayloadEnd < payloadEnd) {
			payloadEnd = candidatePayloadEnd
			sequenceEnd = candidateSequenceEnd
		}
	}
	if idx := strings.Index(data[start:], "\xc2\x9c"); idx >= 0 {
		end := start + idx
		consider(end, end+2)
	}
	if idx := strings.IndexByte(data[start:], 0x9c); idx >= 0 {
		end := start + idx
		payloadEnd := end
		if end > start && data[end-1] == 0xc2 {
			payloadEnd = end - 1
		}
		consider(payloadEnd, end+1)
	}
	if idx := strings.Index(data[start:], "\x1b\\"); idx >= 0 {
		end := start + idx
		consider(end, end+len("\x1b\\"))
	}
	if allowBEL {
		if idx := strings.IndexByte(data[start:], '\x07'); idx >= 0 {
			end := start + idx
			consider(end, end+1)
		}
	}
	return payloadEnd, sequenceEnd, sequenceEnd > 0
}

func c1StringTerminatedLength(data string, start int, allowBEL bool) int {
	if _, sequenceEnd, ok := controlStringTerminator(data, start, allowBEL); ok {
		return sequenceEnd
	}
	return 0
}

func isColonExtendedColorField(field string) bool {
	return strings.HasPrefix(field, "38:") || strings.HasPrefix(field, "48:") || strings.HasPrefix(field, "58:")
}

func sgrCode(part string) string {
	if idx := strings.IndexByte(part, ':'); idx >= 0 {
		return part[:idx]
	}
	return part
}
