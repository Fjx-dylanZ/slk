package marks

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestHandleKey_LetterJumpsImmediately(t *testing.T) {
	m := New()
	m.SetRows([]Row{
		{Letter: "a", ChannelName: "general", AuthorName: "alice", Excerpt: "hello"},
		{Letter: "b", ChannelName: "random", AuthorName: "bob", Excerpt: "world"},
	})
	if got := m.HandleKey("a"); got == nil || got.Select != "a" || got.Delete != "" {
		t.Fatalf("letter 'a' produced %+v, want Select=a", got)
	}
}

func TestHandleKey_EnterSelectsHighlightedRow(t *testing.T) {
	m := New()
	m.SetRows([]Row{
		{Letter: "a", ChannelName: "general", Excerpt: "x"},
		{Letter: "b", ChannelName: "random", Excerpt: "y"},
	})
	m.HandleKey("down")
	if got := m.HandleKey("enter"); got == nil || got.Select != "b" {
		t.Fatalf("enter produced %+v, want Select=b", got)
	}
}

func TestHandleKey_DeleteRemovesHighlightedRow(t *testing.T) {
	m := New()
	m.SetRows([]Row{
		{Letter: "a", ChannelName: "general", Excerpt: "x"},
		{Letter: "b", ChannelName: "random", Excerpt: "y"},
	})
	for _, key := range []string{"backspace", "delete"} {
		if got := m.HandleKey(key); got == nil || got.Delete != "a" || got.Select != "" {
			t.Fatalf("%s produced %+v, want Delete=a", key, got)
		}
	}
}

func TestHandleKey_EscCloses(t *testing.T) {
	m := New()
	m.Open()
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Fatal("esc must close the overlay")
	}
}

func TestHandleKey_EmptyListEnterIsNoop(t *testing.T) {
	m := New()
	if got := m.HandleKey("enter"); got != nil {
		t.Fatalf("enter on an empty list produced %+v", got)
	}
}

func TestView_RendersSnapshot(t *testing.T) {
	m := New()
	m.SetRows([]Row{
		{Letter: "a", ChannelName: "general", AuthorName: "alice", Excerpt: "hello world"},
	})
	m.Open()
	view := m.View(80, 24)
	for _, want := range []string{"a", "#general", "alice: hello world"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestView_EmptyState(t *testing.T) {
	m := New()
	m.Open()
	if !strings.Contains(m.View(80, 24), "No marks set") {
		t.Fatalf("empty overlay must report no marks set:\n%s", m.View(80, 24))
	}
}

// The excerpt may be long or contain wide glyphs (emoji); the row is
// truncated on measured width, so every rendered line must fit the box
// and long excerpts must carry a truncation tail.
func TestView_TruncatesWideExcerptsToWidth(t *testing.T) {
	m := New()
	var rows []Row
	for i := 0; i < 12; i++ {
		rows = append(rows, Row{
			Letter:      string(rune('a' + i)),
			ChannelName: "general",
			AuthorName:  "alice",
			Excerpt:     strings.Repeat("hello 😀 ", 10),
		})
	}
	m.SetRows(rows)
	m.Open()
	view := m.View(60, 24)
	boxWidth := 30 // boxWidth(60) == 60/2
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > boxWidth {
			t.Errorf("row overflows the box (%d cells, want <= %d): %q", w, boxWidth, line)
		}
	}
	if !strings.Contains(view, "…") {
		t.Fatalf("long excerpts must be truncated with a tail marker:\n%s", view)
	}
}

// filterRows are four marks whose fields each match a different query:
// "deploy" is in b's channel and d's excerpt, "carol" is c's author.
func filterRows() []Row {
	return []Row{
		{Letter: "a", ChannelName: "general", AuthorName: "alice", Excerpt: "hello"},
		{Letter: "b", ChannelName: "deploys", AuthorName: "bob", Excerpt: "green"},
		{Letter: "c", ChannelName: "random", AuthorName: "Carol", Excerpt: "lunch?"},
		{Letter: "d", ChannelName: "eng", AuthorName: "dave", Excerpt: "Déploy at 5"},
	}
}

// openFiltered opens the overlay over filterRows and sends keys.
func openFiltered(keys ...string) Model {
	m := New()
	m.SetRows(filterRows())
	m.Open()
	for _, k := range keys {
		m.HandleKey(k)
	}
	return m
}

func shownLetters(m Model) string {
	var b strings.Builder
	for _, i := range m.shown {
		b.WriteString(m.rows[i].Letter)
	}
	return b.String()
}

func TestFilter_MatchesLetterChannelAuthorExcerpt(t *testing.T) {
	for _, c := range []struct {
		query, want string
	}{
		{"", "abcd"},
		{"deploy", "bd"}, // channel, and excerpt with an accent folded
		{"carol", "c"},   // author, case folded
		{"lunch?", "c"},  // excerpt with punctuation
		{"o", "abcd"},    // hellO, bOb, randOm, Déploy
		{"b", "b"},       // the letter, and bob
		{"zz", ""},
	} {
		keys := []string{"/"}
		for _, r := range c.query {
			keys = append(keys, string(r))
		}
		if got := shownLetters(openFiltered(keys...)); got != c.want {
			t.Errorf("filter %q shows %q, want %q", c.query, got, c.want)
		}
	}
}

// TestFilter_LettersTypeWhileTyping: mark letters are filter text while
// the filter is typed and jump again once Enter has kept it.
func TestFilter_LettersTypeWhileTyping(t *testing.T) {
	m := openFiltered("/")
	if !m.Typing() {
		t.Fatal("/ did not start the filter")
	}
	for _, k := range []string{"c", "a", "r", "'", "`"} {
		if got := m.HandleKey(k); got != nil {
			t.Fatalf("%q while typing produced %+v, want filter text", k, got)
		}
	}
	if m.filter.Query() != "car'`" {
		t.Errorf("query = %q, want %q", m.filter.Query(), "car'`")
	}
	m = openFiltered("/", "c", "a", "r", "enter")
	if m.Typing() {
		t.Fatal("enter did not stop typing")
	}
	if got := m.HandleKey("a"); got == nil || got.Select != "a" {
		t.Errorf("letter with a kept filter produced %+v, want Select=a", got)
	}
}

func TestFilter_EnterSelectsHighlightedFilteredRow(t *testing.T) {
	m := openFiltered("/", "d", "e", "p", "down")
	if got := m.HandleKey("enter"); got != nil {
		t.Fatalf("enter while typing produced %+v, want it to keep the filter", got)
	}
	if got := m.HandleKey("enter"); got == nil || got.Select != "d" {
		t.Errorf("enter on the second filtered row produced %+v, want Select=d", got)
	}
}

func TestFilter_DeleteRemovesHighlightedFilteredRow(t *testing.T) {
	m := openFiltered("/", "d", "e", "p", "enter", "down")
	got := m.HandleKey("backspace")
	if got == nil || got.Delete != "d" {
		t.Fatalf("backspace on the second filtered row produced %+v, want Delete=d", got)
	}
	// The App refreshes the rows after the delete; the filter survives.
	rows := filterRows()[:3]
	m.SetRows(rows)
	if m.filter.Query() != "dep" || shownLetters(m) != "b" || m.selected != 0 {
		t.Errorf("after refresh: query %q, shown %q, selected %d; want dep, b, 0", m.filter.Query(), shownLetters(m), m.selected)
	}
	if got := m.HandleKey("delete"); got == nil || got.Delete != "b" {
		t.Errorf("delete after refresh produced %+v, want Delete=b", got)
	}
}

func TestFilter_EscClearsThenCloses(t *testing.T) {
	m := openFiltered("/", "c", "a", "r")
	m.HandleKey("esc")
	if !m.IsVisible() || m.filter.Active() || shownLetters(m) != "abcd" {
		t.Fatalf("esc while typing: visible %v, active %v, shown %q", m.IsVisible(), m.filter.Active(), shownLetters(m))
	}
	m = openFiltered("/", "c", "a", "r", "enter")
	m.HandleKey("esc")
	if !m.IsVisible() || m.filter.Active() {
		t.Fatalf("esc with a kept filter: visible %v, active %v; want open and cleared", m.IsVisible(), m.filter.Active())
	}
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Error("esc with no filter must close the overlay")
	}
}

func TestFilter_OpenResetsIt(t *testing.T) {
	m := openFiltered("/", "c", "a", "r", "enter")
	m.Open()
	if m.filter.Active() || shownLetters(m) != "abcd" {
		t.Errorf("Open kept the filter: active %v, shown %q", m.filter.Active(), shownLetters(m))
	}
}

func TestView_FilterStates(t *testing.T) {
	m := openFiltered()
	if strings.Contains(m.View(80, 24), "filter") {
		t.Error("idle: the box must not draw a filter line")
	}
	m = openFiltered("/", "z", "z")
	view := m.View(80, 24)
	if !strings.Contains(view, "zz█") || !strings.Contains(view, "No matching marks") {
		t.Errorf("typing with no match must show the input and No matching marks:\n%s", view)
	}
	m.HandleKey("enter")
	if !strings.Contains(m.View(80, 24), "filter: zz") {
		t.Errorf("kept filter must show filter: zz:\n%s", m.View(80, 24))
	}
}
