// Package linkpicker provides the modal overlay that lets the user
// pick one item from a message: which link to open (the `o`
// keybinding) or which file attachment to download (the `d`
// keybinding). The chosen item is dispatched as ui.OpenLinkMsg or
// ui.DownloadFileMsg by the mode handler, depending on the kind the
// App recorded when opening the picker. "/" filters the rows
// (listfilter) over label, URL and detail.
package linkpicker

import (
	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/bubbles/listfilter"
)

// Item is one selectable row.
type Item struct {
	URL   string
	Label string // filename for file rows; link label (may be empty) for links
	// Detail is trailing muted info shown after the label (e.g. file
	// size). Empty for link rows.
	Detail string
	// InApp marks links that the router will navigate inside slk
	// (active-workspace archive permalinks); rendered with a badge.
	InApp bool
	// Index is the item's position in the slice passed to Open,
	// assigned by Open so the dispatcher can map the chosen row back
	// to its source data. Filtering does not change it.
	Index int
}

// Model is the picker overlay state.
type Model struct {
	title    string
	items    []Item
	shown    []int // indices into items that match the filter
	filter   listfilter.Filter
	selected int // index into shown
	visible  bool
}

var _ chrome.Dialog = (*Model)(nil)

// New creates a hidden picker.
func New() *Model { return &Model{} }

// Open shows the picker over items with the given dialog title, first
// row selected, no filter.
func (m *Model) Open(title string, items []Item) {
	m.title = title
	m.items = items
	for i := range m.items {
		m.items[i].Index = i
	}
	m.selected = 0
	m.filter.Reset()
	m.applyFilter()
	m.visible = true
}

// Close hides the picker and drops its items and filter.
func (m *Model) Close() {
	m.visible = false
	m.items = nil
	m.selected = 0
	m.filter.Reset()
	m.applyFilter()
}

// IsVisible reports whether the picker is showing.
func (m *Model) IsVisible() bool { return m.visible }

// Title returns the dialog title set by Open.
func (m *Model) Title() string { return m.title }

// Items returns every item given to Open, unfiltered.
func (m *Model) Items() []Item { return m.items }

// Selected returns the highlighted row's index among the filtered rows.
func (m *Model) Selected() int { return m.selected }

// Query returns the filter text, typed or kept.
func (m *Model) Query() string { return m.filter.Query() }

// Typing reports whether the user is typing the filter.
func (m *Model) Typing() bool { return m.filter.Typing() }

// applyFilter rebuilds shown from the filter and clamps the selection.
func (m *Model) applyFilter() {
	m.shown = m.shown[:0]
	for i, it := range m.items {
		if m.filter.Matches(it.Label, it.URL, it.Detail) {
			m.shown = append(m.shown, i)
		}
	}
	m.selected = max(min(m.selected, len(m.shown)-1), 0)
}

// HandleKey processes one key. Returns (item, true) when the user
// chose a row with enter (the picker closes itself); (Item{}, false)
// otherwise. The filter takes its keys first, so while it is typed q,
// j and k are filter text and enter keeps the filter. esc/q close
// without choosing.
func (m *Model) HandleKey(key string) (Item, bool) {
	if consumed, changed := m.filter.HandleKey(key); consumed {
		if changed {
			m.selected = 0
			m.applyFilter()
		}
		return Item{}, false
	}
	switch key {
	case "esc", "q":
		m.Close()
	case "j", "down":
		if m.selected < len(m.shown)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "enter":
		if len(m.shown) == 0 {
			return Item{}, false
		}
		item := m.items[m.shown[m.selected]]
		m.Close()
		return item, true
	}
	return Item{}, false
}
