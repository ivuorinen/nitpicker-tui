package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

const (
	// securityRow is the security lens with one open finding.
	securityRow = "  security (1)"
	// allOpenRow is the current "all" row while the three open findings show.
	allOpenRow = "✓ all (3)"
)

// pickerTitles lists the picker rows currently visible.
func pickerTitles(m Model) []string {
	var out []string
	for _, it := range m.picker.list.VisibleItems() {
		if pi, ok := it.(pickerItem); ok {
			out = append(out, pi.Title())
		}
	}

	return out
}

func selectedTitle(m Model) string {
	if pi, ok := m.picker.list.SelectedItem().(pickerItem); ok {
		return pi.Title()
	}

	return ""
}

func TestLensPickerOpensWithCounts(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "l")

	if m.mode != modePicker {
		t.Fatalf("mode = %v, want picker", m.mode)
	}
	// Counts follow the active state filter (open); ci has only resolved findings.
	want := []string{allOpenRow, "  audit (1)", "  ci (0)", "  docs (1)", securityRow}
	if got := pickerTitles(m); !slices.Equal(got, want) {
		t.Errorf("picker rows = %q, want %q", got, want)
	}
	if got := selectedTitle(m); got != allOpenRow {
		t.Errorf("cursor on %q, want the current value", got)
	}
}

// While filtering, the list draws its filter prompt where its title would
// be, so the prompt is what tells the user which facet the picker sets.
func TestPickerNamesItsFacet(t *testing.T) {
	t.Parallel()

	for k, name := range map[string]string{"s": "state: ", "l": "lens: ", "v": "severity: ", "c": "category: "} {
		m, _ := press(t, newTestModel(t, fixtureFiles()), k)
		if content := m.View().Content; !strings.Contains(content, name) {
			t.Errorf("picker for %q does not name its facet %q", k, name)
		}
	}
}

func TestPickerTypeToNarrowAndApply(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "l")
	m = typeText(t, m, "sec")

	if got := pickerTitles(m); !slices.Equal(got, []string{securityRow}) {
		t.Fatalf("after typing sec, rows = %q, want only security", got)
	}

	m, _ = press(t, m, "enter")
	if m.mode != modeList {
		t.Fatalf("mode = %v after enter, want list", m.mode)
	}
	if got := visibleIDs(m); !slices.Equal(got, []string{idSecurityOpen}) {
		t.Errorf("visible = %q, want only security-00000002", got)
	}
	if !strings.Contains(m.list.Title, "lens: security") {
		t.Errorf("title %q lacks the lens filter", m.list.Title)
	}
}

// The state picker opens with the cursor on "open" (row 1); typing must not
// leave it past the end of the narrowed rows.
func TestPickerEnterAfterNarrowingFromSelectedValue(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "s")
	m = typeText(t, m, "fix")
	m, _ = press(t, m, "enter")

	if m.mode != modeList {
		t.Fatalf("mode = %v after enter, want list", m.mode)
	}
	if got := m.query.Sel[findings.State]; got != "fixed" {
		t.Errorf("state = %q, want fixed", got)
	}
	if got := len(visibleIDs(m)); got != 2 {
		t.Errorf("visible = %d findings, want the 2 fixed ones", got)
	}
}

// Filter results computed for a picker that was closed must not fill the
// next one: here the lens picker's results arrive after the category picker
// opened.
func TestPickerDropsResultsOfAClosedPicker(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "l")
	m, held := step(t, m, keyMsg("s"))
	m, _ = press(t, m, "esc", "c")
	before := pickerTitles(m)

	m = settle(t, m, held)

	if got := pickerTitles(m); !slices.Equal(got, before) {
		t.Errorf("category picker rows = %q after a stale lens result, want %q", got, before)
	}
}

// The list's own AcceptWhileFiltering keys (tab, shift+tab, ctrl+k, ctrl+j)
// must not end typing mode.
func TestPickerKeepsNarrowingAfterTab(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "l")
	m = typeText(t, m, "s")
	m, _ = press(t, m, "tab")
	m = typeText(t, m, "e")

	if got := pickerTitles(m); !slices.Equal(got, []string{securityRow}) {
		t.Errorf("rows after s, tab, e = %q, want only %q", got, securityRow)
	}
}

func TestPickerEscLeavesFilterUnchanged(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "l")
	m = typeText(t, m, "sec")
	m, _ = press(t, m, "esc")

	if m.mode != modeList {
		t.Fatalf("mode = %v after esc, want list", m.mode)
	}
	if _, ok := m.query.Sel[findings.Lens]; ok {
		t.Errorf("esc set a lens: %v", m.query.Sel)
	}
	if got := len(visibleIDs(m)); got != 3 {
		t.Errorf("visible = %d findings, want 3", got)
	}
}

func TestPickerEnterWithNoMatchStaysOpen(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "l")
	m = typeText(t, m, "zzz")
	m, _ = press(t, m, "enter")

	if m.mode != modePicker {
		t.Errorf("mode = %v, want the picker to stay open", m.mode)
	}
	if _, ok := m.query.Sel[findings.Lens]; ok {
		t.Errorf("enter with no match set a lens: %v", m.query.Sel)
	}
}

func TestStatePickerChooseAll(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "s")
	if got := selectedTitle(m); got != "✓ open (3)" {
		t.Fatalf("cursor on %q, want the current state", got)
	}

	m, _ = press(t, m, "up", "enter")
	if _, ok := m.query.Sel[findings.State]; ok {
		t.Errorf("state still selected: %v", m.query.Sel)
	}
	if got := len(visibleIDs(m)); got != 6 {
		t.Errorf("visible = %d findings, want all 6", got)
	}
}

func TestCategoryPickerListsNone(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "c")

	want := []string{allOpenRow, "  conventions (0)", "  correctness (1)", securityRow, "  (none) (1)"}
	if got := pickerTitles(m); !slices.Equal(got, want) {
		t.Errorf("category rows = %q, want %q", got, want)
	}
}

func TestSeverityPickerRankOrder(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "v")

	want := []string{allOpenRow, "  critical (1)", "  high (1)", "  medium (0)", "  low (1)", "  advisory (0)"}
	if got := pickerTitles(m); !slices.Equal(got, want) {
		t.Errorf("severity rows = %q, want %q", got, want)
	}
}

func TestPickerKeepsVanishedSelection(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, fixtureFiles())
	m.query.Sel[findings.Lens] = "gone"
	m.refilter()
	m, _ = press(t, m, "l")

	if got := selectedTitle(m); got != "✓ gone (0)" {
		t.Errorf("cursor on %q, want the vanished selection kept with count 0", got)
	}
}

// down in the picker moves the cursor to the next value.
func TestPickerDownMovesCursor(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "l")
	start := m.picker.list.Index()
	if len(pickerTitles(m)) < start+2 {
		t.Fatalf("set-up: rows %q leave nothing below the cursor", pickerTitles(m))
	}

	m, _ = press(t, m, "down")

	if got := m.picker.list.Index(); got != start+1 {
		t.Errorf("cursor at %d after down, want %d", got, start+1)
	}
}
