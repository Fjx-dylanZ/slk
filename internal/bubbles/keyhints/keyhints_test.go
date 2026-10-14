package keyhints

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/chrome"
)

// plainStyles has the real geometry (a rounded border) and no colours.
func plainStyles() Styles {
	return Styles{Chrome: chrome.Styles{Box: lipgloss.NewStyle().Border(lipgloss.RoundedBorder())}}
}

// lines is the menu drawn plain, one string per screen row.
func lines(m Model, w, h int) []string {
	v := m.View(plainStyles(), w, h)
	if v == "" {
		return nil
	}
	return strings.Split(ansi.Strip(v), "\n")
}

func keysOf(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, strings.Join(r.Keys, "/"))
	}
	return out
}

func TestRowOrder(t *testing.T) {
	var m Model
	var rows []Row
	for _, k := range []string{"Y", "ctrl+a", "?", "y", "space", "/", "A", "enter", "a", "1", "alt+y", "b"} {
		rows = append(rows, Row{Keys: []string{k}, Desc: "x"})
	}
	rows = append(rows, Row{Keys: []string{"\\"}, Desc: "leader", Group: true}, Row{Keys: []string{"w"}, Desc: "windows", Group: true})
	m.Open("t", rows)
	want := []string{
		"1", "a", "A", "b", "y", "Y", // letters and digits, lowercase first
		"/", "?", // other characters
		"enter", "space", // named keys
		"ctrl+a", "alt+y", // chords, by base key
		"w", "\\", // groups last, in the same order
	}
	if got := keysOf(m.Rows()); !slices.Equal(got, want) {
		t.Fatalf("order\n got %q\nwant %q", got, want)
	}
}

func TestGlyph(t *testing.T) {
	for k, want := range map[string]string{
		"space": "␣", "tab": "⇥", "shift+tab": "⇤", "enter": "⏎", "backspace": "⌫", "esc": "⎋",
		"ctrl+space": "ctrl+␣", "g": "g", "+": "+", "ctrl++": "ctrl++", "left": "left", "ctrl+w": "ctrl+w",
	} {
		if got := Glyph(k); got != want {
			t.Errorf("Glyph(%q) = %q, want %q", k, got, want)
		}
		if w := ansi.StringWidth(Glyph(k)); w != ansi.StringWidth(want) {
			t.Errorf("Glyph(%q) is %d cells", k, w)
		}
	}
	if got := Keys("space", "f"); got != "␣ f" {
		t.Errorf("Keys = %q", got)
	}
}

// One column: the title in the top border, a row per key with the
// arrows aligned, groups after plain rows, the footer in the bottom
// border, and the box as wide as its rows (or its title and footer).
func TestViewOneColumn(t *testing.T) {
	var m Model
	m.Open("␣", []Row{
		{Keys: []string{"w"}, Desc: "windows", Group: true},
		{Keys: []string{"t"}, Desc: "switch channel"},
		{Keys: []string{"space"}, Desc: "switch channel"},
		{Keys: []string{"h", "l"}, Desc: "focus"},
	})
	want := []string{
		"╭─ ␣ ──────────────────╮",
		"│ h/l → focus          │",
		"│ t   → switch channel │",
		"│ ␣   → switch channel │",
		"│ w   → +windows       │",
		"╰─ esc close  ⌫ back ──╯",
	}
	if got := lines(m, 80, 20); !slices.Equal(got, want) {
		t.Fatalf("menu\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func numberedRows(n int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{Keys: []string{fmt.Sprintf("k%02d", i)}, Desc: fmt.Sprintf("row %02d", i)}
	}
	return rows
}

// More rows than fit in a column spread over balanced columns, filled
// top to bottom so the order reads down each column.
func TestViewColumns(t *testing.T) {
	var m Model
	m.Open("many", numberedRows(25))
	got := lines(m, 120, 12) // 10 rows fit: 3 columns of 9
	if len(got) != 11 {
		t.Fatalf("%d lines, want 9 rows and 2 borders:\n%s", len(got), strings.Join(got, "\n"))
	}
	if !strings.Contains(got[1], "k00 → row 00") || !strings.Contains(got[1], "k09 → row 09") || !strings.Contains(got[1], "k18 → row 18") {
		t.Errorf("first row %q, want k00, k09 and k18 side by side", got[1])
	}
	if !strings.Contains(got[7], "k24") || strings.Contains(got[9], "k2") {
		t.Errorf("last column: row 7 %q, row 9 %q", got[7], got[9])
	}
	if strings.Contains(got[10], "ctrl+d") {
		t.Errorf("a menu that fits does not page: %q", got[10])
	}
}

// Rows that the columns cannot hold make pages; Page turns them (and
// only when the menu pages), the footer says which page, and a refresh
// of the same level keeps the page while a new level starts at the first.
func TestPaging(t *testing.T) {
	var m Model
	m.Open("many", numberedRows(25))
	const w, h = 40, 7 // 5 rows and 2 columns fit: 10 rows a page, 3 pages
	got := lines(m, w, h)
	if len(got) != 7 || !strings.Contains(got[6], "ctrl+d/u 1/3") || !strings.Contains(got[1], "k00") {
		t.Fatalf("first page:\n%s", strings.Join(got, "\n"))
	}
	for range 3 { // two pages on, then one more that stays on the last
		if !m.Page(1, w, h) {
			t.Fatal("Page must report that the menu pages")
		}
	}
	got = lines(m, w, h)
	if !strings.Contains(got[6], "3/3") || !strings.Contains(got[1], "k20") || strings.Contains(strings.Join(got, ""), "k19") {
		t.Fatalf("last page (Page stops there):\n%s", strings.Join(got, "\n"))
	}
	if len(got) != 7 {
		t.Errorf("the last page keeps the box's height: %d lines", len(got))
	}
	m.Open("many", numberedRows(25))
	if got := lines(m, w, h); !strings.Contains(got[6], "3/3") {
		t.Errorf("refreshing the same level reset the page: %q", got[6])
	}
	m.Open("other", numberedRows(25))
	if got := lines(m, w, h); !strings.Contains(got[6], "1/3") {
		t.Errorf("a new level kept the page: %q", got[6])
	}
	var small Model
	small.Open("few", numberedRows(3))
	if small.Page(1, w, h) {
		t.Error("a menu that fits does not take the page keys")
	}
}

// Too small for one row (in height, or for a key and a few columns of
// description): nothing is drawn. A long description is cut.
func TestViewTooSmallAndCut(t *testing.T) {
	var m Model
	m.Open("t", []Row{{Keys: []string{"x"}, Desc: "a description far longer than the forty columns a row may use"}})
	if got := m.View(plainStyles(), 80, 2); got != "" {
		t.Errorf("no row fits in 2 rows, got %q", got)
	}
	if got := m.View(plainStyles(), 12, 10); got != "" {
		t.Errorf("no description fits in 12 columns, got %q", got)
	}
	got := lines(m, 80, 10)
	if len(got) != 3 || !strings.Contains(got[1], "x → a description far longer than the forty…") {
		t.Errorf("cut row:\n%s", strings.Join(got, "\n"))
	}
	// A description shorter than the minimum still fits where it fits.
	var short Model
	short.Open("g", []Row{{Keys: []string{"g"}, Desc: "top"}})
	if got := lines(short, 80, 10); len(got) != 3 || !strings.Contains(got[1], "g → top") {
		t.Errorf("short row:\n%s", strings.Join(got, "\n"))
	}
	var closed Model
	if closed.View(plainStyles(), 80, 20) != "" {
		t.Error("a closed menu draws nothing")
	}
}
