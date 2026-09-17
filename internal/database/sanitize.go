package database

import "unicode/utf8"

const (
	// unicodeEscapeLen is the length of a JSON \uXXXX escape.
	unicodeEscapeLen = 6
	// surrogatePairLen is the length of a high surrogate escape followed by a low one.
	surrogatePairLen = 2 * unicodeEscapeLen

	nulCodeUnit      = 0x0000
	highSurrogateMin = 0xD800
	highSurrogateMax = 0xDBFF
	lowSurrogateMin  = 0xDC00
	lowSurrogateMax  = 0xDFFF

	// replacementEscape replaces a refused escape. It keeps the repair visible in the
	// stored JSON and has the same length as the escape it replaces.
	replacementEscape = `\uFFFD`

	// repairedCapacityMargin leaves room for a few invalid UTF-8 bytes growing into a
	// three-byte U+FFFD without reallocating.
	repairedCapacityMargin = 8
)

// repairCounts records how many defects of each kind sanitizePayload replaced.
type repairCounts struct {
	nulEscapes           int
	loneSurrogates       int
	invalidUTF8Sequences int
}

// total returns the number of repairs of any kind.
func (r repairCounts) total() int {
	return r.nulEscapes + r.loneSurrogates + r.invalidUTF8Sequences
}

// add returns the sum of r and other.
func (r repairCounts) add(other repairCounts) repairCounts {
	return repairCounts{
		nulEscapes:           r.nulEscapes + other.nulEscapes,
		loneSurrogates:       r.loneSurrogates + other.loneSurrogates,
		invalidUTF8Sequences: r.invalidUTF8Sequences + other.invalidUTF8Sequences,
	}
}

// sanitizePayload replaces the content PostgreSQL refuses in a jsonb value, but that can
// be repaired without guessing, with U+FFFD: a JSON \u0000 escape, an unpaired UTF-16
// surrogate escape, and each maximal run of invalid UTF-8 bytes. Without it, one such
// message is discarded whole, although the rest of it is legitimate data.
//
// The defect is replaced rather than deleted so that the stored JSON still shows where
// something was lost. Everything PostgreSQL accepts is left alone, even when it looks
// suspicious: escaped controls \u0001-\u001F, noncharacters such as \uFFFF, valid
// surrogate pairs, and \\u0000, which is literal text. Numeric overflow and structural
// errors cannot be repaired and are left for the caller to reject.
//
// It is a byte transform, not a JSON parser: it accepts any input and never fails, so
// invalid JSON goes through and is rejected afterwards. Every backslash is taken as the
// start of an escape and consumes it, which is what makes \\u0000 literal text without
// counting backslashes. It runs on every message, so a payload needing no repair is
// returned as the same slice, without allocating; the output buffer is only created at
// the first defect.
func sanitizePayload(payload []byte) ([]byte, repairCounts) {
	var out []byte
	var counts repairCounts
	for i := 0; i < len(payload); {
		size, found := nextToken(payload, i)
		switch {
		case found.total() == 0:
			if out != nil {
				out = append(out, payload[i:i+size]...)
			}
		default:
			if out == nil {
				out = make([]byte, 0, len(payload)+repairedCapacityMargin)
				out = append(out, payload[:i]...)
			}
			if found.invalidUTF8Sequences > 0 {
				out = utf8.AppendRune(out, utf8.RuneError)
			} else {
				out = append(out, replacementEscape...)
			}
			counts = counts.add(found)
		}
		i += size
	}
	if out == nil {
		return payload, repairCounts{}
	}
	return out, counts
}

// nextToken returns the length of the token starting at payload[i], and the defect it
// holds, if any.
func nextToken(payload []byte, i int) (int, repairCounts) {
	switch {
	case payload[i] == '\\':
		return nextEscape(payload, i)
	case payload[i] < utf8.RuneSelf:
		return 1, repairCounts{}
	}
	return nextNonASCII(payload, i)
}

// nextEscape returns the length of the escape starting with the backslash at payload[i],
// and the defect it holds, if any.
func nextEscape(payload []byte, i int) (int, repairCounts) {
	codeUnit, ok := parseUnicodeEscape(payload[i:])
	if !ok {
		// A two-byte escape such as \\ or \", or a truncated \u. A non-ASCII byte after
		// the backslash is left to the next token, so that an invalid one is still
		// repaired.
		if i+1 < len(payload) && payload[i+1] < utf8.RuneSelf {
			return 2, repairCounts{}
		}
		return 1, repairCounts{}
	}

	switch {
	case codeUnit == nulCodeUnit:
		return unicodeEscapeLen, repairCounts{nulEscapes: 1}
	case highSurrogateMin <= codeUnit && codeUnit <= highSurrogateMax:
		next, ok := parseUnicodeEscape(payload[i+unicodeEscapeLen:])
		if ok && lowSurrogateMin <= next && next <= lowSurrogateMax {
			return surrogatePairLen, repairCounts{}
		}
		return unicodeEscapeLen, repairCounts{loneSurrogates: 1}
	case lowSurrogateMin <= codeUnit && codeUnit <= lowSurrogateMax:
		// A low surrogate preceded by a high one was consumed with it as a pair.
		return unicodeEscapeLen, repairCounts{loneSurrogates: 1}
	}
	return unicodeEscapeLen, repairCounts{}
}

// nextNonASCII returns the length of the UTF-8 sequence starting at payload[i], or of the
// maximal run of invalid bytes starting there, which counts as one defect.
func nextNonASCII(payload []byte, i int) (int, repairCounts) {
	if !invalidUTF8At(payload, i) {
		_, size := utf8.DecodeRune(payload[i:])
		return size, repairCounts{}
	}
	end := i + 1
	for end < len(payload) && invalidUTF8At(payload, end) {
		end++
	}
	return end - i, repairCounts{invalidUTF8Sequences: 1}
}

// invalidUTF8At reports whether payload[i] starts no valid UTF-8 sequence. A literal
// U+FFFD also decodes as utf8.RuneError, but with a size of three: only size one is a
// defect.
func invalidUTF8At(payload []byte, i int) bool {
	r, size := utf8.DecodeRune(payload[i:])
	return r == utf8.RuneError && size == 1
}

// parseUnicodeEscape decodes the \uXXXX escape at the start of b, with case-insensitive
// hex digits. The digits are decoded by hand: strconv would need a string conversion,
// which may allocate on every message.
func parseUnicodeEscape(b []byte) (rune, bool) {
	if len(b) < unicodeEscapeLen || b[0] != '\\' || b[1] != 'u' {
		return 0, false
	}
	var codeUnit rune
	for _, c := range b[2:unicodeEscapeLen] {
		nibble, ok := hexNibble(c)
		if !ok {
			return 0, false
		}
		codeUnit = codeUnit<<4 | nibble
	}
	return codeUnit, true
}

// hexNibble returns the value of the hex digit c.
func hexNibble(c byte) (rune, bool) {
	switch {
	case '0' <= c && c <= '9':
		return rune(c - '0'), true
	case 'a' <= c && c <= 'f':
		return rune(c-'a') + 0xa, true
	case 'A' <= c && c <= 'F':
		return rune(c-'A') + 0xa, true
	}
	return 0, false
}
