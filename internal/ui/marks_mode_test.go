// internal/ui/marks_mode_test.go
//
// Tests for the marks overlay at the App level: the ' chord opening it
// (or not, with the option off), immediate jumps, row selection,
// deletion, and snapshot-only rendering.
package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
)

func TestMarksOverlay_OpensOnJumpByDefault(t *testing.T) {
	app := jumpMarkTestApp(t)
	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})
	if app.mode != ModeMarks || !app.marksOverlay.IsVisible() {
		t.Fatalf("' must open the marks overlay, mode=%v visible=%v", app.mode, app.marksOverlay.IsVisible())
	}
}

func TestMarksOverlay_LetterPressJumpsImmediately(t *testing.T) {
	app := jumpMarkTestApp(t)
	seedMark(t, app, "a", Location{TeamID: "T1", ChannelID: "C2", MessageTS: "10.0"})

	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})

	// The letter jumps in one press — the overlay must not add a
	// keystroke to the 'a flow.
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if cmd == nil {
		t.Fatal("letter press while the overlay is open must jump immediately")
	}
	cs, ok := cmd().(ChannelSelectedMsg)
	if !ok {
		t.Fatalf("want ChannelSelectedMsg, got %T", cmd())
	}
	if cs.ID != "C2" {
		t.Fatalf("jumped to %q, want C2", cs.ID)
	}
	if app.mode != ModeNormal || app.marksOverlay.IsVisible() {
		t.Fatal("the jump must close the overlay")
	}
}

func TestMarksOverlay_SelectRowJumps(t *testing.T) {
	app := jumpMarkTestApp(t)
	seedMark(t, app, "a", Location{TeamID: "T1", ChannelID: "C2", MessageTS: "10.0"})
	seedMark(t, app, "b", Location{TeamID: "T1", ChannelID: "C3", MessageTS: "20.0"})

	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})
	app.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must jump to the highlighted mark")
	}
	cs, ok := cmd().(ChannelSelectedMsg)
	if !ok || cs.ID != "C3" {
		t.Fatalf("selected row jumped to %T %+v, want C3", cmd(), cmd())
	}
	if app.mode != ModeNormal {
		t.Fatal("a row selection must close the overlay")
	}
}

func TestMarksOverlay_DeleteRemovesRowAndKeepsRest(t *testing.T) {
	app := jumpMarkTestApp(t)
	seedMark(t, app, "a", Location{TeamID: "T1", ChannelID: "C2", MessageTS: "10.0"})
	seedMark(t, app, "b", Location{TeamID: "T1", ChannelID: "C3", MessageTS: "20.0"})

	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})
	if _, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyBackspace}); cmd != nil {
		t.Fatalf("delete produced a cmd: %v", cmd)
	}

	if _, ok, _ := app.marks.Load("T1", "a"); ok {
		t.Fatal("mark a must be removed by the delete action")
	}
	if _, ok, _ := app.marks.Load("T1", "b"); !ok {
		t.Fatal("mark b must remain")
	}
	// The overlay stays open showing the remaining marks.
	if !app.marksOverlay.IsVisible() || app.mode != ModeMarks {
		t.Fatal("the overlay must stay open after a delete")
	}
	view := app.marksOverlay.View(80, 24)
	if !strings.Contains(view, "b") || strings.Contains(view, "No marks set") {
		t.Fatalf("overlay must show the remaining mark:\n%s", view)
	}
}

func TestMarksOverlay_EmptyState(t *testing.T) {
	app := jumpMarkTestApp(t)
	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})
	if !strings.Contains(app.marksOverlay.View(80, 24), "No marks set") {
		t.Fatalf("overlay with no marks must report that:\n%s", app.marksOverlay.View(80, 24))
	}
}

// The overlay renders exclusively from the preview snapshot stored at
// mark time: opening and rendering it must make zero channel lookups.
func TestMarksOverlay_RendersFromSnapshotWithoutServiceCalls(t *testing.T) {
	app := jumpMarkTestApp(t)
	lookups := 0
	app.setChannelLookupFuncForTest(func(channelID ids.ChannelID) (string, string, bool) {
		lookups++
		return string(channelID) + "-name", "channel", true
	})
	if err := app.marks.Set("T1", "a", Mark{
		Location:    Location{TeamID: "T1", ChannelID: "C2", MessageTS: "10.0"},
		Letter:      "a",
		ChannelName: "general",
		AuthorName:  "alice",
		Excerpt:     "hello world",
	}); err != nil {
		t.Fatalf("Set mark: %v", err)
	}

	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})
	view := app.marksOverlay.View(80, 24)

	if lookups != 0 {
		t.Fatalf("opening/rendering the overlay made %d channel lookups, want 0 (snapshot only)", lookups)
	}
	if !strings.Contains(view, "alice: hello world") {
		t.Fatalf("overlay did not render the stored snapshot:\n%s", view)
	}
}

func TestMarksOverlay_SuppressedWhenOptionOff(t *testing.T) {
	app := jumpMarkTestApp(t)
	app.SetShowJumpOverlay(false)

	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})
	if app.mode == ModeMarks || app.marksOverlay.IsVisible() {
		t.Fatal("the overlay must not open when show_jump_overlay is off")
	}
	if !app.pendingJumpMark {
		t.Fatal("with the overlay off, ' must arm the silent pending-key flow")
	}
}

// Overlay rows render shortcodes as glyphs and stay on one line.
// A raw excerpt spilled newlines down the box, breaking the
// one-row-per-mark layout; unresolved :shortcode: text is also just
// wrong next to every other surface in the app.
func TestMarksOverlay_PreviewResolvesEmojiAndCollapsesNewlines(t *testing.T) {
	if got := previewText(":pretzel: lunch :pretzel:"); strings.Contains(got, ":pretzel:") {
		t.Errorf("shortcodes not resolved: %q", got)
	}
	got := previewText("Hello team,\nthe demo moves\n\nto Friday.")
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("newlines survived into a row: %q", got)
	}
	if !strings.Contains(got, "Hello team, the demo moves") {
		t.Errorf("collapse mangled the text: %q", got)
	}
}

// Ctrl+C leaves ModeMarks through the global quit intercept, not
// through handleMarksMode. The list must close with the mode, or it
// stays drawn over the quit prompt and, once the prompt is cancelled,
// over a channel whose keys are live underneath.
func TestMarksOverlay_ClosesWhenQuitPromptInterrupts(t *testing.T) {
	app := jumpMarkTestApp(t)
	app.Update(keyPress('\''))
	if !app.marksOverlay.IsVisible() {
		t.Fatal("precondition: ' did not open the marks list")
	}

	app.Update(keyMod('c', tea.ModCtrl))
	if app.mode != ModeConfirm {
		t.Fatalf("mode = %v, want ModeConfirm", app.mode)
	}
	if app.marksOverlay.IsVisible() {
		t.Fatal("marks list still visible behind the quit prompt")
	}

	app.Update(keyCode(tea.KeyEscape))
	if app.mode != ModeNormal || app.marksOverlay.IsVisible() {
		t.Fatalf("after cancelling the prompt: mode=%v list visible=%v, want normal and closed", app.mode, app.marksOverlay.IsVisible())
	}
}

// marksClickApp opens the marks list over an 80x24 App with marks a
// (C2) and b (C3), and returns the box's top-left screen cell.
func marksClickApp(t *testing.T) (*App, int, int) {
	t.Helper()
	app := jumpMarkTestApp(t)
	app.width, app.height = 80, 24
	seedMark(t, app, "a", Location{TeamID: "T1", ChannelID: "C2", MessageTS: "10.0"})
	seedMark(t, app, "b", Location{TeamID: "T1", ChannelID: "C3", MessageTS: "20.0"})
	app.Update(tea.KeyPressMsg{Code: '\'', Text: "'"})
	if app.mode != ModeMarks {
		t.Fatalf("mode = %v, want ModeMarks", app.mode)
	}
	w, h := app.marksOverlay.BoxSize(app.width, app.height)
	return app, max((app.width-w)/2, 0), max((app.height-h)/2, 0)
}

func TestMarksOverlay_ClickRowJumps(t *testing.T) {
	app, x, y := marksClickApp(t)
	// The list starts at box-local row 4: border, padding, title, blank.
	_, cmd := app.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: y + 4 + 1})
	if cmd == nil {
		t.Fatal("clicking a row must jump to its mark")
	}
	cs, ok := cmd().(ChannelSelectedMsg)
	if !ok || cs.ID != "C3" {
		t.Fatalf("clicked the second row, got %T %+v, want a jump to C3", cmd(), cmd())
	}
	if app.mode != ModeNormal || app.marksOverlay.IsVisible() {
		t.Errorf("the jump must close the list: mode=%v visible=%v", app.mode, app.marksOverlay.IsVisible())
	}
}

func TestMarksOverlay_ClickInsideOffRowKeepsListOpen(t *testing.T) {
	app, x, y := marksClickApp(t)
	// Box-local row 2 is the title.
	if _, cmd := app.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: y + 2}); cmd != nil {
		t.Errorf("clicking the title returned a cmd: %v", cmd)
	}
	if app.mode != ModeMarks || !app.marksOverlay.IsVisible() {
		t.Errorf("the list must stay open: mode=%v visible=%v", app.mode, app.marksOverlay.IsVisible())
	}
}

func TestMarksOverlay_ClickOutsideCloses(t *testing.T) {
	app, _, _ := marksClickApp(t)
	if _, cmd := app.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0}); cmd != nil {
		if _, ok := cmd().(ChannelSelectedMsg); ok {
			t.Fatal("an outside click must not jump")
		}
	}
	if app.mode != ModeNormal || app.marksOverlay.IsVisible() {
		t.Errorf("an outside click must close the list: mode=%v visible=%v", app.mode, app.marksOverlay.IsVisible())
	}
}

// filterMarksApp opens the marks list over a (#general, C2), b (#random,
// C3) and c (#ranch, C1) and presses keys through App.Update, returning
// the last key's cmd.
func filterMarksApp(t *testing.T, keys ...tea.KeyMsg) (*App, tea.Cmd) {
	t.Helper()
	app := jumpMarkTestApp(t)
	for _, m := range []struct{ letter, ch, ts, name string }{
		{"a", "C2", "10.0", "general"}, {"b", "C3", "20.0", "random"}, {"c", "C1", "1.0", "ranch"},
	} {
		loc := Location{TeamID: "T1", ChannelID: ids.ChannelID(m.ch), MessageTS: ids.MessageTS(m.ts)}
		if err := app.marks.Set("T1", m.letter, Mark{Location: loc, Letter: m.letter, ChannelName: m.name}); err != nil {
			t.Fatalf("seed mark %s: %v", m.letter, err)
		}
	}
	app.Update(keyPress('\''))
	if app.mode != ModeMarks {
		t.Fatalf("precondition: mode = %v, want ModeMarks", app.mode)
	}
	var cmd tea.Cmd
	for _, k := range keys {
		_, cmd = app.Update(k)
	}
	return app, cmd
}

// runeKeys returns a key press for each rune of s.
func runeKeys(s string) []tea.KeyMsg {
	var out []tea.KeyMsg
	for _, r := range s {
		out = append(out, keyPress(r))
	}
	return out
}

// TestMarksOverlay_FilterTypesLettersAndQuotes: while the filter is
// typed, mark letters, ' and ` are filter text, not jumps or the
// back-jump.
func TestMarksOverlay_FilterTypesLettersAndQuotes(t *testing.T) {
	app, cmd := filterMarksApp(t, runeKeys("/ra'`")...)
	if app.mode != ModeMarks || !app.marksOverlay.IsVisible() || !app.marksOverlay.Typing() {
		t.Fatalf("mode=%v visible=%v typing=%v, want the list open and typing", app.mode, app.marksOverlay.IsVisible(), app.marksOverlay.Typing())
	}
	if cmd != nil {
		if _, ok := cmd().(ChannelSelectedMsg); ok {
			t.Fatal("a key typed into the filter jumped")
		}
	}
	if view := app.marksOverlay.View(80, 24); !strings.Contains(view, "ra'`█") {
		t.Errorf("the filter line does not show the typed query:\n%s", view)
	}
}

func TestMarksOverlay_FilterEnterSelectsFilteredRow(t *testing.T) {
	keys := append(runeKeys("/rand"), keyCode(tea.KeyEnter), keyCode(tea.KeyEnter))
	app, cmd := filterMarksApp(t, keys...)
	if cmd == nil {
		t.Fatal("enter on the kept filter must jump to the highlighted row")
	}
	if cs, ok := cmd().(ChannelSelectedMsg); !ok || cs.ID != "C3" {
		t.Fatalf("jumped with %T %+v, want C3 (#random, the only match)", cmd(), cmd())
	}
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want Normal after the jump", app.mode)
	}
}

// TestMarksOverlay_FilterKeptLettersJump: after Enter keeps the filter,
// a mark letter jumps again, even to a mark the filter hides.
func TestMarksOverlay_FilterKeptLettersJump(t *testing.T) {
	keys := append(runeKeys("/ran"), keyCode(tea.KeyEnter), keyPress('a'))
	_, cmd := filterMarksApp(t, keys...)
	if cmd == nil {
		t.Fatal("a mark letter with a kept filter must jump")
	}
	if cs, ok := cmd().(ChannelSelectedMsg); !ok || cs.ID != "C2" {
		t.Fatalf("jumped with %T %+v, want C2", cmd(), cmd())
	}
}

func TestMarksOverlay_FilterKeptBackJumpCloses(t *testing.T) {
	keys := append(runeKeys("/ran"), keyCode(tea.KeyEnter), keyPress('\''))
	app, _ := filterMarksApp(t, keys...)
	if app.mode != ModeNormal || app.marksOverlay.IsVisible() {
		t.Errorf("' with a kept filter must take the back-jump: mode=%v visible=%v", app.mode, app.marksOverlay.IsVisible())
	}
}

// TestMarksOverlay_FilterDeleteKeepsFilter: backspace on a kept filter
// deletes the highlighted filtered row, and the refreshed list is still
// filtered.
func TestMarksOverlay_FilterDeleteKeepsFilter(t *testing.T) {
	keys := append(runeKeys("/ran"), keyCode(tea.KeyEnter), keyCode(tea.KeyDown), keyCode(tea.KeyBackspace))
	app, _ := filterMarksApp(t, keys...)
	if _, ok, _ := app.marks.Load("T1", "c"); ok {
		t.Fatal("mark c (#ranch, the second filtered row) must be deleted")
	}
	for _, l := range []string{"a", "b"} {
		if _, ok, _ := app.marks.Load("T1", l); !ok {
			t.Fatalf("mark %s must remain", l)
		}
	}
	if app.mode != ModeMarks {
		t.Fatalf("mode = %v, want the list to stay open", app.mode)
	}
	view := app.marksOverlay.View(80, 24)
	if !strings.Contains(view, "filter: ran") || !strings.Contains(view, "#random") || strings.Contains(view, "#general") {
		t.Errorf("the refreshed list must keep the filter:\n%s", view)
	}
}

func TestMarksOverlay_FilterEscClearsThenCloses(t *testing.T) {
	app, _ := filterMarksApp(t, append(runeKeys("/ran"), keyCode(tea.KeyEscape))...)
	if view := app.marksOverlay.View(80, 24); app.mode != ModeMarks || app.marksOverlay.Typing() || strings.Contains(view, "ran█") || !strings.Contains(view, "#general") {
		t.Fatalf("esc while typing must clear the filter and keep the list: mode=%v", app.mode)
	}

	app, _ = filterMarksApp(t, append(runeKeys("/ran"), keyCode(tea.KeyEnter), keyCode(tea.KeyEscape))...)
	if app.mode != ModeMarks || strings.Contains(app.marksOverlay.View(80, 24), "filter:") {
		t.Fatalf("esc with a kept filter must clear it and keep the list: mode=%v", app.mode)
	}
	app.Update(keyCode(tea.KeyEscape))
	if app.mode != ModeNormal || app.marksOverlay.IsVisible() {
		t.Errorf("esc with no filter must close: mode=%v visible=%v", app.mode, app.marksOverlay.IsVisible())
	}
}
