// Package ui is the terminal interface: a list of findings with search, facet
// filters, a detail view and rescans. It never touches the filesystem; main
// hands it a loaded store.
package ui

import (
	"cmp"
	"fmt"
	"maps"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

// noneLabel stands for an empty facet value.
const noneLabel = "(none)"

func label(v string) string {
	return cmp.Or(v, noneLabel)
}

// item is one finding in the list.
type item struct{ f *findings.Finding }

func (i item) Title() string {
	return fmt.Sprintf("%-8s  %s", strings.ToUpper(label(i.f.Severity)), cmp.Or(i.f.Title, "(untitled)"))
}

func (i item) Description() string {
	return strings.Join([]string{i.f.ID, i.f.Auditor, i.f.Status, label(i.f.Category), i.f.Area}, " · ")
}

// FilterValue is unused: the list's own filtering is off, search is ours.
func (i item) FilterValue() string { return i.f.Title }

// mode is the view on screen; only one shows at a time.
type mode int

const (
	modeList mode = iota
	modePicker
	modeDetail
)

// Model is the bubbletea model for the browser.
type Model struct {
	all      []findings.Finding
	warnings []string
	query    findings.Query
	shown    int

	list   list.Model
	search textinput.Model
	picker picker
	mode   mode
	keys   keyMap
	width  int
	height int

	// The detail view shows detail, or the load warnings when it is nil,
	// rendered in the glamour style matching the terminal background.
	viewport viewport.Model
	detail   *findings.Finding
	style    string

	// pickerGen numbers each picker opened, so messages from a closed one
	// can be told apart (pickerMsg).
	pickerGen int

	reload     Reload
	rescanning bool
	// status is the last rescan's outcome, shown in the title bar.
	status string
}

// New returns a model showing store, filtered to open findings; reload is
// called for each rescan (nil disables rescanning).
func New(store findings.Store, reload Reload) Model {
	keys := newKeyMap()

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.SetFilteringEnabled(false)
	// The list's Quit is bound to "v" in bubbles v2.2.1; quitting is ours.
	l.DisableQuitKeybindings()
	// h and l page in bubbles; l opens the lens picker here.
	l.KeyMap.NextPage.SetKeys("right", "pgdown", "f", "d")
	l.KeyMap.PrevPage.SetKeys("left", "pgup", "b", "u")
	l.AdditionalShortHelpKeys = keys.listHelp
	l.AdditionalFullHelpKeys = keys.listHelp
	l.SetStatusBarItemName("finding", "findings")

	search := textinput.New()
	search.Prompt = "/ "
	search.Placeholder = "press / to search"

	vp := viewport.New()
	vp.SoftWrap = true

	m := Model{
		list:     l,
		search:   search,
		keys:     keys,
		query:    findings.Query{Sel: map[findings.Facet]string{findings.State: findings.StatusOpen}},
		viewport: vp,
		style:    styles.DarkStyle,
		reload:   reload,
	}
	m.setStore(store)

	return m
}

// Run shows store until the user quits, calling reload for each rescan.
func Run(store findings.Store, reload Reload) error {
	_, err := tea.NewProgram(New(store, reload)).Run()

	return err
}

// Init asks the terminal for its background color, which picks the glamour
// style; the dark one is used until it answers.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)

		return m, nil
	case tea.BackgroundColorMsg:
		m.setStyle(msg.IsDark())

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case rescanMsg:
		return m.applyRescan(msg)
	case pickerMsg:
		return m.updatePickerMsg(msg)
	}

	return m.forward(msg)
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var content string
	switch m.mode {
	case modePicker:
		content = m.picker.list.View()
	case modeDetail:
		content = m.viewport.View()
	case modeList:
		content = lipgloss.JoinVertical(lipgloss.Left, m.search.View(), m.list.View())
	}
	v := tea.NewView(content)
	v.AltScreen = true

	return v
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.list.SetSize(width, height-1)
	// A picker is built at the current size; only an open one resizes.
	if m.mode == modePicker {
		m.picker.list.SetSize(width, height)
	}
	// A negative width makes the textinput panic while it draws; a pty can
	// report 0 columns.
	m.search.SetWidth(max(width-4, 0))
	m.list.Title = m.title()
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(height)
	if m.mode == modeDetail {
		m.renderDetail()
	}
}

func (m *Model) setStyle(dark bool) {
	m.style = styles.DarkStyle
	if !dark {
		m.style = styles.LightStyle
	}
	if m.mode == modeDetail {
		m.renderDetail()
	}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.ForceQuit) {
		return m, tea.Quit
	}
	switch m.mode {
	case modePicker:
		return m.updatePicker(msg)
	case modeDetail:
		return m.updateDetail(msg)
	case modeList:
	}

	return m.updateList(msg)
}

// forward hands any other message (filter results, cursor blinks, status
// timers) to the components on screen.
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modePicker:
		var cmd tea.Cmd
		m.picker.list, cmd = m.picker.list.Update(msg)
		m.clampPicker()

		return m, tagPicker(m.picker.gen, cmd)
	case modeDetail:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)

		return m, cmd
	case modeList:
	}
	var searchCmd, listCmd tea.Cmd
	m.search, searchCmd = m.search.Update(msg)
	m.list, listCmd = m.list.Update(msg)

	return m, tea.Batch(searchCmd, listCmd)
}

// setStore replaces the findings and warnings and re-applies the query.
func (m *Model) setStore(s findings.Store) {
	m.all = s.Findings
	findings.Sort(m.all)
	m.warnings = s.Warnings
	m.refilter()
}

// refilter rebuilds the list from the query and moves to its top.
func (m *Model) refilter() {
	idx := findings.Filter(m.all, m.query)
	items := make([]list.Item, len(idx))
	for i, j := range idx {
		items[i] = item{f: &m.all[j]}
	}
	m.list.SetItems(items)
	m.list.ResetSelected()
	m.shown = len(idx)
	m.list.Title = m.title()
}

// title names every active filter, the counts and any warnings, cut to the
// terminal width.
func (m Model) title() string {
	var parts []string
	for _, f := range findings.Facets {
		if v, ok := m.query.Sel[f]; ok {
			parts = append(parts, f.String()+": "+label(v))
		}
	}
	parts = append(parts, fmt.Sprintf("%d/%d", m.shown, len(m.all)))
	if n := len(m.warnings); n > 0 {
		parts = append(parts, fmt.Sprintf("⚠ %d warnings (w)", n))
	}
	if m.status != "" {
		parts = append(parts, m.status)
	}
	t := strings.Join(parts, " · ")
	if m.width > 4 {
		t = lipgloss.NewStyle().MaxWidth(m.width - 4).Render(t)
	}

	return t
}

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.search.Focused() {
		return m.updateSearch(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Search):
		return m, m.search.Focus()
	case key.Matches(msg, m.keys.Open):
		if it, ok := m.list.SelectedItem().(item); ok {
			m.openDetail(it.f)
		}

		return m, nil
	case key.Matches(msg, m.keys.Warnings):
		m.openDetail(nil)

		return m, nil
	case key.Matches(msg, m.keys.Rescan):
		return m.startRescan()
	}
	for f, b := range m.keys.facets() {
		if key.Matches(msg, b) {
			return m.openPicker(f)
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

// updateSearch sends every key to the search field, except enter (keep the
// query) and esc (clear it).
func (m Model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Accept):
		m.search.Blur()

		return m, nil
	case key.Matches(msg, m.keys.Cancel):
		m.search.Reset()
		m.search.Blur()
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	if v := m.search.Value(); v != m.query.Text {
		m.query.Text = v
		m.refilter()
	}

	return m, cmd
}

// openPicker shows the picker for facet f, already filtering, with the cursor
// on the current value. The list fills its filtered set only through its own
// "/" handler, so the picker opens by pressing it.
func (m Model) openPicker(f findings.Facet) (Model, tea.Cmd) {
	p, at := newPicker(m.all, m.query, f, m.width, m.height)
	m.pickerGen++
	p.gen = m.pickerGen
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	p.list.Select(at)
	m.picker, m.mode = p, modePicker

	return m, tagPicker(p.gen, cmd)
}

// reopenPicker rebuilds the picker for facet f with text already typed, as
// after a rescan. SetFilterText filters at once but leaves typing mode, so
// typing mode is restored after it.
func (m Model) reopenPicker(f findings.Facet, text string) (Model, tea.Cmd) {
	m, cmd := m.openPicker(f)
	if text != "" {
		m.picker.list.SetFilterText(text)
		m.picker.list.SetFilterState(list.Filtering)
		m.clampPicker()
	}

	return m, cmd
}

// updatePickerMsg hands a tagged message to the picker it was made for, and
// drops one whose picker has been closed or replaced.
func (m Model) updatePickerMsg(msg pickerMsg) (tea.Model, tea.Cmd) {
	if m.mode != modePicker || msg.gen != m.picker.gen {
		return m, nil
	}
	var cmd tea.Cmd
	m.picker.list, cmd = m.picker.list.Update(msg.msg)
	m.clampPicker()

	return m, tagPicker(m.picker.gen, cmd)
}

// updatePicker handles enter, esc and the arrows itself, so the picker stays
// in typing mode throughout; every other key narrows the values.
func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Accept):
		it, ok := m.picker.list.SelectedItem().(pickerItem)
		if !ok {
			// Nothing matches what was typed.
			return m, nil
		}
		sel := maps.Clone(m.query.Sel)
		if it.all {
			delete(sel, m.picker.facet)
		} else {
			sel[m.picker.facet] = it.value
		}
		m.query.Sel = sel
		m.mode = modeList
		m.refilter()

		return m, nil
	case key.Matches(msg, m.keys.Cancel):
		m.mode = modeList

		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.picker.list.CursorUp()

		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.picker.list.CursorDown()

		return m, nil
	}
	var cmd tea.Cmd
	m.picker.list, cmd = m.picker.list.Update(msg)
	m.clampPicker()

	return m, tagPicker(m.picker.gen, cmd)
}

// clampPicker keeps the cursor on a visible row. The list replaces its
// filtered rows without moving the cursor, so a cursor openPicker placed on
// the current value is left past the last row once typing narrows the rows,
// and enter then finds nothing to apply.
func (m *Model) clampPicker() {
	if m.picker.list.SelectedItem() == nil && len(m.picker.list.VisibleItems()) > 0 {
		m.picker.list.Select(0)
	}
}

// openDetail shows f in the detail view, or the load warnings when f is nil.
func (m *Model) openDetail(f *findings.Finding) {
	m.detail = f
	m.mode = modeDetail
	m.viewport.SetYOffset(0)
	m.renderDetail()
}

// renderDetail fills the detail view for the current width and style,
// keeping the scroll position (the viewport clamps it).
func (m *Model) renderDetail() {
	offset := m.viewport.YOffset()
	if m.detail == nil {
		m.viewport.SetContent(warningsText(m.warnings))
	} else {
		m.viewport.SetContent(header(*m.detail) + renderBody(m.detail.Body, m.style, m.width))
	}
	m.viewport.SetYOffset(offset)
}

func (m Model) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Back) {
		m.mode = modeList

		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	return m, cmd
}
