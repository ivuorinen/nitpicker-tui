package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func runArgs(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)

	return code, out.String(), errOut.String()
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	code, out, _ := runArgs("-version")
	if code != 0 || out != "dev\n" {
		t.Errorf("run -version = %d %q, want 0 %q", code, out, "dev\n")
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	code, _, errOut := runArgs("-h")
	if code != 0 || !strings.Contains(errOut, "usage: nitpicker-tui") {
		t.Errorf("run -h = %d %q, want 0 and the usage", code, errOut)
	}
}

func TestRunUsageErrors(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"a", "b"}, {"-nope"}} {
		code, _, errOut := runArgs(args...)
		if code != 2 || !strings.Contains(errOut, "usage: nitpicker-tui") {
			t.Errorf("run %q = %d %q, want 2 and the usage", args, code, errOut)
		}
	}
}

func TestRunWithoutStore(t *testing.T) {
	t.Parallel()

	code, _, errOut := runArgs(t.TempDir())
	if code != 1 || !strings.Contains(errOut, "no docs/audit/findings") {
		t.Errorf("run in a directory without a store = %d %q, want 1 and the reason", code, errOut)
	}
}

func TestFindStoreWalksUp(t *testing.T) {
	t.Parallel()

	got, err := findStore(filepath.Join("testdata", "store", "docs", "audit"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(filepath.Join("testdata", "store", "docs", "audit", "findings"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("findStore = %q, want %q", got, want)
	}
}

func TestFindStoreMissing(t *testing.T) {
	t.Parallel()

	if got, err := findStore(t.TempDir()); err == nil {
		t.Errorf("findStore in a temp dir = %q, want an error", got)
	}
}

// findings.py refuses a store reached through a symlink; a cloned repository
// must not point the browser at another directory on the machine.
func TestFindStoreRefusesSymlinkedComponent(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "outside", "docs", "audit", "findings"), 0o750); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside", "docs"), filepath.Join(project, "docs")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}

	got, err := findStore(project)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("findStore through a symlinked docs = %q, %v; want a symlink error", got, err)
	}
}

// A store path that cannot be resolved (here a symlink loop, ELOOP; an
// unreadable directory gives EACCES) is reported as such, not as missing.
func TestFindStoreReportsStatErrors(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	if err := os.Symlink(filepath.Join(project, "docs"), filepath.Join(project, "docs")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}

	got, err := findStore(project)
	if err == nil || strings.Contains(err.Error(), "no docs/audit/findings") {
		t.Errorf("findStore with a looping docs symlink = %q, %v; want the stat error", got, err)
	}
}

// A file named docs is not a store; the walk goes on to the parent.
func TestFindStoreSkipsFileNamedDocs(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "docs", "audit", "findings"), 0o750); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "child")
	writeFile(t, filepath.Join(child, "docs"), "not a directory\n")

	got, err := findStore(child)
	if err != nil || got != filepath.Join(parent, "docs", "audit", "findings") {
		t.Errorf("findStore = %q, %v; want the parent's store", got, err)
	}
}

func TestFixtureStoreLoads(t *testing.T) {
	t.Parallel()

	dir, err := findStore("testdata/store")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newLoader(dir)()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Findings) != 7 || len(s.Warnings) != 1 {
		t.Errorf("fixture: %d findings, %d warnings (%q), want 7 and 1", len(s.Findings), len(s.Warnings), s.Warnings)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func loadedIDs(t *testing.T, load func() ([]string, error)) []string {
	t.Helper()

	ids, err := load()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)

	return ids
}

// A rescan is another call of the same loader; it must see what an audit
// changed on disk in between.
func TestLoaderSeesChangesBetweenLoads(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "docs", "audit", "findings")
	ledger := filepath.Join(dir, "resolved.jsonl")
	resolved3 := `{"id":"audit-00000003","auditor":"audit","status":"fixed","title":"Three"}` + "\n"
	writeFile(t, filepath.Join(dir, "audit", "open", "audit-00000001.md"), "---\nid: audit-00000001\n---\n\n# One\n")
	writeFile(t, filepath.Join(dir, "audit", "open", "audit-00000002.md"), "---\nid: audit-00000002\n---\n\n# Two\n")
	writeFile(t, ledger, resolved3)

	load := newLoader(dir)
	ids := func() ([]string, error) {
		s, err := load()
		out := make([]string, 0, len(s.Findings))
		for _, f := range s.Findings {
			out = append(out, f.ID)
		}

		return out, err
	}
	if got := loadedIDs(t, ids); !slices.Equal(got, []string{"audit-00000001", "audit-00000002", "audit-00000003"}) {
		t.Fatalf("first load = %q", got)
	}

	// An audit resolves audit-00000002 (the ledger gains its record) and files
	// audit-00000004.
	writeFile(t, ledger, resolved3+`{"id":"audit-00000002","auditor":"audit","status":"fixed","title":"Two"}`+"\n")
	if err := os.Remove(filepath.Join(dir, "audit", "open", "audit-00000002.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "audit", "open", "audit-00000004.md"), "---\nid: audit-00000004\n---\n\n# Four\n")

	want := []string{"audit-00000001", "audit-00000002", "audit-00000003", "audit-00000004"}
	if got := loadedIDs(t, ids); !slices.Equal(got, want) {
		t.Errorf("second load = %q, want %q", got, want)
	}
}
