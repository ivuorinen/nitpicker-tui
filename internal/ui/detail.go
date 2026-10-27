package ui

import (
	"cmp"
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

// minWrap keeps a body readable in a very narrow terminal.
const minWrap = 20

var (
	headerStyle = lipgloss.NewStyle().PaddingLeft(2)
	titleStyle  = lipgloss.NewStyle().Bold(true)
	fieldStyle  = lipgloss.NewStyle().Faint(true)
)

// wrapWidth is the body's word-wrap width in a terminal w columns wide,
// leaving room for glamour's margins.
func wrapWidth(w int) int {
	return max(w-4, minWrap)
}

// header lists the finding's fields above its body.
func header(f findings.Finding) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(cmp.Or(f.Title, "(untitled)")) + "\n\n")
	for _, kv := range [][2]string{
		{"id", f.ID}, {"severity", label(f.Severity)}, {"category", label(f.Category)},
		{"lens", f.Auditor}, {"area", f.Area}, {"location", f.Location},
		{"state", f.Status}, {"found", f.Found}, {"resolved", f.Resolved},
	} {
		if kv[1] != "" {
			fmt.Fprintf(&b, "%s %s\n", fieldStyle.Render(fmt.Sprintf("%-9s", kv[0])), kv[1])
		}
	}

	return headerStyle.Render(b.String())
}

// renderBody renders markdown in the given glamour style. Building a
// renderer costs a fraction of the render itself, so one is built per call
// rather than kept in step with the width and the terminal background. If
// building the renderer or rendering fails, the markdown is shown as it is,
// below the error; both failures share the one branch an unknown style name
// reaches. Either way the result passes through keepStyling before it can
// reach the screen.
func renderBody(body, style string, width int) string {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(wrapWidth(width)))
	var out string
	if err == nil {
		out, err = r.Render(body)
	}
	if err != nil {
		out = fmt.Sprintf("render failed: %v\n\n%s", err, body)
	}

	return keepStyling(out)
}

// keepStyling drops every escape sequence and control character except SGR
// (CSI … m with no prefix or intermediate byte), OSC 8 hyperlinks and
// newlines: the only ones glamour emits on purpose. Store text is sanitized
// at load, but goldmark decodes character references such as &#x1b; while
// rendering, and bubbletea forwards terminated DCS, APC, SOS and PM sequences
// to the terminal, so the rendered body is filtered too.
func keepStyling(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	p := ansi.NewParser()
	var state byte
	for len(s) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(s, state, p)
		state = newState
		s = s[n:]
		switch {
		case width > 0, seq == "\n":
			b.WriteString(seq)
		case ansi.HasCsiPrefix(seq) && p.Command() == 'm':
			b.WriteString(seq)
		case ansi.HasOscPrefix(seq) && p.Command() == 8:
			b.WriteString(seq)
		}
	}

	return b.String()
}

func warningsText(ws []string) string {
	if len(ws) == 0 {
		return "No load warnings.\n"
	}

	return fmt.Sprintf("%d load warnings\n\n%s\n", len(ws), strings.Join(ws, "\n"))
}
