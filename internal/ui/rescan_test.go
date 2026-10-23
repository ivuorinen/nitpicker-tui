package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

// rescannedFiles is the fixture after an audit ran: docs-00000003 was
// resolved away and two findings were filed, one in a new lens.
func rescannedFiles() fstest.MapFS {
	fsys := fixtureFiles()
	delete(fsys, "docs/open/docs-00000003.md")
	fsys["audit/open/audit-00000007.md"] = &fstest.MapFile{Data: []byte(
		"---\nid: audit-00000007\nauditor: audit\nseverity: critical\ncategory: correctness\n" +
			"status: open\nfound: 2026-10-05\n---\n\n# New race\n\nbody\n")}
	fsys["perf/open/perf-00000008.md"] = &fstest.MapFile{Data: []byte(
		"---\nid: perf-00000008\nauditor: perf\nseverity: low\nstatus: open\nfound: 2026-10-05\n---\n\n# Slow loop\n\nbody\n",
	)}

	return fsys
}

// reloadFrom returns a Reload that loads fsys, counting its calls.
func reloadFrom(t *testing.T, fsys fstest.MapFS, calls *int) Reload {
	t.Helper()

	return func() (findings.Store, error) {
		*calls++

		return findings.Load(fsys)
	}
}

func selectedID(m Model) string {
	if it, ok := m.list.SelectedItem().(item); ok {
		return it.f.ID
	}

	return ""
}

func TestRescanInFlight(t *testing.T) {
	t.Parallel()

	var calls int
	m := newReloadingModel(t, fixtureFiles(), reloadFrom(t, rescannedFiles(), &calls))

	m, cmd := step(t, m, keyMsg("r"))
	if cmd == nil {
		t.Fatal("r returned no command")
	}
	if !strings.Contains(m.list.Title, "rescanning…") {
		t.Errorf("title %q does not show the rescan", m.list.Title)
	}
	if _, again := step(t, m, keyMsg("r")); again != nil {
		t.Error("a second r started another rescan while one was running")
	}
	if calls != 0 {
		t.Errorf("reload ran %d times before its command did", calls)
	}
}

func TestRescanAppliesNewStore(t *testing.T) {
	t.Parallel()

	var calls int
	m := newReloadingModel(t, fixtureFiles(), reloadFrom(t, rescannedFiles(), &calls))
	m, _ = press(t, m, "/")
	m = typeText(t, m, "r")
	m, _ = press(t, m, "enter", "down")
	if got := selectedID(m); got != idAuditOpen {
		t.Fatalf("set-up: cursor on %q, want audit-00000001", got)
	}

	m, cmd := step(t, m, keyMsg("r"))
	m = settle(t, m, cmd)

	if calls != 1 {
		t.Errorf("reload ran %d times, want 1", calls)
	}
	// "r" still matches New race, Secret in logs, Token refresh race and
	// perf-00000008 (its id); newest critical first.
	want := []string{"audit-00000007", idSecurityOpen, idAuditOpen, "perf-00000008"}
	if got := visibleIDs(m); !slices.Equal(got, want) {
		t.Errorf("visible = %q, want %q", got, want)
	}
	if got := selectedID(m); got != idAuditOpen {
		t.Errorf("cursor on %q, want it kept on audit-00000001", got)
	}
	if m.query.Text != "r" || m.query.Sel[findings.State] != findings.StatusOpen {
		t.Errorf("query = %+v, want the search text and state filter kept", m.query)
	}
	if !strings.Contains(m.list.Title, "rescanned: 7 findings") {
		t.Errorf("title %q does not report the rescan", m.list.Title)
	}
}

func TestRescanClampsRemovedSelection(t *testing.T) {
	t.Parallel()

	var calls int
	m := newReloadingModel(t, fixtureFiles(), reloadFrom(t, rescannedFiles(), &calls))
	m, _ = press(t, m, "down", "down")
	if got := selectedID(m); got != "docs-00000003" {
		t.Fatalf("set-up: cursor on %q, want docs-00000003", got)
	}

	m, cmd := step(t, m, keyMsg("r"))
	m = settle(t, m, cmd)

	if got, n := m.list.Index(), len(visibleIDs(m)); got != 2 || n != 4 {
		t.Errorf("cursor index %d of %d rows, want 2 of 4 (clamped near the removed row)", got, n)
	}
}

func TestRescanErrorKeepsData(t *testing.T) {
	t.Parallel()

	failing := func() (findings.Store, error) { return findings.Store{}, errors.New("store vanished") }
	m := newReloadingModel(t, fixtureFiles(), failing)

	m, cmd := step(t, m, keyMsg("r"))
	m = settle(t, m, cmd)

	if got := len(visibleIDs(m)); got != 3 {
		t.Errorf("visible = %d findings after a failed rescan, want the old 3", got)
	}
	if !strings.Contains(m.list.Title, "rescan failed: store vanished") {
		t.Errorf("title %q does not show the failure", m.list.Title)
	}
	if m.rescanning {
		t.Error("still marked rescanning after the failure")
	}
}

func TestRTypedIntoSearchDoesNotRescan(t *testing.T) {
	t.Parallel()

	var calls int
	m := newReloadingModel(t, fixtureFiles(), reloadFrom(t, rescannedFiles(), &calls))
	m, _ = press(t, m, "/", "r")

	if calls != 0 || m.rescanning {
		t.Errorf("r in search rescanned (calls %d, rescanning %v)", calls, m.rescanning)
	}
	if m.search.Value() != "r" {
		t.Errorf("search value = %q, want %q", m.search.Value(), "r")
	}
}

func TestRescanKeepsPickerText(t *testing.T) {
	t.Parallel()

	var calls int
	m := newReloadingModel(t, fixtureFiles(), reloadFrom(t, rescannedFiles(), &calls))
	m, held := step(t, m, keyMsg("r"))
	m, _ = press(t, m, "l")
	m = typeText(t, m, "se")

	m = settle(t, m, held)

	if got := pickerTitles(m); !slices.Equal(got, []string{securityRow}) {
		t.Errorf("rows after the rescan = %q, want the typed filter kept: %q", got, securityRow)
	}
	m = typeText(t, m, "c")
	if got := pickerTitles(m); !slices.Equal(got, []string{securityRow}) {
		t.Errorf("rows after typing on = %q, want typing mode kept", got)
	}
}

func TestRescanRefreshesWarningsView(t *testing.T) {
	t.Parallel()

	damaged := fixtureFiles()
	damaged["audit/open/NIT-200.md"] = &fstest.MapFile{Data: []byte("no frontmatter\n")}
	var calls int
	m := newReloadingModel(t, fixtureFiles(), reloadFrom(t, damaged, &calls))
	m, held := step(t, m, keyMsg("r"))
	m, _ = press(t, m, "w")

	m = settle(t, m, held)

	if content := m.View().Content; !strings.Contains(content, "audit/open/NIT-200.md") {
		t.Errorf("warnings view after the rescan lacks the new warning: %q", content)
	}
}

func TestRescanRebuildsOpenPicker(t *testing.T) {
	t.Parallel()

	var calls int
	m := newReloadingModel(t, fixtureFiles(), reloadFrom(t, rescannedFiles(), &calls))
	m, cmd := step(t, m, keyMsg("r"))
	m, _ = press(t, m, "l")
	if m.mode != modePicker {
		t.Fatalf("set-up: mode = %v, want picker", m.mode)
	}

	m = settle(t, m, cmd)

	if !slices.Contains(pickerTitles(m), "  perf (1)") {
		t.Errorf("picker rows = %q, want the new perf lens", pickerTitles(m))
	}
}
