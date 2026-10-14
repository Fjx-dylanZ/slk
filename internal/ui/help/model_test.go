package help

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
)

func sampleEntries() []Entry {
	return []Entry{
		{Key: "?", Desc: "show keybindings"},
		{Key: "j/down", Desc: "down"},
		{Key: "k/up", Desc: "up"},
		{Key: "/", Desc: "search"},
		{Key: "ctrl+t", Desc: "fuzzy find"},
		{Key: "Q", Desc: "quit (confirm)"},
	}
}

func TestOpenClose(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	if m.IsVisible() {
		t.Error("should not be visible initially")
	}
	m.Open()
	if !m.IsVisible() {
		t.Error("should be visible after Open")
	}
	m.Close()
	if m.IsVisible() {
		t.Error("should not be visible after Close")
	}
}

func TestEscClosesModal(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Error("should be closed after esc")
	}
}

func TestQClosesModal(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("q")
	if m.IsVisible() {
		t.Error("should be closed after q")
	}
}

func TestQuestionMarkClosesModal(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("?")
	if m.IsVisible() {
		t.Error("should be closed after ?")
	}
}

func TestNavigationDown(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	if m.Selected() != 0 {
		t.Errorf("initial Selected = %d, want 0", m.Selected())
	}
	m.HandleKey("j")
	if m.Selected() != 1 {
		t.Errorf("after j, Selected = %d, want 1", m.Selected())
	}
	m.HandleKey("down")
	if m.Selected() != 2 {
		t.Errorf("after down, Selected = %d, want 2", m.Selected())
	}
}

func TestNavigationUp(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("j")
	m.HandleKey("j")
	m.HandleKey("k")
	if m.Selected() != 1 {
		t.Errorf("after j j k, Selected = %d, want 1", m.Selected())
	}
	m.HandleKey("up")
	if m.Selected() != 0 {
		t.Errorf("after up, Selected = %d, want 0", m.Selected())
	}
}

func TestNavigationBoundaries(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("k") // already at top
	if m.Selected() != 0 {
		t.Errorf("up at top, Selected = %d, want 0", m.Selected())
	}
	for i := 0; i < 20; i++ {
		m.HandleKey("j")
	}
	if m.Selected() != len(sampleEntries())-1 {
		t.Errorf("down past bottom, Selected = %d, want %d", m.Selected(), len(sampleEntries())-1)
	}
}

func TestSearchModeFiltersByDescription(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	if !m.IsSearching() {
		t.Error("should be in search mode after /")
	}
	m.HandleKey("d")
	m.HandleKey("o")
	m.HandleKey("w")
	m.HandleKey("n")
	visible := m.VisibleEntries()
	if len(visible) != 1 {
		t.Fatalf("filter 'down' should match 1 entry, got %d: %v", len(visible), visible)
	}
	if visible[0].Desc != "down" {
		t.Errorf("expected 'down', got %q", visible[0].Desc)
	}
}

func TestSearchModeMatchesKeyToo(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("c")
	m.HandleKey("t")
	m.HandleKey("r")
	m.HandleKey("l")
	visible := m.VisibleEntries()
	if len(visible) != 1 {
		t.Fatalf("filter 'ctrl' should match 1 entry, got %d", len(visible))
	}
	if visible[0].Key != "ctrl+t" {
		t.Errorf("expected ctrl+t, got %q", visible[0].Key)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("D")
	m.HandleKey("O")
	m.HandleKey("W")
	m.HandleKey("N")
	visible := m.VisibleEntries()
	if len(visible) != 1 {
		t.Errorf("uppercase filter should match case-insensitively, got %d entries", len(visible))
	}
}

func TestSearchBackspace(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("d")
	m.HandleKey("o")
	m.HandleKey("backspace")
	if m.Query() != "d" {
		t.Errorf("Query after backspace = %q, want d", m.Query())
	}
}

func TestEscFromSearchExitsSearch(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("d")
	m.HandleKey("esc")
	if !m.IsVisible() {
		t.Error("first esc should exit search, not close modal")
	}
	if m.IsSearching() {
		t.Error("should no longer be searching")
	}
	if m.Query() != "" {
		t.Errorf("query should be cleared, got %q", m.Query())
	}
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Error("second esc should close modal")
	}
}

func TestQDoesNotCloseDuringSearch(t *testing.T) {
	// While typing in the search box, q is a filter character, not close.
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("q")
	if !m.IsVisible() {
		t.Error("q during search should not close modal")
	}
	if m.Query() != "q" {
		t.Errorf("Query = %q, want q", m.Query())
	}
}

func TestSearchEnterExitsSearchKeepsFilter(t *testing.T) {
	// Enter while searching exits search mode but keeps the filter applied,
	// so the user can navigate with j/k over the filtered list.
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("d")
	m.HandleKey("enter")
	if m.IsSearching() {
		t.Error("enter should exit search mode")
	}
	if m.Query() != "d" {
		t.Errorf("Query after enter = %q, want d preserved", m.Query())
	}
}

// TestEscAfterEnterClearsFilterThenCloses pins the uniform listfilter
// rule: with a filter kept, the first esc clears it and the second closes.
func TestEscAfterEnterClearsFilterThenCloses(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	for _, k := range []string{"/", "d", "o", "w", "n", "enter"} {
		m.HandleKey(k)
	}
	if n := len(m.VisibleEntries()); n != 1 {
		t.Fatalf("precondition: kept filter shows %d entries, want 1", n)
	}
	m.HandleKey("esc")
	if !m.IsVisible() {
		t.Fatal("first esc after enter closed the modal; it should clear the filter")
	}
	if m.Query() != "" || m.IsSearching() {
		t.Errorf("after esc: query %q, searching %v; want cleared and idle", m.Query(), m.IsSearching())
	}
	if n := len(m.VisibleEntries()); n != len(sampleEntries()) {
		t.Errorf("after esc: %d entries visible, want all %d", n, len(sampleEntries()))
	}
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Error("second esc should close the modal")
	}
}

func TestQuestionMarkTypesWhileSearching(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("?")
	if !m.IsVisible() {
		t.Fatal("? while searching closed the modal")
	}
	if m.Query() != "?" {
		t.Errorf("Query = %q, want ?", m.Query())
	}
	if v := m.VisibleEntries(); len(v) != 1 || v[0].Key != "?" {
		t.Errorf("visible = %v, want the ? entry alone", v)
	}
}

func TestQClosesWithAKeptFilter(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	for _, k := range []string{"/", "d", "enter", "q"} {
		m.HandleKey(k)
	}
	if m.IsVisible() {
		t.Error("q with a kept filter (not typing) should close the modal")
	}
}

func TestSearchTakesAnyPrintableRuneAndSpace(t *testing.T) {
	m := New()
	m.SetEntries(append(sampleEntries(), Entry{Key: "x", Desc: "café menu"}))
	m.Open()
	for _, k := range []string{"/", "c", "a", "f", "é", "space", "m"} {
		m.HandleKey(k)
	}
	if m.Query() != "café m" {
		t.Fatalf("Query = %q, want %q", m.Query(), "café m")
	}
	if v := m.VisibleEntries(); len(v) != 1 || v[0].Desc != "café menu" {
		t.Errorf("visible = %v, want the café entry alone", v)
	}
	m.HandleKey("backspace")
	m.HandleKey("backspace")
	if m.Query() != "café" {
		t.Errorf("Query after two backspaces = %q, want %q: backspace deletes a rune", m.Query(), "café")
	}
	m.HandleKey("backspace")
	if m.Query() != "caf" {
		t.Errorf("Query after backspace over é = %q, want %q", m.Query(), "caf")
	}
}

func TestArrowsNavigateWhileSearching(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	m.HandleKey("/")
	m.HandleKey("down")
	if !m.IsSearching() {
		t.Error("down left search mode")
	}
	if m.Selected() != 1 {
		t.Errorf("Selected after down while searching = %d, want 1", m.Selected())
	}
	m.HandleKey("up")
	if m.Selected() != 0 {
		t.Errorf("Selected after up while searching = %d, want 0", m.Selected())
	}
}

func TestInputLineFollowsTheFilter(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	if box := m.renderBox(80, 24); !strings.Contains(box, "Press / to search") {
		t.Error("idle: the search hint is missing")
	}
	m.HandleKey("/")
	m.HandleKey("d")
	if box := m.renderBox(80, 24); !strings.Contains(box, "d█") {
		t.Error("typing: the filter input is missing")
	}
	m.HandleKey("enter")
	if box := m.renderBox(80, 24); !strings.Contains(box, "filter: d") {
		t.Error("kept: the filter: line is missing")
	}
}

func TestFromKeyMap(t *testing.T) {
	// FromKeyMap should derive entries from any struct of key.Binding fields.
	type sampleMap struct {
		A key.Binding
		B key.Binding
	}
	km := sampleMap{
		A: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alpha")),
		B: key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "beta")),
	}
	entries := FromKeyMap(km)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Verify both entries are present (order may be sorted).
	keys := make(map[string]string)
	for _, e := range entries {
		keys[e.Key] = e.Desc
	}
	if keys["a"] != "alpha" || keys["b"] != "beta" {
		t.Errorf("expected alpha and beta entries, got %v", keys)
	}
}

func TestFromKeyMapSkipsEmptyHelp(t *testing.T) {
	// Bindings without help metadata should be skipped.
	type sampleMap struct {
		A key.Binding
		B key.Binding
	}
	km := sampleMap{
		A: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alpha")),
		B: key.NewBinding(key.WithKeys("b")), // no help
	}
	entries := FromKeyMap(km)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Desc != "alpha" {
		t.Errorf("expected alpha, got %q", entries[0].Desc)
	}
}

func TestFromKeyMapSorted(t *testing.T) {
	// Entries returned should be alphabetized by description for consistent display.
	type sampleMap struct {
		A key.Binding
		B key.Binding
		C key.Binding
	}
	km := sampleMap{
		A: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "zeta")),
		B: key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "alpha")),
		C: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "mu")),
	}
	entries := FromKeyMap(km)
	descs := []string{entries[0].Desc, entries[1].Desc, entries[2].Desc}
	want := []string{"alpha", "mu", "zeta"}
	for i, w := range want {
		if descs[i] != w {
			t.Errorf("entries[%d].Desc = %q, want %q (full: %v)", i, descs[i], w, descs)
		}
	}
}

func TestViewOverlayInvisibleReturnsBackground(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	// Not open
	out := m.ViewOverlay(80, 24, "background")
	if out != "background" {
		t.Errorf("invisible overlay should return background unchanged, got %q", out)
	}
}

func TestViewOverlayContainsTitle(t *testing.T) {
	m := New()
	m.SetEntries(sampleEntries())
	m.Open()
	out := m.ViewOverlay(80, 24, strings.Repeat(" ", 80*24))
	if !strings.Contains(out, "Keybindings") {
		t.Error("rendered overlay should contain title 'Keybindings'")
	}
}
