package marks

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func clickRows() []Row {
	return []Row{
		{Letter: "a", ChannelName: "general", Excerpt: "one"},
		{Letter: "b", ChannelName: "random", Excerpt: "two"},
		{Letter: "c", ChannelName: "dev", Excerpt: "three"},
	}
}

// TestBoxSizeMatchesRender locks BoxSize to the rendered box, so the
// geometry the click router uses cannot drift from what renderBox draws.
func TestBoxSizeMatchesRender(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []Row
	}{
		{"rows", clickRows()},
		{"empty", nil},
		{"scrolling", make([]Row, maxVisibleRows+4)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New()
			m.SetRows(tc.rows)
			m.Open()
			w, h := m.BoxSize(80, 24)
			box := m.renderBox(80, 24)
			if gw := lipgloss.Width(box); w != gw {
				t.Errorf("BoxSize width = %d, rendered width = %d", w, gw)
			}
			if gh := lipgloss.Height(box); h != gh {
				t.Errorf("BoxSize height = %d, rendered height = %d", h, gh)
			}
		})
	}
}

// TestClickRowSelectsRow checks that a click on a list row selects it,
// and that clicks on the title or below the last row do nothing.
func TestClickRowSelectsRow(t *testing.T) {
	m := New()
	m.SetRows(clickRows())
	m.Open()

	if !m.ClickRow(80, 24, m.listTopOffset()+2) {
		t.Fatal("ClickRow on a row should return true")
	}
	if m.selected != 2 {
		t.Errorf("selected = %d, want 2", m.selected)
	}
	if m.ClickRow(80, 24, m.listTopOffset()-1) {
		t.Error("ClickRow above the list should return false")
	}
	if m.ClickRow(80, 24, m.listTopOffset()+3) {
		t.Error("ClickRow past the last row should return false")
	}
	if m.selected != 2 {
		t.Errorf("a missed click moved the selection to %d", m.selected)
	}
}

// TestClickRowScrolledWindow checks that a click maps through the scroll
// window, not from the top of the list.
func TestClickRowScrolledWindow(t *testing.T) {
	rows := make([]Row, maxVisibleRows+4)
	for i := range rows {
		rows[i] = Row{Letter: string(rune('a' + i))}
	}
	m := New()
	m.SetRows(rows)
	m.Open()
	for range maxVisibleRows + 2 {
		m.HandleKey("down")
	}
	start, _ := m.visibleWindow(24)
	if start == 0 {
		t.Fatal("fixture should scroll the window")
	}
	if !m.ClickRow(80, 24, m.listTopOffset()) {
		t.Fatal("ClickRow on the first visible row should return true")
	}
	if m.selected != start {
		t.Errorf("selected = %d, want the window's first row %d", m.selected, start)
	}
}

// TestClickRowEmptyList checks that the "No marks set" line is not a row.
func TestClickRowEmptyList(t *testing.T) {
	m := New()
	m.Open()
	if m.ClickRow(80, 24, m.listTopOffset()) {
		t.Error("ClickRow on the empty-state line should return false")
	}
}

// TestShortTerminalKeepsSelectionOnScreen checks that in a terminal too
// short for ten rows the window shrinks so the box fits and the selected
// row is drawn, and that BoxSize and ClickRow use the same window.
func TestShortTerminalKeepsSelectionOnScreen(t *testing.T) {
	rows := make([]Row, maxVisibleRows+4)
	for i := range rows {
		rows[i] = Row{Letter: string(rune('a' + i)), ChannelName: "c"}
	}
	for _, termHeight := range []int{3, 8, 12, 16, 40} {
		m := New()
		m.SetRows(rows)
		m.Open()
		for range len(rows) - 1 {
			m.HandleKey("down")
		}
		box := m.renderBox(80, termHeight)
		w, h := m.BoxSize(80, termHeight)
		if gw, gh := lipgloss.Width(box), lipgloss.Height(box); w != gw || h != gh {
			t.Errorf("height %d: BoxSize = %dx%d, rendered %dx%d", termHeight, w, h, gw, gh)
		}
		if want := max(min(termHeight, maxVisibleRows+boxChromeRows), boxChromeRows+1); h != want {
			t.Errorf("height %d: box is %d rows, want %d", termHeight, h, want)
		}
		last := rows[len(rows)-1].Letter
		if !strings.Contains(ansi.Strip(box), "▌"+last+"  #c") {
			t.Errorf("height %d: the selected row %q is not drawn:\n%s", termHeight, last, box)
		}
		start, end := m.visibleWindow(termHeight)
		if !m.ClickRow(80, termHeight, m.listTopOffset()+end-start-1) || m.selected != end-1 {
			t.Errorf("height %d: a click on the last drawn row selected %d, want %d", termHeight, m.selected, end-1)
		}
		if m.ClickRow(80, termHeight, m.listTopOffset()+end-start) {
			t.Errorf("height %d: a click below the drawn rows hit a row", termHeight)
		}
	}
}

// TestFilterGeometry checks that the filter line moves the list down a
// row in BoxSize, the render and ClickRow alike, while it is typed and
// while it is kept, and that clicks map onto the filtered rows.
func TestFilterGeometry(t *testing.T) {
	for _, keys := range [][]string{{"/", "e"}, {"/", "e", "enter"}} {
		m := New()
		m.SetRows(clickRows())
		m.Open()
		idleTop := m.listTopOffset()
		_, idleH := m.BoxSize(80, 24)
		for _, k := range keys {
			m.HandleKey(k)
		}
		if got := m.listTopOffset(); got != idleTop+1 {
			t.Errorf("%v: listTopOffset = %d, want %d", keys, got, idleTop+1)
		}
		box := m.renderBox(80, 24)
		w, h := m.BoxSize(80, 24)
		if gw, gh := lipgloss.Width(box), lipgloss.Height(box); w != gw || h != gh {
			t.Errorf("%v: BoxSize = %dx%d, rendered %dx%d", keys, w, h, gw, gh)
		}
		// "e" leaves general and dev ("three"): two rows where three
		// were, plus the filter line, so the box keeps its height.
		if h != idleH {
			t.Errorf("%v: box is %d rows, want %d", keys, h, idleH)
		}
		lines := strings.Split(ansi.Strip(box), "\n")
		if !strings.Contains(lines[m.listTopOffset()], "#general") {
			t.Errorf("%v: row %d is %q, want the first filtered row", keys, m.listTopOffset(), lines[m.listTopOffset()])
		}
		if !m.ClickRow(80, 24, m.listTopOffset()+1) || m.selected != 1 {
			t.Fatalf("%v: click on the second filtered row selected %d", keys, m.selected)
		}
		if m.ClickRow(80, 24, m.listTopOffset()+2) {
			t.Errorf("%v: a click below the two filtered rows hit a row", keys)
		}
		if !m.filter.Typing() {
			if got := m.HandleKey("enter"); got == nil || got.Select != "c" {
				t.Errorf("%v: enter after the click = %+v, want Select=c", keys, got)
			}
		}
	}
}
