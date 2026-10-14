package linkpicker

import (
	"strings"
	"testing"
)

func items3() []Item {
	return []Item{
		{URL: "https://a.example/1", Label: "first"},
		{URL: "https://b.example/2"},
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000001", InApp: true},
	}
}

func TestOpenAndNavigate(t *testing.T) {
	m := New()
	if m.IsVisible() {
		t.Fatal("visible before Open")
	}
	m.Open("Open link", items3())
	if !m.IsVisible() {
		t.Fatal("not visible after Open")
	}
	if _, chosen := m.HandleKey("j"); chosen {
		t.Error("j must not choose")
	}
	m.HandleKey("j")
	item, chosen := m.HandleKey("enter")
	if !chosen {
		t.Fatal("enter should choose")
	}
	if item.URL != items3()[2].URL {
		t.Errorf("chose %q", item.URL)
	}
	if m.IsVisible() {
		t.Error("should close after choose")
	}
}

func TestNavigationBounds(t *testing.T) {
	m := New()
	m.Open("Open link", items3())
	m.HandleKey("k") // at top; no-op
	item, chosen := m.HandleKey("enter")
	if !chosen || item.URL != items3()[0].URL {
		t.Errorf("chose %+v chosen=%v", item, chosen)
	}
	m.Open("Open link", items3())
	for i := 0; i < 10; i++ {
		m.HandleKey("j") // clamps at bottom
	}
	item, _ = m.HandleKey("enter")
	if item.URL != items3()[2].URL {
		t.Errorf("chose %q", item.URL)
	}
}

func TestEscCloses(t *testing.T) {
	m := New()
	m.Open("Open link", items3())
	if _, chosen := m.HandleKey("esc"); chosen {
		t.Error("esc must not choose")
	}
	if m.IsVisible() {
		t.Error("esc should close")
	}
}

func TestEnterOnEmptyIsNoop(t *testing.T) {
	m := New()
	m.Open("Open link", nil)
	if _, chosen := m.HandleKey("enter"); chosen {
		t.Error("enter on empty list must not choose")
	}
}

func TestOpenAssignsIndexAndTitle(t *testing.T) {
	m := New()
	m.Open("Download file", items3())
	if m.Title() != "Download file" {
		t.Errorf("title = %q", m.Title())
	}
	m.HandleKey("j")
	item, chosen := m.HandleKey("enter")
	if !chosen || item.Index != 1 {
		t.Errorf("chosen item = %+v chosen=%v", item, chosen)
	}
}

// filterItems are a link, a file and an in-app permalink: "report"
// is the file's label, "example" both URLs, "mb" the file's detail.
func filterItems() []Item {
	return []Item{
		{URL: "https://example.com/a", Label: "Example"},
		{URL: "https://files.example/report.pdf", Label: "report.pdf", Detail: "1.2 MB"},
		{URL: "https://myteam.slack.com/archives/C1/p1", InApp: true},
	}
}

func openFiltered(keys ...string) *Model {
	m := New()
	m.Open("Open link", filterItems())
	for _, k := range keys {
		m.HandleKey(k)
	}
	return m
}

func shownIndexes(m *Model) []int {
	out := []int{}
	for _, i := range m.shown {
		out = append(out, m.items[i].Index)
	}
	return out
}

func TestFilterMatchesLabelURLDetail(t *testing.T) {
	for _, c := range []struct {
		query string
		want  []int
	}{
		{"", []int{0, 1, 2}},
		{"report", []int{1}},     // label
		{"example", []int{0, 1}}, // label and URLs
		{"archives", []int{2}},   // URL only
		{"mb", []int{1}},         // detail, case folded
		{"zz", []int{}},
	} {
		keys := []string{"/"}
		for _, r := range c.query {
			keys = append(keys, string(r))
		}
		got := shownIndexes(openFiltered(keys...))
		if len(got) != len(c.want) {
			t.Errorf("filter %q shows %v, want %v", c.query, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("filter %q shows %v, want %v", c.query, got, c.want)
				break
			}
		}
	}
}

// TestFilterTypesQAndJ: while the filter is typed, q and j are text,
// not close and move.
func TestFilterTypesQAndJ(t *testing.T) {
	m := openFiltered("/", "q", "j")
	if !m.IsVisible() {
		t.Fatal("q while typing closed the picker")
	}
	if m.Query() != "qj" || m.Selected() != 0 {
		t.Errorf("query %q, selected %d; want qj, 0", m.Query(), m.Selected())
	}
}

// TestFilterEnterKeepsThenChoosesOriginalItem: the first enter keeps
// the filter, the second chooses the highlighted filtered row, and the
// chosen item's Index still names its place in the slice given to Open.
func TestFilterEnterKeepsThenChoosesOriginalItem(t *testing.T) {
	m := openFiltered("/", "a", "r", "c")
	if _, chosen := m.HandleKey("enter"); chosen {
		t.Fatal("enter while typing chose a row; it should keep the filter")
	}
	if m.Typing() || m.Query() != "arc" || !m.IsVisible() {
		t.Fatalf("after enter: typing %v, query %q, visible %v", m.Typing(), m.Query(), m.IsVisible())
	}
	item, chosen := m.HandleKey("enter")
	if !chosen || item.Index != 2 || item.URL != filterItems()[2].URL {
		t.Errorf("chose %+v (chosen %v), want the permalink at Index 2", item, chosen)
	}
}

func TestFilterArrowsMoveWhileTyping(t *testing.T) {
	m := openFiltered("/", "e", "x", "a", "down")
	if !m.Typing() || m.Selected() != 1 {
		t.Fatalf("down while typing: typing %v, selected %d; want true, 1", m.Typing(), m.Selected())
	}
	m.HandleKey("enter")
	item, _ := m.HandleKey("enter")
	if item.Index != 1 {
		t.Errorf("chose Index %d, want 1", item.Index)
	}
}

func TestFilterEscClearsThenQCloses(t *testing.T) {
	m := openFiltered("/", "r", "e", "p", "enter")
	m.HandleKey("esc")
	if !m.IsVisible() || m.Query() != "" || len(m.shown) != 3 {
		t.Fatalf("esc with a kept filter: visible %v, query %q, %d shown", m.IsVisible(), m.Query(), len(m.shown))
	}
	m = openFiltered("/", "r", "e", "p", "enter", "q")
	if m.IsVisible() {
		t.Error("q with a kept filter (not typing) should close")
	}
}

func TestFilterSpaceTypes(t *testing.T) {
	m := New()
	m.Open("Download file", []Item{{Label: "q3 report.pdf"}, {Label: "q3-report.pdf"}})
	for _, k := range []string{"/", "3", "space", "r"} {
		m.HandleKey(k)
	}
	if got := shownIndexes(m); len(got) != 1 || got[0] != 0 {
		t.Errorf("filter %q shows %v, want [0]", m.Query(), got)
	}
}

func TestViewFilterStates(t *testing.T) {
	m := openFiltered()
	view := m.renderBox(100)
	if strings.Contains(view, "Type to filter") || strings.Contains(view, "filter: ") {
		t.Error("idle: no filter line expected")
	}
	if !strings.Contains(view, "/ filter") {
		t.Errorf("footer must advertise / filter:\n%s", view)
	}
	m = openFiltered("/", "z", "z")
	if view := m.renderBox(100); !strings.Contains(view, "zz█") || !strings.Contains(view, "No matching items") {
		t.Errorf("typing a filter with no match:\n%s", view)
	}
	m.HandleKey("enter")
	if view := m.renderBox(100); !strings.Contains(view, "filter: zz") {
		t.Errorf("kept filter:\n%s", view)
	}
}
