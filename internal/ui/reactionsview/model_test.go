package reactionsview

import (
	"strings"
	"testing"
)

func sampleGroups() []ReactionGroup {
	return []ReactionGroup{
		{Emoji: "thumbsup", Users: []string{"Alice", "Bob", "You (you)"}, Count: 3},
		{Emoji: "eyes", Users: []string{"Carol"}, Count: 1},
	}
}

func TestOpenCloseVisibility(t *testing.T) {
	m := New()
	if m.IsVisible() {
		t.Fatal("new model should not be visible")
	}
	m.Open(sampleGroups())
	if !m.IsVisible() {
		t.Fatal("Open should make the model visible")
	}
	m.Close()
	if m.IsVisible() {
		t.Fatal("Close should hide the model")
	}
}

func TestHandleKeyEscapeCloses(t *testing.T) {
	m := New()
	m.Open(sampleGroups())
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Fatal("esc should close the modal")
	}
}

func manyGroups(n int) []ReactionGroup {
	groups := make([]ReactionGroup, 0, n)
	for i := 0; i < n; i++ {
		groups = append(groups, ReactionGroup{
			Emoji: "thumbsup",
			Users: []string{"User"},
		})
	}
	return groups
}

func TestHandleKeyScrollClamps(t *testing.T) {
	m := New()
	// 10 groups -> 20 content lines (header + one user each). This overflows a
	// short terminal (maxOff > 0) but fits entirely in a tall one.
	m.Open(manyGroups(10))
	// A render must happen for maxOff to be computed; use a short height.
	m.ViewOverlay(80, 14, strings.Repeat("\n", 14))

	// Scrolling up at the top stays at 0.
	m.HandleKey("up")
	if m.Offset() != 0 {
		t.Fatalf("offset should clamp to 0 at top, got %d", m.Offset())
	}

	// Scrolling down far past the end clamps at the (positive) max offset.
	for i := 0; i < 500; i++ {
		m.HandleKey("down")
		m.ViewOverlay(80, 14, strings.Repeat("\n", 14)) // re-render recomputes maxOff
	}
	maxed := m.Offset()
	if maxed <= 0 {
		t.Fatalf("expected a positive clamped offset for overflowing content, got %d", maxed)
	}
	// One more down must not move past the clamp.
	m.HandleKey("down")
	if m.Offset() != maxed {
		t.Fatalf("offset should stay clamped at %d, got %d", maxed, m.Offset())
	}

	// A resize to a tall terminal (all 20 lines fit) re-clamps offset to 0.
	m.ViewOverlay(80, 40, strings.Repeat("\n", 40))
	if m.Offset() != 0 {
		t.Fatalf("offset should re-clamp to 0 when all content fits, got %d", m.Offset())
	}
}

func TestCountLabelKnownVsTotal(t *testing.T) {
	// Equal known/total -> plain number.
	if got := countLabel(3, 3); got != "3" {
		t.Fatalf("countLabel(3,3) = %q, want \"3\"", got)
	}
	// Unset total (0) falls back to known.
	if got := countLabel(3, 0); got != "3" {
		t.Fatalf("countLabel(3,0) = %q, want \"3\"", got)
	}
	// Slack reported more reactors than we have cached -> known/total.
	if got := countLabel(3, 8); got != "3/8" {
		t.Fatalf("countLabel(3,8) = %q, want \"3/8\"", got)
	}
}

func TestViewOverlayRendersTruncatedCount(t *testing.T) {
	m := New()
	// Slack says 8 reacted but only 2 user IDs are cached.
	m.Open([]ReactionGroup{
		{Emoji: "thumbsup", Users: []string{"Alice", "Bob"}, Count: 8},
	})
	out := m.ViewOverlay(80, 24, strings.Repeat("\n", 24))
	if !strings.Contains(out, "2/8") {
		t.Fatalf("expected header to show known/total \"2/8\", got:\n%s", out)
	}
}

func TestViewOverlayEdgeCasesDoNotPanic(t *testing.T) {
	// Empty groups must render without panicking and still show the title.
	m := New()
	m.Open(nil)
	out := m.ViewOverlay(80, 24, strings.Repeat("\n", 24))
	if !strings.Contains(out, "Reactions") {
		t.Fatalf("empty modal should still render the title, got:\n%s", out)
	}

	// A 1-row terminal must not panic.
	m.Open(sampleGroups())
	_ = m.ViewOverlay(80, 1, "\n")

	// A very narrow terminal must not panic.
	_ = m.ViewOverlay(2, 24, strings.Repeat("\n", 24))
}

func TestViewOverlayRendersNamesAndCounts(t *testing.T) {
	m := New()
	out := m.ViewOverlay(80, 24, "background")
	if out != "background" {
		t.Fatal("hidden modal should return background unchanged")
	}

	m.Open(sampleGroups())
	out = m.ViewOverlay(80, 24, strings.Repeat("\n", 24))
	for _, want := range []string{"Reactions", "Alice", "Bob", "Carol", "You (you)", "(3)", "(1)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered modal missing %q\n%s", want, out)
		}
	}
}

// openWith opens sampleGroups and sends keys.
func openWith(keys ...string) *Model {
	m := New()
	m.Open(sampleGroups())
	for _, k := range keys {
		m.HandleKey(k)
	}
	return m
}

// shownText is the filtered list as "emoji:user,user;…".
func shownText(m *Model) string {
	var parts []string
	for _, sg := range m.shown {
		parts = append(parts, m.groups[sg.group].Emoji+":"+strings.Join(sg.users, ","))
	}
	return strings.Join(parts, ";")
}

func TestFilterGroupsAndUsers(t *testing.T) {
	for _, c := range []struct {
		keys []string
		want string
	}{
		{nil, "thumbsup:Alice,Bob,You (you);eyes:Carol"},
		{[]string{"/", "t", "h", "u"}, "thumbsup:Alice,Bob,You (you)"}, // emoji name keeps every user
		{[]string{"/", "e", "y"}, "eyes:Carol"},
		{[]string{"/", "b", "o", "b"}, "thumbsup:Bob"},                                   // only matching users; eyes dropped
		{[]string{"/", "c", "a", "r"}, "eyes:Carol"},                                     // user match keeps its group
		{[]string{"/", "y", "o", "u"}, "thumbsup:You (you)"},                             // users only: no emoji has "you"
		{[]string{"/", "z", "z"}, ""},                                                    // nothing left
		{[]string{"/", "b", "o", "b", "esc"}, "thumbsup:Alice,Bob,You (you);eyes:Carol"}, // esc clears
	} {
		if got := shownText(openWith(c.keys...)); got != c.want {
			t.Errorf("%v: shown %q, want %q", c.keys, got, c.want)
		}
	}
}

// TestFilterHeaderCountsTheWholeGroup: a group filtered down to some of
// its users still reports how many reacted.
func TestFilterHeaderCountsTheWholeGroup(t *testing.T) {
	m := openWith("/", "b", "o", "b")
	out := m.renderBox(80, 24)
	if !strings.Contains(out, "(3)") || strings.Contains(out, "Alice") {
		t.Errorf("filtered view must show (3) and only Bob:\n%s", out)
	}
}

func TestFilterKeysAreTextWhileTyping(t *testing.T) {
	m := openWith("/", "q", "L", "j", "k")
	if !m.IsVisible() {
		t.Fatal("a key typed into the filter closed the modal")
	}
	if m.Query() != "qLjk" || !m.Typing() {
		t.Errorf("query %q, typing %v; want qLjk, true", m.Query(), m.Typing())
	}
}

// TestFilterKeptEscClearsThenQCloses: with a kept filter the first esc
// clears it and keeps the modal; q and L close over a kept filter, as
// in the help overlay and the link picker.
func TestFilterKeptEscClearsThenQCloses(t *testing.T) {
	m := openWith("/", "b", "o", "b", "enter")
	m.HandleKey("escape")
	if !m.IsVisible() || m.Query() != "" {
		t.Fatalf("esc with a kept filter: visible %v, query %q; want open and cleared", m.IsVisible(), m.Query())
	}
	m.HandleKey("q")
	if m.IsVisible() {
		t.Error("q with no filter should close")
	}
	for _, k := range []string{"q", "L"} {
		m := openWith("/", "b", "o", "b", "enter")
		m.HandleKey(k)
		if m.IsVisible() {
			t.Errorf("%s with a kept filter did not close the modal", k)
		}
	}
}

func TestFilterChangeResetsScroll(t *testing.T) {
	m := New()
	m.Open(manyGroups(10))
	m.ViewOverlay(80, 14, strings.Repeat("\n", 14))
	for range 3 {
		m.HandleKey("down")
	}
	if m.Offset() != 3 {
		t.Fatalf("precondition: offset = %d, want 3", m.Offset())
	}
	m.HandleKey("/")
	if m.Offset() != 3 {
		t.Errorf("/ alone changed the offset to %d", m.Offset())
	}
	m.HandleKey("u")
	if m.Offset() != 0 {
		t.Errorf("a filter change left the offset at %d, want 0", m.Offset())
	}
	m.HandleKey("down") // arrows scroll while typing
	if m.Offset() != 1 || m.Query() != "u" {
		t.Errorf("down while typing: offset %d, query %q; want 1, u", m.Offset(), m.Query())
	}
}

// TestFilterLineTakesAWindowRow: the filter line is paid for with one
// list row, so the box keeps its height and stays inside the terminal.
func TestFilterLineTakesAWindowRow(t *testing.T) {
	m := New()
	m.Open(manyGroups(20))
	idle := strings.Count(m.renderBox(80, 20), "\n")
	for _, k := range []string{"/", "u", "enter"} {
		m.HandleKey(k)
	}
	if got := strings.Count(m.renderBox(80, 20), "\n"); got != idle {
		t.Errorf("box with the filter line is %d lines, want %d", got+1, idle+1)
	}
}

func TestViewFilterStates(t *testing.T) {
	out := openWith().renderBox(80, 24)
	if strings.Contains(out, "filter:") || !strings.Contains(out, "/ filter") {
		t.Errorf("idle: no filter line, footer advertises / filter:\n%s", out)
	}
	m := openWith("/", "z", "z")
	if out := m.renderBox(80, 24); !strings.Contains(out, "zz█") || !strings.Contains(out, "No matching reactions") {
		t.Errorf("typing with no match:\n%s", out)
	}
	m.HandleKey("enter")
	if out := m.renderBox(80, 24); !strings.Contains(out, "filter: zz") {
		t.Errorf("kept filter:\n%s", out)
	}
}
