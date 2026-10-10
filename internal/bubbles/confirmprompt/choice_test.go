package confirmprompt

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type pressedMsg struct{ n int }

func numKey(n string) key.Binding { return key.NewBinding(key.WithKeys(n)) }

// slackChoice mirrors Slackbot's "not in channel" buttons: number keys,
// nothing highlighted, other keys ignored.
func slackChoice() Choice {
	return Choice{
		Title: "Slackbot",
		Body:  "You mentioned @x, but they're not in this channel.",
		Buttons: []Button{
			{Label: "Add Them", Keys: numKey("1"), Action: func() tea.Msg { return pressedMsg{1} }},
			{Label: "Dismiss", Keys: numKey("2"), Action: func() tea.Msg { return pressedMsg{2} }},
			{Label: "Don't Show Again", Keys: numKey("3"), Action: func() tea.Msg { return pressedMsg{3} }},
		},
		Default: -1,
	}
}

func openChoice(t *testing.T, c Choice) Model {
	t.Helper()
	m := New(WithWidth(80))
	m.OpenChoice(c)
	return m
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	return cmd()
}

func TestChoiceShortcutPressesButton(t *testing.T) {
	t.Parallel()
	m, cmd := openChoice(t, slackChoice()).Update(keyPress('2'))
	if got := runCmd(t, cmd); got != (pressedMsg{2}) {
		t.Errorf("pressed %v, want button 2", got)
	}
	if m.IsVisible() {
		t.Error("prompt should close after a press")
	}
}

func TestChoiceEnterWithoutHighlightDoesNothing(t *testing.T) {
	t.Parallel()
	m, cmd := openChoice(t, slackChoice()).Update(keyCode(tea.KeyEnter))
	if cmd != nil || !m.IsVisible() {
		t.Errorf("Enter with nothing highlighted: cmd=%v visible=%v, want nil and open", cmd != nil, m.IsVisible())
	}
}

func TestChoiceMovementThenEnter(t *testing.T) {
	t.Parallel()
	m := openChoice(t, slackChoice())
	for _, k := range []tea.KeyPressMsg{keyPress('l'), keyCode(tea.KeyRight), keyCode(tea.KeyTab)} {
		m, _ = m.Update(k)
	}
	// l -> 0, right -> 1, tab -> 2
	if m.Highlighted() != 2 {
		t.Fatalf("Highlighted = %d, want 2", m.Highlighted())
	}
	m, _ = m.Update(keyCode(tea.KeyTab)) // wraps to 0
	m, _ = m.Update(keyPress('h'))       // wraps back to 2
	if m.Highlighted() != 2 {
		t.Fatalf("Highlighted = %d after wrap, want 2", m.Highlighted())
	}
	_, cmd := m.Update(keyCode(tea.KeyEnter))
	if got := runCmd(t, cmd); got != (pressedMsg{3}) {
		t.Errorf("Enter pressed %v, want button 3", got)
	}
}

func TestChoiceEscClosesWithoutPressing(t *testing.T) {
	t.Parallel()
	m, cmd := openChoice(t, slackChoice()).Update(keyCode(tea.KeyEscape))
	if cmd != nil || m.IsVisible() {
		t.Error("Esc should close without a command")
	}
}

func TestChoiceUnboundKey(t *testing.T) {
	t.Parallel()
	m, cmd := openChoice(t, slackChoice()).Update(keyPress('z'))
	if cmd != nil || !m.IsVisible() {
		t.Error("an unbound key should be ignored when CancelOnOtherKeys is false")
	}
	c := slackChoice()
	c.CancelOnOtherKeys = true
	m, cmd = openChoice(t, c).Update(keyPress('z'))
	if cmd != nil || m.IsVisible() {
		t.Error("an unbound key should close when CancelOnOtherKeys is true")
	}
}

func TestChoiceShortcutBeatsMovement(t *testing.T) {
	t.Parallel()
	c := slackChoice()
	c.Buttons[0].Keys = numKey("l")
	_, cmd := openChoice(t, c).Update(keyPress('l'))
	if got := runCmd(t, cmd); got != (pressedMsg{1}) {
		t.Errorf("l pressed %v, want button 1 (its shortcut)", got)
	}
}

func TestChoiceThenReplacesChoice(t *testing.T) {
	t.Parallel()
	confirm := Choice{
		Title:   "Are you sure?",
		Buttons: []Button{{Label: "Cancel", Keys: numKey("1")}, {Label: "Add", Keys: numKey("2"), Action: func() tea.Msg { return pressedMsg{9} }}},
		Default: -1,
	}
	c := slackChoice()
	c.Buttons[0].Action = nil
	c.Buttons[0].Then = &confirm
	confirm.Buttons[0].Then = &c

	m, cmd := openChoice(t, c).Update(keyPress('1'))
	if cmd != nil || !m.IsVisible() || !strings.Contains(m.View(), "Are you sure?") {
		t.Fatalf("Then should swap to the confirm step in place:\n%s", m.View())
	}
	m, _ = m.Update(keyPress('1')) // Cancel goes back
	if !strings.Contains(m.View(), "Add Them") {
		t.Fatalf("Cancel should return to the original choice:\n%s", m.View())
	}
	m, _ = m.Update(keyPress('1'))
	_, cmd = m.Update(keyPress('2'))
	if got := runCmd(t, cmd); got != (pressedMsg{9}) {
		t.Errorf("confirm OK pressed %v, want pressedMsg{9}", got)
	}
}

func TestChoiceNilActionJustCloses(t *testing.T) {
	t.Parallel()
	c := slackChoice()
	c.Buttons[1].Action = nil
	m, cmd := openChoice(t, c).Update(keyPress('2'))
	if cmd != nil || m.IsVisible() {
		t.Error("a button without Action should close with no command")
	}
}

func TestPressHighlighted(t *testing.T) {
	t.Parallel()
	m := openChoice(t, slackChoice())
	if _, cmd := m.PressHighlighted(); cmd != nil {
		t.Error("nothing highlighted: PressHighlighted should do nothing")
	}
	m.Highlight(1)
	_, cmd := m.PressHighlighted()
	if got := runCmd(t, cmd); got != (pressedMsg{2}) {
		t.Errorf("pressed %v, want button 2", got)
	}
}

func TestChoiceDefaultHighlights(t *testing.T) {
	t.Parallel()
	c := slackChoice()
	c.Default = 1
	if got := openChoice(t, c).Highlighted(); got != 1 {
		t.Errorf("Highlighted = %d, want Default 1", got)
	}
	c.Default = 7
	if got := openChoice(t, c).Highlighted(); got != -1 {
		t.Errorf("out-of-range Default: Highlighted = %d, want -1", got)
	}
}

func TestChoiceViewShowsButtons(t *testing.T) {
	t.Parallel()
	out := openChoice(t, slackChoice()).View()
	for _, want := range []string{"Slackbot", "[1] Add Them", "[2] Dismiss", "[3] Don't Show Again"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

func TestChoiceButtonWithoutKeysShowsBareLabel(t *testing.T) {
	t.Parallel()
	c := slackChoice()
	c.Buttons[2].Keys = key.Binding{}
	if out := openChoice(t, c).View(); !strings.Contains(out, "[Don't Show Again]") {
		t.Errorf("a button with no keys should render as [label]:\n%s", out)
	}
}

func TestPresetHighlightsConfirm(t *testing.T) {
	t.Parallel()
	if got := opened(t, nil).Highlighted(); got != 0 {
		t.Errorf("yes/no preset: Highlighted = %d, want 0 (Confirm)", got)
	}
}
