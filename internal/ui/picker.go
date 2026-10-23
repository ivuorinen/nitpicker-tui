package ui

import (
	"fmt"
	"slices"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

// pickerItem is one picker row: "all", or one value of the facet, with the
// number of findings choosing it would show.
type pickerItem struct {
	value   string
	all     bool
	count   int
	current bool
}

// Title marks the current value with a check mark, as text, not only by
// highlight.
func (p pickerItem) Title() string {
	mark := "  "
	if p.current {
		mark = "✓ "
	}

	return fmt.Sprintf("%s%s (%d)", mark, p.name(), p.count)
}

func (p pickerItem) Description() string { return "" }

// FilterValue is what typing in the picker matches, fuzzily.
func (p pickerItem) FilterValue() string { return p.name() }

func (p pickerItem) name() string {
	if p.all {
		return "all"
	}

	return label(p.value)
}

// picker chooses the value of one facet. gen tells its messages apart from a
// picker opened before it.
type picker struct {
	facet findings.Facet
	list  list.Model
	gen   int
}

// pickerMsg is a message produced by the commands of picker generation gen.
// The list's FilterMatchesMsg carries no identity of its own, so without the
// tag a result computed for a closed picker would fill the one open now.
type pickerMsg struct {
	gen int
	msg tea.Msg
}

// tagPicker wraps cmd so every message it produces, batches included,
// arrives as a pickerMsg of generation gen.
func tagPicker(gen int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}

	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			tagged := make(tea.BatchMsg, len(batch))
			for i, c := range batch {
				tagged[i] = tagPicker(gen, c)
			}

			return tagged
		}

		return pickerMsg{gen: gen, msg: msg}
	}
}

// newPicker builds the picker for facet f and returns it with the index of
// the current value. A selected value the findings no longer contain stays
// listed, with count 0, so the selection is never invisible.
func newPicker(all []findings.Finding, q findings.Query, f findings.Facet, width, height int) (picker, int) {
	values := findings.Values(all, f)
	cur, selected := q.Sel[f]
	if selected && !slices.Contains(values, cur) {
		values = append(values, cur)
	}
	total, per := findings.Counts(all, q, f)

	items := make([]list.Item, 0, len(values)+1)
	items = append(items, pickerItem{all: true, count: total, current: !selected})
	at := 0
	for i, v := range values {
		isCurrent := selected && v == cur
		if isCurrent {
			at = i + 1
		}
		items = append(items, pickerItem{value: v, count: per[v], current: isCurrent})
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)
	l := list.New(items, d, width, height)
	// The picker stays in filtering mode, where the list draws its filter
	// prompt instead of its title: the prompt names the facet.
	l.Title = f.String()
	l.FilterInput.Prompt = f.String() + ": "
	l.DisableQuitKeybindings()
	// The list ends typing mode on tab, shift+tab, ctrl+k and ctrl+j too; the
	// model handles enter, esc and the arrows itself, so the list keeps none.
	l.KeyMap.AcceptWhileFiltering.SetKeys()

	return picker{facet: f, list: l}, at
}
