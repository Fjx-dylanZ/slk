// internal/ui/marks/marks.go
//
// The marks overlay widget: a scrollable list of the active
// workspace's marks, one row per mark. It follows the repo's overlay
// convention — HandleKey(keyStr string) *MarksResult (nil = stay
// open), ViewOverlay(termWidth, termHeight, background) string — and
// draws its box, list window, marker and scrollbar through
// internal/bubbles/chrome, like the channel finder.
//
// Every row renders exclusively from the preview snapshot stored at
// mark time: the widget never fetches, never resolves a channel or
// message live. That is the whole reason the snapshot exists — the
// list must draw correctly offline and immediately after a restart.
//
// A mark letter jumps immediately (the overlay must not add a
// keystroke to the 'a flow); Enter selects the highlighted row; and
// backspace/Delete removes the highlighted row (the same removal the
// :delmarks command uses). "/" filters the rows (listfilter) over the
// letter, channel, author and excerpt. While the filter is typed it
// takes letters, backspace, Enter (keeps the filter), Esc (clears it)
// and Delete (ignored, so it cannot remove a mark mid-query); once the
// filter is kept, Enter and backspace/Delete act on the highlighted
// filtered row again.
package marks

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/bubbles/listfilter"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

// Row is one mark as the overlay renders it: the letter, the channel
// name, and the message preview (author + excerpt). All fields come
// from the snapshot recorded at mark time; the jump resolves live.
type Row struct {
	Letter      string
	ChannelName string
	AuthorName  string
	Excerpt     string
}

// Preview is the row's message preview as the list draws it: the
// author, then the excerpt ("alice: lunch?"); the excerpt alone without
// an author. The key-hints menu's mark rows read the same.
func (r Row) Preview() string {
	if r.AuthorName != "" {
		return r.AuthorName + ": " + r.Excerpt
	}
	return r.Excerpt
}

// MarksResult is returned by HandleKey when the user acts on a row.
// Exactly one of Select / Delete is non-empty; nil means the overlay
// stays open and nothing happened.
type MarksResult struct {
	// Select names the mark to jump to through the same path as the
	// ' chord (applyLocation), so the overlay and the 'a jump cannot diverge.
	Select string
	// Delete names the mark to remove — the same removal the
	// :delmarks command will use, so the two surfaces stay in sync.
	Delete string
}

// Model is the marks overlay widget.
type Model struct {
	rows     []Row
	shown    []int // indices into rows that match the filter
	filter   listfilter.Filter
	selected int // index into shown
	visible  bool
}

var _ interface {
	chrome.Dialog
	chrome.Clickable
} = (*Model)(nil)

// New creates the widget in its closed state.
func New() Model {
	return Model{}
}

// SetRows replaces the mark list (e.g. after a deletion), re-applies
// the filter to it and clamps the selection into the filtered rows.
func (m *Model) SetRows(rows []Row) {
	m.rows = rows
	m.applyFilter()
}

// applyFilter rebuilds shown from the filter and clamps the selection.
func (m *Model) applyFilter() {
	m.shown = m.shown[:0]
	for i, r := range m.rows {
		if m.filter.Matches(r.Letter, r.ChannelName, r.AuthorName, r.Excerpt) {
			m.shown = append(m.shown, i)
		}
	}
	m.selected = max(min(m.selected, len(m.shown)-1), 0)
}

// Open shows the overlay with the first row selected and no filter.
func (m *Model) Open() {
	m.visible = true
	m.selected = 0
	m.filter.Reset()
	m.applyFilter()
}

// Close hides the overlay and drops its filter.
func (m *Model) Close() {
	m.visible = false
	m.filter.Reset()
	m.applyFilter()
}

// IsVisible reports whether the overlay is showing.
func (m Model) IsVisible() bool {
	return m.visible
}

// Typing reports whether the user is typing the filter, so every
// printable key, ' and ` included, is filter text rather than a jump.
func (m Model) Typing() bool {
	return m.filter.Typing()
}

// HandleKey processes a key and returns a result when the user acts
// on a row, nil otherwise. The filter takes its keys first. A mark
// letter (a-z, A-Z) jumps immediately when the filter is not being
// typed, so 'a never takes an extra keystroke. Enter selects and
// backspace/Delete removes the highlighted filtered row.
func (m *Model) HandleKey(keyStr string) *MarksResult {
	if consumed, changed := m.filter.HandleKey(keyStr); consumed {
		if changed {
			m.selected = 0
			m.applyFilter()
		}
		return nil
	}
	if len(keyStr) == 1 && isMarkLetter(keyStr[0]) {
		return &MarksResult{Select: keyStr}
	}

	switch keyStr {
	case "enter":
		if len(m.shown) == 0 {
			return nil
		}
		return &MarksResult{Select: m.rows[m.shown[m.selected]].Letter}

	case "esc":
		m.Close()
		return nil

	case "down", "ctrl+n":
		if m.selected < len(m.shown)-1 {
			m.selected++
		}
		return nil

	case "up", "ctrl+p":
		if m.selected > 0 {
			m.selected--
		}
		return nil

	case "backspace", "delete":
		if len(m.shown) == 0 {
			return nil
		}
		return &MarksResult{Delete: m.rows[m.shown[m.selected]].Letter}
	}
	return nil
}

func isMarkLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// maxVisibleRows is the height of the scroll window for the list.
const maxVisibleRows = 10

// boxWidth returns the modal's outer width for a given terminal
// width, matching the channel finder's sizing.
func boxWidth(termWidth int) int {
	w := termWidth / 2
	if w < 30 {
		w = 30
	}
	if w > 80 {
		w = 80
	}
	return w
}

// boxChromeRows is the box's height without its list and filter line:
// the border and padding above, the title and the blank line under it,
// then the padding and border below.
const boxChromeRows = 6

// chromeRows is the box's height without its list: boxChromeRows plus
// the filter line while the filter is active.
func (m *Model) chromeRows() int {
	if m.filter.Active() {
		return boxChromeRows + 1
	}
	return boxChromeRows
}

// listRows is how many list rows the window shows in a terminal
// termHeight rows tall: maxVisibleRows, fewer when the box would not
// fit, and at least one.
func (m *Model) listRows(termHeight int) int {
	return max(min(maxVisibleRows, termHeight-m.chromeRows()), 1)
}

// visibleWindow returns the [start, end) slice of shown currently
// drawn in the list.
func (m *Model) visibleWindow(termHeight int) (int, int) {
	return chrome.Window(m.selected, len(m.shown), m.listRows(termHeight))
}

// listTopOffset is the box-local row of the first list row.
func (m *Model) listTopOffset() int {
	return chrome.BodyTop(messages.ChromeStyles(), m.filter.Active())
}

// BoxSize returns the outer size of the rendered box.
func (m *Model) BoxSize(termWidth, termHeight int) (int, int) {
	start, end := m.visibleWindow(termHeight)
	nRows := max(end-start, 1) // an empty list still draws its Empty line
	return boxWidth(termWidth), nRows + m.chromeRows()
}

// ClickRow moves the selection to the list row at box-local localY
// (0 is the top border) and reports whether a row was there. A click
// on a row ends typing the filter (keeping it), so the Enter the App
// synthesises after it selects the row instead of committing the
// query.
func (m *Model) ClickRow(termWidth, termHeight, localY int) bool {
	row := localY - m.listTopOffset()
	if row < 0 {
		return false
	}
	start, end := m.visibleWindow(termHeight)
	if row >= end-start {
		return false
	}
	m.selected = start + row
	if m.filter.Typing() {
		m.filter.HandleKey("enter")
	}
	return true
}

// View renders just the overlay box (no backdrop). Used by tests.
func (m Model) View(termWidth, termHeight int) string {
	return m.renderBox(termWidth, termHeight)
}

// ViewOverlay renders the overlay as a centered modal with a dimmed
// backdrop, following the repo's overlay convention.
func (m Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.visible {
		return background
	}
	box := m.renderBox(termWidth, termHeight)
	if box == "" {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}

func (m Model) renderBox(termWidth, termHeight int) string {
	if !m.visible {
		return ""
	}
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4 // border + padding

	start, end := m.visibleWindow(termHeight)
	list := chrome.List{Start: start, End: end, Total: len(m.shown)}
	contentWidth := list.ContentWidth(innerWidth)

	letterStyle := s.Row.Foreground(styles.Accent).Bold(true)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		row := m.rows[m.shown[i]]
		letter, content := letterStyle, s.Row
		if i == m.selected {
			letter, content = letterStyle.Foreground(styles.Primary), s.RowSelected
		}

		preview := row.Preview()
		line := letter.Render(row.Letter) + "  " +
			content.Render("#"+row.ChannelName) + "  " +
			content.Render(preview)

		// Truncate on measured width (lipgloss.Width), not rune count:
		// excerpts are arbitrary user text and can carry wide glyphs
		// (emoji). truncate.StringWithTail is ANSI-aware.
		if lipgloss.Width(line) > contentWidth {
			line = truncate.StringWithTail(line, uint(contentWidth), "…")
		}
		// Right-pad with spaces to fill the row.
		if pad := contentWidth - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		rows = append(rows, line)
	}

	empty := "No marks set"
	if len(m.rows) > 0 {
		empty = "No matching marks"
	}
	return chrome.Frame{
		Title: "Marks",
		Input: m.filter.Input(s, "Type to filter...", ""),
		Body:  list.Rows(s, m.selected, rows),
		Empty: empty,
	}.Render(s, overlayWidth)
}
