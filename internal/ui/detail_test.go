package ui

import (
	"image/color"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	tea "charm.land/bubbletea/v2"
)

func TestEnterOpensDetailAndEscReturns(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "enter")

	if m.mode != modeDetail {
		t.Fatalf("mode = %v after enter, want detail", m.mode)
	}
	content := m.View().Content
	for _, want := range []string{"Secret in logs", idSecurityOpen, "api/log.go", "Details of"} {
		if !strings.Contains(content, want) {
			t.Errorf("detail view lacks %q", want)
		}
	}

	m, _ = press(t, m, "esc")
	if m.mode != modeList {
		t.Errorf("mode = %v after esc, want list", m.mode)
	}
}

func TestQInDetailGoesBackWithoutQuitting(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "enter")
	m, cmd := press(t, m, "q")

	if quits(cmd) {
		t.Error("q in the detail view quit the program")
	}
	if m.mode != modeList {
		t.Errorf("mode = %v after q, want list", m.mode)
	}
}

// End to end: an escape sequence in a stored body must not reach the screen,
// even though glamour passes OSC 52 through.
func TestDetailNeverRendersStoredEscapes(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"audit/open/audit-00000009.md": {Data: []byte(
		"---\nid: audit-00000009\nauditor: audit\nseverity: high\n---\n\n" +
			"# Clipboard \x1b]52;c;ZXZpbA==\x07 title\n\nBody \x1b]52;c;ZXZpbA==\x07 text.\n",
	)}}
	m, _ := press(t, newTestModel(t, fsys), "enter")

	content := m.View().Content
	if !strings.Contains(content, "Body") {
		t.Fatalf("detail not rendered: %q", content)
	}
	for _, bad := range []string{"\x1b]52", "\x07"} {
		if strings.Contains(content, bad) {
			t.Errorf("detail view contains %q", bad)
		}
	}
}

// Goldmark decodes character references after Sanitize ran, and bubbletea
// forwards terminated DCS/APC/SOS/PM sequences to the terminal, so the
// rendered body itself must carry nothing but SGR and OSC 8.
func TestDetailDropsEscapesDecodedFromEntities(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"audit/open/audit-0000000a.md": {Data: []byte(
		"---\nid: audit-0000000a\nauditor: audit\nseverity: high\n---\n\n# Entities\n\n" +
			"clip &#x1b;]52;c;ZXZpbA==&#x07; end\n\n" +
			"tmux &#x1b;Ptmux;&#x1b;&#x1b;]52;c;ZXZpbA==&#x07;&#x1b;&#x5c; end\n\n" +
			"apc &#x1b;_Gf=100;AAAA&#x1b;&#x5c; end\n\n" +
			"clear &#27;[2J end\n\n[a link](https://example.com/x)\n",
	)}}
	m, _ := press(t, newTestModel(t, fsys), "enter")
	content := m.View().Content
	if !strings.Contains(content, "clip") || !strings.Contains(content, "a link") {
		t.Fatalf("detail not rendered: %q", content)
	}

	allowed := regexp.MustCompile("\x1b\\[[0-9;:]*m|\x1b\\]8;[^\x07\x1b]*(?:\x07|\x1b\\\\)")
	if rest := allowed.ReplaceAllString(content, ""); strings.ContainsAny(rest, "\x1b\x07") {
		i := strings.IndexAny(rest, "\x1b\x07")
		t.Errorf(
			"detail view keeps a non-SGR, non-OSC-8 control sequence near %q",
			rest[max(i-10, 0):min(i+20, len(rest))],
		)
	}
}

func TestKeepStyling(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, in, want string }{
		{"sgr kept", "\x1b[31mred\x1b[m", "\x1b[31mred\x1b[m"},
		{"osc 8 kept", "\x1b]8;;https://x\x07link\x1b]8;;\x07", "\x1b]8;;https://x\x07link\x1b]8;;\x07"},
		{"newline and text kept", "a\nä 漢字", "a\nä 漢字"},
		{"osc 52 dropped", "o\x1b]52;c;ZXZpbA==\x07k", "ok"},
		{"dcs dropped", "d\x1bPtmux;\x1b\x1b]52;c;x\x07\x1b\\c", "dc"},
		{"apc dropped", "a\x1b_Gf=100;AAAA\x1b\\p", "ap"},
		{"csi other than sgr dropped", "c\x1b[2Js", "cs"},
		{"private csi ending in m dropped", "p\x1b[>4;2mm", "pm"},
		{"bel dropped", "b\x07l", "bl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := keepStyling(tt.in); got != tt.want {
				t.Errorf("keepStyling(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// glamour rejects an unknown style name; the markdown is then shown as it is,
// below the error, and still passes through keepStyling.
func TestRenderBodyFallsBackOnRenderError(t *testing.T) {
	t.Parallel()

	got := renderBody("body \x1b[2Jtext", "no-such-style", 80)
	const want = "render failed: no-such-style: style not found\n\n"
	if !strings.HasPrefix(got, want) {
		t.Errorf("renderBody = %q, want the prefix %q", got, want)
	}
	if !strings.Contains(got, "body text") || strings.Contains(got, "\x1b[2J") {
		t.Errorf("fallback body %q: want the text, without the escape", got)
	}
}

func TestBackgroundColorPicksStyle(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "enter")
	dark := m.View().Content

	m = update(t, m, tea.BackgroundColorMsg{Color: color.White})
	light := m.View().Content
	if light == dark {
		t.Error("a light background did not change the rendering")
	}

	m = update(t, m, tea.BackgroundColorMsg{Color: color.Black})
	if m.View().Content != dark {
		t.Error("a dark background did not restore the dark rendering")
	}
}

func TestDetailSurvivesTinyTerminal(t *testing.T) {
	t.Parallel()

	m, _ := press(t, newTestModel(t, fixtureFiles()), "enter")
	m = update(t, m, tea.WindowSizeMsg{Width: 10, Height: 5})

	if m.View().Content == "" {
		t.Error("detail view is empty at width 10")
	}
}

// A pty can report 0 columns, and a pane can be squeezed to almost none.
func TestTinyTerminalsDoNotPanic(t *testing.T) {
	t.Parallel()

	for _, keys := range [][]string{nil, {"s"}, {"enter"}, {"w"}, {"/"}} {
		for width := range 6 {
			for height := range 4 {
				m, _ := press(t, newTestModel(t, fixtureFiles()), keys...)
				m = update(t, m, tea.WindowSizeMsg{Width: width, Height: height})
				_ = m.View()
			}
		}
	}
}

func TestWrapWidth(t *testing.T) {
	t.Parallel()

	tests := []struct{ width, want int }{{10, 20}, {0, 20}, {24, 20}, {100, 96}}
	for _, tt := range tests {
		if got := wrapWidth(tt.width); got != tt.want {
			t.Errorf("wrapWidth(%d) = %d, want %d", tt.width, got, tt.want)
		}
	}
}

func TestWarningsView(t *testing.T) {
	t.Parallel()

	fsys := fixtureFiles()
	fsys["audit/open/NIT-119.md"] = &fstest.MapFile{Data: []byte("no frontmatter here\n")}
	m := newTestModel(t, fsys)

	if !strings.Contains(m.list.Title, "⚠ 1 warnings (w)") {
		t.Errorf("title %q does not announce the warning", m.list.Title)
	}
	m, _ = press(t, m, "w")
	if m.mode != modeDetail {
		t.Fatalf("mode = %v after w, want the warnings view", m.mode)
	}
	if content := m.View().Content; !strings.Contains(content, "audit/open/NIT-119.md: no frontmatter") {
		t.Errorf("warnings view lacks the damaged file: %q", content)
	}
}
