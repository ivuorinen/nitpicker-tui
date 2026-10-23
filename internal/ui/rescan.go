package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

// Reload loads the findings store again. main supplies it, so the model
// never touches the filesystem.
type Reload func() (findings.Store, error)

// rescanMsg carries a finished rescan back to Update.
type rescanMsg struct {
	store findings.Store
	err   error
}

// rescan runs reload off the UI goroutine. It shares nothing with the model:
// the result reaches it only as a rescanMsg, inside Update.
func rescan(reload Reload) tea.Cmd {
	return func() tea.Msg {
		s, err := reload()

		return rescanMsg{store: s, err: err}
	}
}

// startRescan begins a rescan unless one is running already, so results
// never overlap or arrive out of order.
func (m Model) startRescan() (tea.Model, tea.Cmd) {
	if m.rescanning || m.reload == nil {
		return m, nil
	}
	m.rescanning = true
	m.status = "rescanning…"
	m.list.Title = m.title()

	return m, rescan(m.reload)
}

// applyRescan swaps in a rescanned store, keeping the query and the finding
// under the cursor. A failed rescan keeps the old data on screen.
func (m Model) applyRescan(msg rescanMsg) (tea.Model, tea.Cmd) {
	m.rescanning = false
	if msg.err != nil {
		m.status = "rescan failed: " + findings.Sanitize(msg.err.Error())
		m.list.Title = m.title()

		return m, nil
	}

	var id, status string
	if it, ok := m.list.SelectedItem().(item); ok {
		id, status = it.f.ID, it.f.Status
	}
	index := m.list.Index()
	m.status = fmt.Sprintf("rescanned: %d findings", len(msg.store.Findings))
	m.setStore(msg.store)
	m.reselect(id, status, index)
	switch m.mode {
	case modePicker:
		return m.reopenPicker(m.picker.facet, m.picker.list.FilterInput.Value())
	case modeDetail:
		m.refreshDetail()
	case modeList:
	}

	return m, nil
}

// refreshDetail points an open detail view at the same finding (id and
// status) in the new store and renders it again; the warnings view is
// rendered from the new warnings. A finding the rescan no longer holds stays
// on screen as it was read.
func (m *Model) refreshDetail() {
	if m.detail != nil {
		for i := range m.all {
			if m.all[i].ID == m.detail.ID && m.all[i].Status == m.detail.Status {
				m.detail = &m.all[i]

				break
			}
		}
	}
	m.renderDetail()
}

// reselect moves the cursor back to the finding it was on, or, when that
// finding is gone, to the nearest row at its old position.
func (m *Model) reselect(id, status string, index int) {
	items := m.list.Items()
	for i, it := range items {
		if fi, ok := it.(item); ok && fi.f.ID == id && fi.f.Status == status {
			m.list.Select(i)

			return
		}
	}
	if len(items) > 0 {
		m.list.Select(min(index, len(items)-1))
	}
}
