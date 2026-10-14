// Package keyhints is a which-key menu: a floating panel listing the
// keys that may follow a pending key sequence, or the keys that act in
// a context, one "key → description" row each, drawn through
// bubbles/chrome (chrome.Panel). The model holds the rows and the page;
// the host decides when the menu is open, what it lists and where on
// screen it goes.
//
// Rows are sorted the way which-key sorts them: plain actions first,
// then groups ("+label"); within each, letters and digits (lowercase
// before uppercase), then other characters, then named keys, then
// chords. Too many rows for one column spread over several; too many
// for the columns that fit make pages, turned with Page.
package keyhints

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/chrome"
)

// Row is one row of the menu.
type Row struct {
	// Keys are the keys the row is for, spelled as tea spells them
	// ("g", "space", "ctrl+w"). They are drawn with glyphs for the
	// special keys (Glyph) and joined by "/".
	Keys []string
	// Desc is what the keys do, or a group's label.
	Desc string
	// Group marks keys that lead to more keys: drawn "+Desc" in
	// Styles.Group, after the plain rows.
	Group bool
}

// Styles is the menu's palette: the chrome it is drawn in (Panel's box,
// title and footer) and the three parts of a row.
type Styles struct {
	Chrome chrome.Styles
	Key    lipgloss.Style // the keys
	Arrow  lipgloss.Style // the " → " between keys and description
	Desc   lipgloss.Style // a plain row's description
	Group  lipgloss.Style // a group's "+label"
}

// Model is the menu.
type Model struct {
	title   string
	rows    []Row
	page    int
	visible bool
	version uint64 // see Version
}

var _ chrome.Dialog = (*Model)(nil)

// Open shows rows (sorted) under title. Reopening with the same title,
// a level that is merely refreshed, keeps the page; a new title starts
// at the first page.
func (m *Model) Open(title string, rows []Row) {
	if !m.visible || title != m.title {
		m.page = 0
	}
	m.title = title
	m.rows = sortRows(rows)
	m.visible = true
	m.version++
}

// Close hides the menu.
func (m *Model) Close() {
	*m = Model{version: m.version + 1}
}

// IsVisible reports whether the menu is open.
func (m Model) IsVisible() bool { return m.visible }

// Title is the menu's title.
func (m Model) Title() string { return m.title }

// Rows are the menu's rows, in display order.
func (m Model) Rows() []Row { return m.rows }

// Version changes whenever what View draws for a given size and Styles
// may change (Open, Page, Close), so a host can keep the drawn box until
// it does.
func (m Model) Version() uint64 { return m.version }

// Page turns the page by delta (1 next, -1 previous) when the menu,
// drawn in maxW × maxH as View draws it, has more than one page. It
// reports whether the menu pages, and so whether the page key was its.
// The page is the one View shows, so a resize that left fewer pages
// than the page turned to does not swallow the next key.
func (m *Model) Page(delta, maxW, maxH int) bool {
	l, ok := m.layout(maxW, maxH)
	if !m.visible || !ok || l.pages < 2 {
		return false
	}
	if page := min(max(min(m.page, l.pages-1)+delta, 0), l.pages-1); page != m.page {
		m.page = page
		m.version++
	}
	return true
}

// View draws the open menu to fit maxW × maxH cells, sized to its rows:
// "" when it is closed or not even one row fits.
func (m Model) View(s Styles, maxW, maxH int) string {
	if !m.visible {
		return ""
	}
	l, ok := m.layout(maxW, maxH)
	if !ok {
		return ""
	}
	perPage := l.cols * l.perCol
	start := min(m.page, l.pages-1) * perPage
	page := m.rows[start:min(start+perPage, len(m.rows))]
	lines := make([]string, l.perCol)
	for i, r := range page {
		row := i % l.perCol
		if i >= l.perCol {
			lines[row] += strings.Repeat(" ", colGap)
		}
		lines[row] += cell(s, r, l)
	}
	return chrome.Panel(s.Chrome, m.title, l.footer, strings.Join(lines, "\n"), l.width)
}

const (
	arrow   = " → "
	colGap  = 3  // columns between two columns of rows
	maxDesc = 40 // a description is cut after this many columns
	minDesc = 6  // less room than this for a cut description does not fit
	chromeW = 4  // the border and padding left and right
	chromeH = 2  // the top and bottom border
	// borderText is the border a title or footer needs besides its
	// own width: the corners, a fill cell and a space either side.
	borderText = 5
)

// layout is the menu's geometry for one size.
type layout struct {
	keyW, descW int
	cols        int // columns on a page
	perCol      int // rows in a column
	pages       int
	width       int // outer width
	footer      string
}

// layout fits the rows into maxW × maxH: one column when they fit, as
// many balanced columns as they need when those fit, else pages of the
// columns that fit. ok is false when not even one row fits.
func (m Model) layout(maxW, maxH int) (layout, bool) {
	n := len(m.rows)
	rowsFit := maxH - chromeH
	if n == 0 || rowsFit < 1 {
		return layout{}, false
	}
	var l layout
	for _, r := range m.rows {
		l.keyW = max(l.keyW, ansi.StringWidth(label(r)))
		l.descW = max(l.descW, ansi.StringWidth(desc(r)))
	}
	inner := maxW - chromeW
	arrowW := ansi.StringWidth(arrow)
	// A description gets the room left after the keys, cut at maxDesc;
	// less room than minDesc (or than the longest description, when
	// that is shorter) does not fit.
	room := inner - l.keyW - arrowW
	if room < min(l.descW, minDesc) {
		return layout{}, false
	}
	l.descW = min(l.descW, maxDesc, room)
	cellW := l.keyW + arrowW + l.descW
	colsFit := max((inner+colGap)/(cellW+colGap), 1)
	if cols := ceilDiv(n, rowsFit); cols <= colsFit {
		l.cols, l.perCol, l.pages = cols, ceilDiv(n, cols), 1
	} else {
		l.cols, l.perCol = colsFit, rowsFit
		l.pages = ceilDiv(n, colsFit*rowsFit)
	}
	l.footer = "esc close  " + Glyph("backspace") + " back"
	if l.pages > 1 {
		l.footer += fmt.Sprintf("  ctrl+d/u %d/%d", min(m.page, l.pages-1)+1, l.pages)
	}
	l.width = l.cols*cellW + (l.cols-1)*colGap + chromeW
	// The title and footer sit in the border: widen for them, up to maxW.
	text := max(ansi.StringWidth(m.title), ansi.StringWidth(l.footer))
	l.width = min(max(l.width, text+borderText+1), maxW)
	return l, true
}

// cell draws one row l.keyW + arrow + l.descW columns wide.
func cell(s Styles, r Row, l layout) string {
	d, style := desc(r), s.Desc
	if r.Group {
		style = s.Group
	}
	d = pad(ansi.Truncate(d, l.descW, "…"), l.descW)
	return s.Key.Render(pad(label(r), l.keyW)) + s.Arrow.Render(arrow) + style.Render(d)
}

func pad(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }

// label is a row's keys as drawn: glyphs, joined by "/".
func label(r Row) string {
	parts := make([]string, len(r.Keys))
	for i, k := range r.Keys {
		parts[i] = Glyph(k)
	}
	return strings.Join(parts, "/")
}

// desc is a row's description as drawn: a group's label after "+".
func desc(r Row) string {
	if r.Group {
		return "+" + r.Desc
	}
	return r.Desc
}

// glyphs are the special keys drawn as a symbol. All are narrow (one
// cell) outside any Nerd Font. Arrow keys stay words: "←" would read as
// the row's own arrow.
var glyphs = map[string]string{
	"space":     "␣",
	"tab":       "⇥",
	"shift+tab": "⇤",
	"enter":     "⏎",
	"backspace": "⌫",
	"esc":       "⎋",
	"delete":    "⌦",
}

// Glyph is key k as the menu draws it: a symbol for a special key, also
// after a modifier ("ctrl+␣"), else k.
func Glyph(k string) string {
	if g, ok := glyphs[k]; ok {
		return g
	}
	mods, base := splitMods(k)
	if g, ok := glyphs[base]; ok && mods != "" {
		return mods + g
	}
	return k
}

// Keys is a key path as a menu title draws it: each key's Glyph,
// separated by spaces ("␣ f").
func Keys(keys ...string) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = Glyph(k)
	}
	return strings.Join(parts, " ")
}

// splitMods splits k into its modifier prefix ("ctrl+alt+") and base
// key. A "+" at the very end is the key itself ("ctrl++").
func splitMods(k string) (mods, base string) {
	base = k
	for {
		i := strings.Index(base, "+")
		if i <= 0 || i == len(base)-1 {
			return k[:len(k)-len(base)], base
		}
		switch base[:i] {
		case "ctrl", "alt", "shift", "super", "meta", "hyper":
			base = base[i+1:]
		default:
			return k[:len(k)-len(base)], base
		}
	}
}

// sortRows returns rows in display order (see the package doc).
func sortRows(rows []Row) []Row {
	out := slices.Clone(rows)
	slices.SortStableFunc(out, func(a, b Row) int {
		if a.Group != b.Group {
			if a.Group {
				return 1
			}
			return -1
		}
		return compareKeys(first(a), first(b))
	})
	return out
}

func first(r Row) string {
	if len(r.Keys) == 0 {
		return ""
	}
	return r.Keys[0]
}

// compareKeys orders two keys: by kind (keyKind), then case-insensitively
// by base key with lowercase first (a A b B), then by spelling.
func compareKeys(a, b string) int {
	ka, ba := keyKind(a)
	kb, bb := keyKind(b)
	if ka != kb {
		return ka - kb
	}
	if c := strings.Compare(strings.ToLower(ba), strings.ToLower(bb)); c != 0 {
		return c
	}
	if c := strings.Compare(bb, ba); c != 0 { // lowercase sorts after uppercase in ASCII
		return c
	}
	return strings.Compare(a, b)
}

// keyKind ranks a key: 0 a single letter or digit, 1 any other single
// character, 2 a named key (space, enter, up, pgdown, …), 3 a chord with
// a modifier. base is the key without its modifiers.
func keyKind(k string) (kind int, base string) {
	mods, base := splitMods(k)
	if mods != "" {
		return 3, base
	}
	r := []rune(k)
	switch {
	case len(r) == 1 && (unicode.IsLetter(r[0]) || unicode.IsDigit(r[0])):
		return 0, k
	case len(r) == 1:
		return 1, k
	}
	return 2, k
}
