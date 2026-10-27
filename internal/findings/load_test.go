package findings

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func openFile(fm, title, body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("---\n" + fm + "---\n\n# " + title + "\n\n" + body)}
}

// stripped drops the unexported search text so findings compare by their
// visible fields.
func stripped(fs []Finding) []Finding {
	out := make([]Finding, len(fs))
	for i, f := range fs {
		f.haystack = ""
		out[i] = f
	}

	return out
}

func mustLoad(t *testing.T, fsys fs.FS) Store {
	t.Helper()

	s, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	return s
}

func assertFindings(t *testing.T, got, want []Finding) {
	t.Helper()

	got = stripped(got)
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("finding %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestLoadOpenFindings(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"audit/open/audit-11111111.md": openFile(
			"id: audit-11111111\nauditor: audit\nseverity: high\ncategory: correctness\n"+
				"area: src/a.go\nstatus: open\nfound: 2026-10-01\nlocation: src/a.go:10-12\n",
			"Off by one", "## Problem\nLoop ends early.\n"),
		"security/open/security-22222222.md": openFile(
			"id: security-22222222\nauditor: security\nseverity: critical\ncategory: security\n"+
				"area: api/\nstatus: invalid\nfound: 2026-09-30\n",
			"Token in log", "## Problem\nLeaks.\n"),
		// Every finding of this auditor is resolved, so it has no open/ dir.
		"docs/.keep": {},
		"INDEX.md":   {Data: []byte("# Audit Findings Index\n")},
		".lock":      {},
	}

	s := mustLoad(t, fsys)

	assertFindings(t, s.Findings, []Finding{
		{
			ID: "audit-11111111", Auditor: lensAudit, Severity: severityHigh, Category: catCorrectness,
			Area: "src/a.go", Status: StatusOpen, Found: "2026-10-01", Location: "src/a.go:10-12",
			Title: "Off by one", Body: "## Problem\nLoop ends early.\n", Path: "audit/open/audit-11111111.md",
		},
		{
			// An open file is open whatever its frontmatter says, as in findings.py list.
			ID: "security-22222222", Auditor: lensSecurity, Severity: severityCritical, Category: catSecurity,
			Area: "api/", Status: StatusOpen, Found: "2026-09-30",
			Title: "Token in log", Body: "## Problem\nLeaks.\n", Path: "security/open/security-22222222.md",
		},
	})
	if len(s.Warnings) != 0 {
		t.Errorf("warnings = %q, want none", s.Warnings)
	}
}

func TestLoadLedger(t *testing.T) {
	t.Parallel()

	ledger := `{"id":"ci-33333333","auditor":"ci","severity":"low","category":"","area":".github/",` +
		`"status":"fixed","found":"2026-08-01","resolved":"2026-08-02","title":"Unpinned action",` +
		`"body":"## Problem\nx\n","extra":{"location":".github/ci.yml:3"}}` + "\n" +
		`{"id": not json` + "\n" +
		"\n" +
		`{"id":"N-001","auditor":"audit","severity":"medium","category":"docs","area":"README.md",` +
		`"status":"invalid","found":"2026-07-01","resolved":"2026-07-03","title":"Stale","body":"b\n"}` + "\n"

	s := mustLoad(t, fstest.MapFS{LedgerName: {Data: []byte(ledger)}})

	assertFindings(t, s.Findings, []Finding{
		{
			ID: "ci-33333333", Auditor: lensCI, Severity: severityLow, Area: ".github/", Status: statusFixed,
			Found: "2026-08-01", Resolved: "2026-08-02", Location: ".github/ci.yml:3",
			Title: "Unpinned action", Body: "## Problem\nx\n", Path: LedgerName,
		},
		{
			ID: "N-001", Auditor: lensAudit, Severity: severityMedium, Category: catDocs, Area: "README.md",
			Status: statusInvalid, Found: "2026-07-01", Resolved: "2026-07-03",
			Title: "Stale", Body: "b\n", Path: LedgerName,
		},
	})
	if len(s.Warnings) != 1 || !strings.HasPrefix(s.Warnings[0], LedgerName+":2: ") {
		t.Errorf("warnings = %q, want one for %s:2", s.Warnings, LedgerName)
	}
}

// A bufio.Scanner stops at 64 KiB per line; the ledger has no such limit.
func TestLoadLedgerLongLine(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 100*1024)
	line := `{"id":"a-44444444","auditor":"a","status":"fixed","title":"Long","body":"` + body + `"}` + "\n"

	s := mustLoad(t, fstest.MapFS{LedgerName: {Data: []byte(line)}})

	if len(s.Findings) != 1 || len(s.Findings[0].Body) != len(body) {
		t.Fatalf("got %d findings (warnings %q), want one with a %d-byte body", len(s.Findings), s.Warnings, len(body))
	}
}

func TestLoadSanitizesDecodedEscapes(t *testing.T) {
	t.Parallel()

	line := `{"id":"a-55555555","auditor":"a","status":"fixed","title":"T\u001b[2J",` +
		`"body":"see \u001b]52;c;ZXZpbA==\u0007 here"}` + "\n"
	evil := "---\nid: b-66666666\narea: x\x1b]0;title\x07\n---\n# Open \x1b[31mred\n\nbody\x9b\n"

	s := mustLoad(t, fstest.MapFS{
		LedgerName:               {Data: []byte(line)},
		"b/open/b-66666666.md":   {Data: []byte(evil)},
		"b/open/\x1bname-1.md":   openFile("id: c-77777777\n", "Named", "x\n"),
		"b/open/\x1bname-1.md.x": {},
	})

	for _, f := range s.Findings {
		for _, v := range []string{f.ID, f.Title, f.Body, f.Area, f.Path, f.haystack} {
			if strings.ContainsAny(v, "\x1b\x07\u009b") {
				t.Errorf("finding %s keeps a control character: %q", f.ID, v)
			}
		}
	}
	if len(s.Findings) != 3 {
		t.Errorf("got %d findings, want 3", len(s.Findings))
	}
}

func TestLoadDamagedAndIgnoredFiles(t *testing.T) {
	t.Parallel()

	mangled := "______________________________________________________________________\n\n" +
		"## id: NIT-119 auditor: audit severity: advisory\n\n# .goreleaser.yaml use\n\nText\n"
	s := mustLoad(t, fstest.MapFS{
		"audit/open/NIT-119.md":  {Data: []byte(mangled)},
		"audit/open/notes.txt":   {Data: []byte("not a finding")},
		"audit/open/sub/deep.md": openFile("id: deep-1\n", "Too deep", "x\n"),
	})

	assertFindings(t, s.Findings, []Finding{{
		ID: "NIT-119", Auditor: lensAudit, Status: StatusOpen, Title: ".goreleaser.yaml use",
		Body: "______________________________________________________________________\n\n" +
			"## id: NIT-119 auditor: audit severity: advisory\n\n\nText\n",
		Path: "audit/open/NIT-119.md",
	}})
	if len(s.Warnings) != 1 || !strings.HasPrefix(s.Warnings[0], "audit/open/NIT-119.md: ") {
		t.Errorf("warnings = %q, want one for audit/open/NIT-119.md", s.Warnings)
	}
}

func TestLoadSkipsSymlinks(t *testing.T) {
	t.Parallel()

	s := mustLoad(t, fstest.MapFS{
		LedgerName:           {Data: []byte("/etc/passwd"), Mode: fs.ModeSymlink},
		"evil":               {Data: []byte("/tmp"), Mode: fs.ModeSymlink},
		"audit/open/link.md": {Data: []byte("../../outside.md"), Mode: fs.ModeSymlink},
	})

	if len(s.Findings) != 0 {
		t.Errorf("findings = %+v, want none", stripped(s.Findings))
	}
	wantWarn := []string{"audit/open/link.md: ", "evil: ", LedgerName + ": "}
	if len(s.Warnings) != len(wantWarn) {
		t.Fatalf("warnings = %q, want %d", s.Warnings, len(wantWarn))
	}
	for i, prefix := range wantWarn {
		if !strings.HasPrefix(s.Warnings[i], prefix) {
			t.Errorf("warning %d = %q, want prefix %q", i, s.Warnings[i], prefix)
		}
	}
}

func TestLoadSkipsSymlinkedOpenDir(t *testing.T) {
	t.Parallel()

	s := mustLoad(t, fstest.MapFS{
		"a/open":         {Data: []byte("../b/open"), Mode: fs.ModeSymlink},
		"b/open/b-1.md":  openFile("id: b-1\n", "Real", "x\n"),
		"b/open/.keep":   {},
		"a/open-not-dir": {},
	})

	if got := stripped(s.Findings); len(got) != 1 || got[0].Path != "b/open/b-1.md" {
		t.Errorf("findings = %+v, want only b/open/b-1.md", got)
	}
	if len(s.Warnings) != 1 || !strings.HasPrefix(s.Warnings[0], "a/open: ") {
		t.Errorf("warnings = %q, want one for a/open", s.Warnings)
	}
}

// Single-line fields are drawn as one list row each; a newline in one would
// draw a fake row.
func TestLoadFlattensSingleLineFields(t *testing.T) {
	t.Parallel()

	line := `{"id":"a-1\nx","auditor":"a","status":"fixed","area":"p\tq","title":"one\ntwo\tthree",` +
		`"body":"keep\nlines"}` + "\n"
	s := mustLoad(t, fstest.MapFS{LedgerName: {Data: []byte(line)}})

	f := s.Findings[0]
	if f.Title != "one two three" || f.ID != "a-1 x" || f.Area != "p q" {
		t.Errorf("fields = %q %q %q, want newlines and tabs replaced by spaces", f.ID, f.Title, f.Area)
	}
	if f.Body != "keep\nlines" {
		t.Errorf("body = %q, want its newline kept", f.Body)
	}
}

// Expected values are findings.py's (3.1.0) reading of the same ledger.
func TestLoadLedgerParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ledger    string
		wantIDs   []string
		wantField func(Finding) string
		want      string
		wantWarn  int
	}{
		{
			name:     "null line is not a record",
			ledger:   "null\n" + `{"id":"a","status":"fixed"}` + "\n",
			wantIDs:  []string{"a"},
			wantWarn: 1,
		},
		{
			name:      "last record per id wins",
			ledger:    `{"id":"a","title":"first"}` + "\n" + `{"id":"a","title":"second"}` + "\n",
			wantIDs:   []string{"a"},
			wantField: func(f Finding) string { return f.Title },
			want:      "second",
		},
		{
			name:      "top-level location",
			ledger:    `{"id":"a","location":"x.go:3"}` + "\n",
			wantIDs:   []string{"a"},
			wantField: func(f Finding) string { return f.Location },
			want:      "x.go:3",
		},
		{
			name:      "extra location still read",
			ledger:    `{"id":"a","extra":{"location":"y.go:4"}}` + "\n",
			wantIDs:   []string{"a"},
			wantField: func(f Finding) string { return f.Location },
			want:      "y.go:4",
		},
		{
			name:    "carriage-return separated records",
			ledger:  `{"id":"a"}` + "\r" + `{"id":"b"}` + "\r",
			wantIDs: []string{"a", "b"},
		},
		{
			name:      "keys match exactly",
			ledger:    `{"ID":"a","STATUS":"fixed","id":"b"}` + "\n",
			wantIDs:   []string{"b"},
			wantField: func(f Finding) string { return f.Status },
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := mustLoad(t, fstest.MapFS{LedgerName: {Data: []byte(tt.ledger)}})
			got := make([]string, 0, len(s.Findings))
			for _, f := range s.Findings {
				got = append(got, f.ID)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantIDs, ",") {
				t.Errorf("ids = %q, want %q", got, tt.wantIDs)
			}
			if tt.wantField != nil && len(s.Findings) > 0 {
				if v := tt.wantField(s.Findings[0]); v != tt.want {
					t.Errorf("field = %q, want %q", v, tt.want)
				}
			}
			if len(s.Warnings) != tt.wantWarn {
				t.Errorf("warnings = %q, want %d", s.Warnings, tt.wantWarn)
			}
		})
	}
}

// findings.py uses the file name only when the id key is missing.
func TestLoadEmptyIDKeyKept(t *testing.T) {
	t.Parallel()

	s := mustLoad(t, fstest.MapFS{"a/open/x-1.md": openFile("id: \"\"\n", "T", "b\n")})
	if len(s.Findings) != 1 || s.Findings[0].ID != "" {
		t.Errorf("findings = %+v, want one with id \"\"", stripped(s.Findings))
	}
}

func TestLoadEmptyStore(t *testing.T) {
	t.Parallel()

	s := mustLoad(t, fstest.MapFS{})
	if len(s.Findings) != 0 || len(s.Warnings) != 0 {
		t.Errorf("got %+v, want an empty store", s)
	}
}

func TestLoadUnreadableRoot(t *testing.T) {
	t.Parallel()

	if _, err := Load(os.DirFS(filepath.Join(t.TempDir(), "missing"))); err == nil {
		t.Error("Load of a missing directory returned no error")
	}
}

// escapingStore builds base/store whose ledger and one auditor directory are
// symlinks to base/outside, which holds findings titled LEAKED.
func escapingStore(t *testing.T, base string) string {
	t.Helper()

	outside := filepath.Join(base, "outside")
	store := filepath.Join(base, "store")
	for _, dir := range []string{filepath.Join(outside, "open"), store} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	leakedRecord := `{"id":"x-88888888","auditor":"x","status":"fixed","title":"LEAKED"}` + "\n"
	files := map[string]string{
		filepath.Join(outside, "ledger"):         leakedRecord,
		filepath.Join(outside, "open", "x-1.md"): "---\nid: x-1\n---\n# LEAKED\n",
	}
	for name, data := range files {
		if err := os.WriteFile(name, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		filepath.Join(store, LedgerName): filepath.Join(outside, "ledger"),
		filepath.Join(store, "x"):        outside,
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("cannot create symlinks here: %v", err)
		}
	}

	return store
}

// The store is read through os.Root in production. A committed symlink that
// points outside the store must never bring outside content into a finding.
func TestLoadOnDiskSymlinkEscape(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(escapingStore(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	s := mustLoad(t, root.FS())
	for _, f := range s.Findings {
		if strings.Contains(f.Title+f.Body, "LEAKED") {
			t.Errorf("outside content loaded: %+v", f)
		}
	}
	if len(s.Warnings) != 2 {
		t.Errorf("warnings = %q, want one per symlink", s.Warnings)
	}
}

// errInjected is the read error failFS returns.
var errInjected = errors.New("injected read error")

// failFS is a MapFS whose named files fail to read, for the read errors MapFS
// cannot produce. fs.ReadFile prefers a ReadFileFS, so ReadFile is enough.
type failFS struct {
	fstest.MapFS

	fail map[string]bool
}

func (f failFS) ReadFile(name string) ([]byte, error) {
	if f.fail[name] {
		return nil, errInjected
	}

	return f.MapFS.ReadFile(name)
}

func storeIDs(s Store) []string {
	ids := make([]string, len(s.Findings))
	for i, f := range s.Findings {
		ids[i] = f.ID
	}

	return ids
}

// A file that cannot be read is a warning naming it, and every other finding
// still loads.
func TestLoadReportsUnreadableFiles(t *testing.T) {
	t.Parallel()

	s := mustLoad(t, failFS{
		MapFS: fstest.MapFS{
			"a/open/A-1.md": openFile("id: A-1\n", "One", ""),
			"a/open/A-2.md": openFile("id: A-2\n", "Two", ""),
			LedgerName:      {Data: []byte(`{"id":"R-1","status":"fixed"}` + "\n")},
		},
		fail: map[string]bool{"a/open/A-1.md": true, LedgerName: true},
	})

	if got := storeIDs(s); !slices.Equal(got, []string{"A-2"}) {
		t.Errorf("findings = %q, want only A-2", got)
	}
	want := []string{"a/open/A-1.md: injected read error", "resolved.jsonl: injected read error"}
	if !slices.Equal(s.Warnings, want) {
		t.Errorf("warnings = %q, want %q", s.Warnings, want)
	}
}

// An <auditor>/open that is a file, not a directory, is a warning.
func TestLoadReportsOpenThatIsNotADirectory(t *testing.T) {
	t.Parallel()

	s := mustLoad(t, fstest.MapFS{"a/open": {Data: []byte("x")}})
	if len(s.Findings) != 0 || len(s.Warnings) != 1 || !strings.HasPrefix(s.Warnings[0], "a/open: ") {
		t.Errorf("findings %d, warnings %q; want none and one about a/open", len(s.Findings), s.Warnings)
	}
}

// A ledger that is not a regular file is skipped: a FIFO would block the read.
func TestLoadSkipsIrregularLedger(t *testing.T) {
	t.Parallel()

	s := mustLoad(t, fstest.MapFS{LedgerName: {Mode: fs.ModeNamedPipe}})
	want := []string{"resolved.jsonl: not a regular file; skipped"}
	if len(s.Findings) != 0 || !slices.Equal(s.Warnings, want) {
		t.Errorf("findings %d, warnings %q; want none and %q", len(s.Findings), s.Warnings, want)
	}
}

// A ledger field of the wrong JSON type is a warning for its line; the other
// lines still load.
func TestLoadLedgerWrongFieldTypes(t *testing.T) {
	t.Parallel()

	ledger := `{"id":5}` + "\n" +
		`{"id":"R-2","extra":5}` + "\n" +
		`{"id":"R-3","extra":{"location":5}}` + "\n" +
		`{"id":"R-4","status":"fixed"}` + "\n"
	s := mustLoad(t, fstest.MapFS{LedgerName: {Data: []byte(ledger)}})

	if got := storeIDs(s); !slices.Equal(got, []string{"R-4"}) {
		t.Errorf("findings = %q, want only R-4", got)
	}
	prefixes := []string{
		`resolved.jsonl:1: field "id": `,
		`resolved.jsonl:2: extra: `,
		`resolved.jsonl:3: extra: field "location": `,
	}
	if len(s.Warnings) != len(prefixes) {
		t.Fatalf("warnings = %q, want %d", s.Warnings, len(prefixes))
	}
	for i, p := range prefixes {
		if !strings.HasPrefix(s.Warnings[i], p) {
			t.Errorf("warning %d = %q, want the prefix %q", i+1, s.Warnings[i], p)
		}
	}
}
