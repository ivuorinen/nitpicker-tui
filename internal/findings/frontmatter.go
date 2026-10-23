package findings

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The parsers below mirror parse_frontmatter and parse_finding in nitpicker's
// findings.py (3.1.0). A file the two read differently is a file this browser
// misreports, so a store-format change there means a change here, with tests.

const fmDelim = "\n---\n"

// Line boundaries Python's str.splitlines knows beyond the ASCII ones.
const (
	nextLine           = 0x85
	lineSeparator      = 0x2028
	paragraphSeparator = 0x2029
)

// parseFrontmatter splits text into its frontmatter keys and the body after
// the closing delimiter. ok is false when the file has no frontmatter block;
// the body is then the whole text.
func parseFrontmatter(text string) (fm map[string]string, body string, ok bool) {
	fm = map[string]string{}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return fm, text, false
	}
	end := strings.Index(rest, fmDelim)
	if end < 0 {
		return fm, text, false
	}
	for _, line := range splitLines(rest[:end]) {
		// Indented lines are nested values, not top-level keys.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, found := strings.Cut(line, ": ")
		if !found {
			continue
		}
		fm[pyStrip(key)] = unquote(pyStrip(value))
	}

	return fm, rest[end+len(fmDelim):], true
}

// parseFinding returns the frontmatter, the title (the first "# " line) and
// the rest of the body, trimmed and ending in one newline.
func parseFinding(text string) (fm map[string]string, title, body string, ok bool) {
	fm, body, ok = parseFrontmatter(text)
	var rest []string
	for _, line := range splitLines(body) {
		if title == "" && strings.HasPrefix(line, "# ") {
			title = pyStrip(line[2:])

			continue
		}
		rest = append(rest, line)
	}

	return fm, title, pyStrip(strings.Join(rest, "\n")) + "\n", ok
}

// pyStrip trims like Python's str.strip, which also counts U+001C-U+001F as
// whitespace; strings.TrimSpace does not.
func pyStrip(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f
	})
}

// unquote strips one matching pair of surrounding quotes.
func unquote(v string) string {
	if len(v) >= 2 && v[0] == v[len(v)-1] && (v[0] == '"' || v[0] == '\'') {
		return v[1 : len(v)-1]
	}

	return v
}

// splitLines splits like Python's str.splitlines, which findings.py uses: on
// every Unicode line boundary, "\r\n" counting as one, keeping empty lines,
// with no trailing empty line for a final separator. The caller's CRLF
// replace does not remove every "\r\n": "\r\r\n" leaves one behind.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if i < start {
			// The "\n" of a "\r\n" already consumed.
			continue
		}
		if isLineBreak(r) {
			lines = append(lines, s[start:i])
			start = i + utf8.RuneLen(r)
			if r == '\r' && strings.HasPrefix(s[start:], "\n") {
				start++
			}
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

func isLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', '\x1c', '\x1d', '\x1e', nextLine, lineSeparator, paragraphSeparator:
		return true
	}

	return false
}
