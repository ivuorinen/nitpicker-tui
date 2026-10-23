package findings

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sanitize removes what a terminal would act on rather than display: every
// control character except tab and newline (C0, DEL and the C1 range, which
// holds the 8-bit CSI), the bidi formatting characters, and every byte that
// is not valid UTF-8.
//
// Finding text quotes repository content, and glamour passes escape sequences
// such as an OSC 52 clipboard write straight through, so every string read
// from the store goes through here once, at load. Bidi overrides and isolates
// are format characters (Cf), not controls, but a bidi-capable terminal uses
// them to reorder what it draws, so a title could show text it does not hold.
//
// This covers text as stored. Markdown character references (&#x1b;) are
// decoded later, by the renderer, so rendered output has its own filter.
func Sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		invalid := r == utf8.RuneError && size == 1
		if !invalid && !isBidiControl(r) && (r == '\t' || r == '\n' || !unicode.IsControl(r)) {
			b.WriteString(s[i : i+size])
		}
		i += size
	}

	return b.String()
}

// isBidiControl reports the Unicode bidi formatting characters: the Arabic
// letter mark, the left-to-right and right-to-left marks, the embeddings and
// overrides with their pop, and the isolates with theirs.
func isBidiControl(r rune) bool {
	switch {
	case r == 0x061c, r == 0x200e, r == 0x200f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	}

	return false
}
