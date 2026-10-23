package findings

import (
	"maps"
	"testing"
)

const (
	keyID       = "id"
	keySeverity = "severity"
)

// The expected values are hand-derived from findings.py parse_frontmatter and
// parse_finding (nitpicker 3.1.0), the parser this one must agree with.
func TestParseFinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		wantFM    map[string]string
		wantTitle string
		wantBody  string
		wantOK    bool
	}{
		{
			name: "plain frontmatter",
			in: "---\nid: audit-0123abcd\nauditor: audit\nseverity: high\n---\n\n" +
				"# Broken thing\n\n## Problem\nIt breaks.\n",
			wantFM:    map[string]string{keyID: "audit-0123abcd", "auditor": lensAudit, keySeverity: severityHigh},
			wantTitle: "Broken thing",
			wantBody:  "## Problem\nIt breaks.\n",
			wantOK:    true,
		},
		{
			name:      "CRLF input",
			in:        "---\r\nid: x-1\r\narea: a.go\r\n---\r\n# T\r\nbody\r\n",
			wantFM:    map[string]string{keyID: "x-1", "area": "a.go"},
			wantTitle: "T",
			wantBody:  "body\n",
			wantOK:    true,
		},
		{
			name:      "one matching quote pair stripped",
			in:        "---\narea: \"src/a.go\"\ncategory: 'docs'\ntitle: \"\"x\"\"\nmixed: \"y'\n---\n# T\n",
			wantFM:    map[string]string{"area": "src/a.go", "category": catDocs, "title": "\"x\"", "mixed": "\"y'"},
			wantTitle: "T",
			wantBody:  "\n",
			wantOK:    true,
		},
		{
			name:      "indented and separator-less lines ignored",
			in:        "---\nid: top\n  id: nested\n\tsev: tab\nnoseparator\nkey:novalue\n---\n# T\n",
			wantFM:    map[string]string{keyID: "top"},
			wantTitle: "T",
			wantBody:  "\n",
			wantOK:    true,
		},
		{
			// \xe2\x80\xa8 is U+2028 LINE SEPARATOR, a line boundary to Python.
			name:      "python splitlines separators split frontmatter lines",
			in:        "---\nid: a\xe2\x80\xa8severity: low\n---\n# T\n",
			wantFM:    map[string]string{keyID: "a", keySeverity: severityLow},
			wantTitle: "T",
			wantBody:  "\n",
			wantOK:    true,
		},
		{
			name:      "no closing delimiter",
			in:        "---\nid: x\n# T\nbody\n",
			wantFM:    map[string]string{},
			wantTitle: "T",
			wantBody:  "---\nid: x\nbody\n",
			wantOK:    false,
		},
		{
			name: "mdformat-mangled frontmatter",
			in: "______________________________________________________________________\n\n" +
				"## id: NIT-119 auditor: audit severity: advisory\n\n# .goreleaser.yaml use\n\nText\n",
			wantFM:    map[string]string{},
			wantTitle: ".goreleaser.yaml use",
			wantBody: "______________________________________________________________________\n\n" +
				"## id: NIT-119 auditor: audit severity: advisory\n\n\nText\n",
			wantOK: false,
		},
		{
			name:      "title is the first h1, not an h2",
			in:        "---\nid: x\n---\n## Not title\n# Real title\n# Second h1 stays\n",
			wantFM:    map[string]string{keyID: "x"},
			wantTitle: "Real title",
			wantBody:  "## Not title\n# Second h1 stays\n",
			wantOK:    true,
		},
		{
			// "\r\r\n" leaves "\r\n" after the CRLF replace; splitlines reads
			// that as one break.
			name:      "carriage return before CRLF",
			in:        "---\nid: x\n---\n# T\nline1\r\r\nline2\n",
			wantFM:    map[string]string{keyID: "x"},
			wantTitle: "T",
			wantBody:  "line1\nline2\n",
			wantOK:    true,
		},
		{
			// Python's str.strip also strips U+001C-U+001F.
			name:      "unit separator trimmed like strip",
			in:        "---\nseverity: \"high\"\x1f\n---\n# T\n",
			wantFM:    map[string]string{keySeverity: severityHigh},
			wantTitle: "T",
			wantBody:  "\n",
			wantOK:    true,
		},
		{
			name:      "body trimmed to exactly one trailing newline",
			in:        "---\nid: x\n---\n\n\n# T\n\n\nline\n\n\n\n",
			wantFM:    map[string]string{keyID: "x"},
			wantTitle: "T",
			wantBody:  "line\n",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fm, title, body, ok := parseFinding(tt.in)
			if !maps.Equal(fm, tt.wantFM) {
				t.Errorf("frontmatter = %q, want %q", fm, tt.wantFM)
			}
			if title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}
