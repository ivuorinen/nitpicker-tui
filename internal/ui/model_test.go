package ui

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

const (
	ctrlC          = "ctrl+c"
	idAuditOpen    = "audit-00000001"
	idSecurityOpen = "security-00000002"
)

// fixtureFiles is a store of 3 open findings (3 lenses), 2 fixed and 1
// invalid (a fourth lens, ci, has only the invalid one), built through
// findings.Load like a real store.
func fixtureFiles() fstest.MapFS {
	open := func(id, auditor, severity, category, area, title string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("---\nid: " + id + "\nauditor: " + auditor +
			"\nseverity: " + severity + "\ncategory: " + category + "\narea: " + area +
			"\nstatus: open\nfound: 2026-10-01\n---\n\n# " + title + "\n\n## Problem\nDetails of " + id + ".\n")}
	}

	return fstest.MapFS{
		"audit/open/audit-00000001.md": open(
			idAuditOpen,
			"audit",
			"high",
			"correctness",
			"auth/token.go",
			"Token refresh race",
		),
		"security/open/security-00000002.md": open(
			idSecurityOpen,
			"security",
			"critical",
			"security",
			"api/log.go",
			"Secret in logs",
		),
		"docs/open/docs-00000003.md": open("docs-00000003", "docs", "low", "", "README.md", "Stale README"),
		findings.LedgerName: {Data: []byte(
			`{"id":"audit-00000004","auditor":"audit","severity":"medium","category":"correctness",` +
				`"status":"fixed","found":"2026-09-01","resolved":"2026-09-02","title":"Old tokenizer bug","body":"b\n"}` + "\n" +
				`{"id":"security-00000005","auditor":"security","severity":"high","category":"security",` +
				`"status":"fixed","found":"2026-09-03","resolved":"2026-09-04","title":"Weak hash","body":"b\n"}` + "\n" +
				`{"id":"ci-00000006","auditor":"ci","severity":"advisory","category":"conventions",` +
				`"status":"invalid","found":"2026-09-05","resolved":"2026-09-06","title":"Unpinned","body":"b\n"}` + "\n",
		)},
	}
}

func loadStore(t *testing.T, fsys fstest.MapFS) findings.Store {
	t.Helper()

	s, err := findings.Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	return s
}

// newTestModel builds a model over the fixture store at a fixed size.
func newTestModel(t *testing.T, fsys fstest.MapFS) Model {
	t.Helper()

	return newReloadingModel(t, fsys, nil)
}

// newReloadingModel is newTestModel with a reload function for rescans.
func newReloadingModel(t *testing.T, fsys fstest.MapFS, reload Reload) Model {
	t.Helper()

	m := New(loadStore(t, fsys), reload)

	return update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	m, _ = step(t, m, msg)

	return m
}

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
	}

	return nm, cmd
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case ctrlC:
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]

	return tea.KeyPressMsg{Code: r, Text: k}
}

// press sends each key in turn, running the commands they return, and
// returns the command of the last key.
func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()

	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = step(t, m, keyMsg(k))
		m = settle(t, m, cmd)
	}

	return m, cmd
}

// typeText presses each rune of s.
func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()

	for _, r := range s {
		m, _ = press(t, m, string(r))
	}

	return m
}

// run executes cmd and returns the messages it produced, expanding batches.
// A command still running after a short wait (a cursor-blink timer) is
// dropped: no message this package handles comes from a timer.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range batch {
				out = append(out, run(c)...)
			}

			return out
		}

		return []tea.Msg{msg}
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

// settle feeds back every message cmd produced, except quitting, until the
// model stops issuing commands.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	for range 10 {
		msgs := run(cmd)
		var cmds []tea.Cmd
		for _, msg := range msgs {
			if _, quit := msg.(tea.QuitMsg); quit {
				continue
			}
			var c tea.Cmd
			m, c = step(t, m, msg)
			cmds = append(cmds, c)
		}
		if len(cmds) == 0 {
			return m
		}
		cmd = tea.Batch(cmds...)
	}

	return m
}

func quits(cmd tea.Cmd) bool {
	for _, msg := range run(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}

	return false
}

func visibleIDs(m Model) []string {
	var out []string
	for _, it := range m.list.Items() {
		if fi, ok := it.(item); ok {
			out = append(out, fi.f.ID)
		}
	}

	return out
}

func TestListStartsOnOpenFindings(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, fixtureFiles())

	got := visibleIDs(m)
	want := []string{idSecurityOpen, idAuditOpen, "docs-00000003"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("visible = %q, want %q (open only, severity order)", got, want)
	}
	for _, part := range []string{"state: open", "3/6"} {
		if !strings.Contains(m.list.Title, part) {
			t.Errorf("title %q lacks %q", m.list.Title, part)
		}
	}
}

func TestSearchNarrowsAndEscClears(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, fixtureFiles())
	m, _ = press(t, m, "/")
	m = typeText(t, m, "tok")

	if got := visibleIDs(m); len(got) != 1 || got[0] != idAuditOpen {
		t.Fatalf("after typing tok, visible = %q, want only audit-00000001", got)
	}

	m, _ = press(t, m, "esc")
	if got := visibleIDs(m); len(got) != 3 {
		t.Errorf("after esc, visible = %q, want all 3 open findings", got)
	}
	if m.search.Focused() || m.search.Value() != "" {
		t.Errorf("after esc, search focused=%v value=%q, want blurred and empty", m.search.Focused(), m.search.Value())
	}
}

func TestEnterKeepsQuery(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, fixtureFiles())
	m, _ = press(t, m, "/")
	m = typeText(t, m, "secret")
	m, _ = press(t, m, "enter")

	if m.search.Focused() {
		t.Error("search still focused after enter")
	}
	if got := visibleIDs(m); len(got) != 1 || got[0] != idSecurityOpen {
		t.Errorf("after enter, visible = %q, want only security-00000002", got)
	}
}

func TestQuitKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		keys  []string
		quits bool
	}{
		{"q in the list quits", []string{"q"}, true},
		{"ctrl+c quits", []string{ctrlC}, true},
		{"q typed into search does not quit", []string{"/", "q"}, false},
		// bubbles v2.2.1 binds the list's own Quit to "v" (list/keys.go:91-94).
		{"v does not quit", []string{"v"}, false},
		{"ctrl+c quits from search", []string{"/", ctrlC}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := newTestModel(t, fixtureFiles())
			m, cmd := press(t, m, tt.keys...)
			if got := quits(cmd); got != tt.quits {
				t.Errorf("quits = %v, want %v", got, tt.quits)
			}
			if tt.keys[0] == "/" && !tt.quits && m.search.Value() != "q" {
				t.Errorf("search value = %q, want %q", m.search.Value(), "q")
			}
		})
	}
}

func TestViewShowsTitleAndRows(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, fixtureFiles())
	v := m.View()

	if !v.AltScreen {
		t.Error("view does not use the alternate screen")
	}
	for _, want := range []string{"Secret in logs", "CRITICAL", idSecurityOpen, "state: open"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("view lacks %q", want)
		}
	}
}
