package ui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/bubbles/vimedit"
	"github.com/gammons/slk/internal/ui/help"
	"github.com/gammons/slk/internal/ui/mentionpicker"
)

// App-level behaviour of [compose] vim_mode, driven through App.Update.

// The help overlay lists the composer's vim keys only with vim_mode on.
func TestComposeVim_HelpListsVimKeysOnlyWhenOn(t *testing.T) {
	vimEntries := func(a *App) []string {
		var out []string
		for _, e := range help.FromKeyMap(a.keys) {
			if strings.HasPrefix(e.Desc, "compose vim:") {
				out = append(out, e.Key)
			}
		}
		return out
	}
	a := newTestApp(t)
	if got := vimEntries(a); len(got) != 0 {
		t.Errorf("vim_mode off: help lists %v", got)
	}
	a.SetComposeVimMode(true)
	if got := vimEntries(a); len(got) != 3 {
		t.Errorf("vim_mode on: help lists %v, want the three vim entries", got)
	}
	a.SetComposeVimMode(false)
	if got := vimEntries(a); len(got) != 0 {
		t.Errorf("vim_mode turned off again: help lists %v", got)
	}
}

// vimKeys sends keys through App.Update and returns the last key's
// command. Each character is a key; <esc> <enter> <s-enter> <up> <down>
// <c-o> <c-u> name the others.
func vimKeys(a *App, keys string) tea.Cmd {
	special := map[string]tea.KeyMsg{
		"<esc>": keyCode(tea.KeyEscape), "<enter>": keyCode(tea.KeyEnter),
		"<a-enter>": keyMod(tea.KeyEnter, tea.ModAlt), "<s-enter>": keyMod(tea.KeyEnter, tea.ModShift),
		"<c-j>": keyMod('j', tea.ModCtrl), "<up>": keyCode(tea.KeyUp), "<down>": keyCode(tea.KeyDown),
		"<c-o>": keyMod('o', tea.ModCtrl), "<c-u>": keyMod('u', tea.ModCtrl),
	}
	var cmd tea.Cmd
	for keys != "" {
		k, n := tea.KeyMsg(nil), 0
		for name, km := range special {
			if strings.HasPrefix(keys, name) {
				k, n = km, len(name)
				break
			}
		}
		if k == nil {
			r := []rune(keys)[0]
			k, n = keyPress(r), len(string(r))
		}
		_, cmd = a.Update(k)
		keys = keys[n:]
	}
	return cmd
}

// vimApp is an App on C1 in Normal mode with vim_mode on.
func vimApp(t *testing.T, opts ...testOpt) *App {
	t.Helper()
	a := newTestApp(t, append(insertOpts(), opts...)...)
	a.SetComposeVimMode(true)
	return a
}

func modeLabel(a *App) string {
	text := statusbarText(a)
	for _, l := range []string{"C-NORMAL", "V-LINE", "VISUAL", "INSERT", "NORMAL"} {
		if strings.Contains(text, " "+l+" ") {
			return l
		}
	}
	return "?"
}

func composeVimMode(t *testing.T, c interface {
	VimMode() (vimedit.Mode, bool)
}) vimedit.Mode {
	t.Helper()
	m, ok := c.VimMode()
	if !ok {
		t.Fatal("vim mode is off")
	}
	return m
}

func TestComposeVimOff_Unchanged(t *testing.T) {
	a := newTestApp(t, insertOpts()...)
	vimKeys(a, "ihi")
	if a.mode != ModeInsert || modeLabel(a) != "INSERT" {
		t.Fatalf("i: mode %v label %s", a.mode, modeLabel(a))
	}
	vimKeys(a, "<esc>")
	if a.mode != ModeNormal || modeLabel(a) != "NORMAL" {
		t.Errorf("Esc with vim_mode off: mode %v label %s, want app Normal", a.mode, modeLabel(a))
	}
	if a.compose.Value() != "hi" {
		t.Errorf("draft = %q", a.compose.Value())
	}
}

func TestComposeVim_EscInsertNormalThenLeave(t *testing.T) {
	a := vimApp(t)
	vimKeys(a, "i")
	if a.mode != ModeInsert || modeLabel(a) != "INSERT" || composeVimMode(t, &a.compose) != vimedit.Insert {
		t.Fatalf("i lands in vim INSERT: mode %v label %s", a.mode, modeLabel(a))
	}
	vimKeys(a, "hello world<esc>")
	if a.mode != ModeInsert {
		t.Fatalf("first Esc left the composer: mode %v", a.mode)
	}
	if composeVimMode(t, &a.compose) != vimedit.Normal || modeLabel(a) != "C-NORMAL" {
		t.Fatalf("first Esc: label %s", modeLabel(a))
	}
	vimKeys(a, "bdw")
	if got := a.compose.Value(); got != "hello " {
		t.Errorf("bdw in the composer = %q", got)
	}
	vimKeys(a, "vh")
	if modeLabel(a) != "VISUAL" {
		t.Errorf("v: label %s", modeLabel(a))
	}
	vimKeys(a, "<esc>V")
	if modeLabel(a) != "V-LINE" {
		t.Errorf("V: label %s", modeLabel(a))
	}
	vimKeys(a, "<esc><esc>")
	if a.mode != ModeNormal || modeLabel(a) != "NORMAL" {
		t.Errorf("Esc from idle compose NORMAL: mode %v label %s, want app Normal", a.mode, modeLabel(a))
	}
	if a.compose.Value() != "hello " {
		t.Errorf("leaving kept the draft? %q", a.compose.Value())
	}
	vimKeys(a, "i")
	if composeVimMode(t, &a.compose) != vimedit.Insert || modeLabel(a) != "INSERT" {
		t.Errorf("i again: vim %v label %s, want INSERT", composeVimMode(t, &a.compose), modeLabel(a))
	}
}

// The composer owns the keyboard in compose NORMAL: keys that mean
// something in app Normal mode must not reach it.
func TestComposeVim_AppNormalKeysDoNotFire(t *testing.T) {
	a := vimApp(t)
	vimKeys(a, "iabc<esc>")
	selected, _ := a.messagepane.SelectedMessage()
	vimKeys(a, "q:/jkKZ")
	if a.mode != ModeInsert {
		t.Fatalf("app keys fired from compose NORMAL: mode %v", a.mode)
	}
	if a.confirmPrompt.IsVisible() {
		t.Error("q opened the quit prompt")
	}
	if now, _ := a.messagepane.SelectedMessage(); now.TS != selected.TS {
		t.Error("j/k moved the message selection")
	}
	if a.compose.Value() != "abc" {
		t.Errorf("draft = %q", a.compose.Value())
	}
}

func TestComposeVim_EnterSendsFromNormal(t *testing.T) {
	a := vimApp(t)
	vimKeys(a, "ihi there<esc>0")
	cmd := vimKeys(a, "<enter>")
	sends := deliveredMsgs[SendMessageMsg](t, cmd)
	if len(sends) != 1 || sends[0].Text != "hi there" || sends[0].ChannelID != "C1" {
		t.Fatalf("Enter from compose NORMAL sent %+v", sends)
	}
	if a.mode != ModeNormal || a.compose.Value() != "" {
		t.Errorf("after send: mode %v draft %q", a.mode, a.compose.Value())
	}
}

func TestComposeVim_EnterCancelsAPendingCommandOrSelection(t *testing.T) {
	a := vimApp(t)
	vimKeys(a, "iabc<esc>")
	for _, pending := range []string{"d", "2", "f", "v", "V"} {
		cmd := vimKeys(a, pending+"<enter>")
		if cmd != nil {
			t.Fatalf("%s<enter> returned a command (a send?): %#v", pending, cmd())
		}
		if a.compose.VimBusy() || modeLabel(a) != "C-NORMAL" || a.mode != ModeInsert {
			t.Errorf("%s<enter>: busy %v label %s mode %v", pending, a.compose.VimBusy(), modeLabel(a), a.mode)
		}
	}
	if a.compose.Value() != "abc" {
		t.Errorf("draft = %q", a.compose.Value())
	}
	// Newline keys type nothing in NORMAL.
	vimKeys(a, "<s-enter><c-j>")
	if a.compose.Value() != "abc" {
		t.Errorf("newline keys in NORMAL changed the draft: %q", a.compose.Value())
	}
}

func TestComposeVim_ArrowsAreMotionsInNormal(t *testing.T) {
	a := vimApp(t)
	vimKeys(a, "ione<s-enter>two<esc>")
	vimKeys(a, "<up>")
	if !a.compose.CursorAtFirstLine() {
		t.Error("Up in NORMAL should move a line up")
	}
	vimKeys(a, "<up>")
	vimKeys(a, "x")
	if got := a.compose.Value(); got != "on\ntwo" {
		t.Errorf("Up on the first line must not jump to the start in NORMAL: %q", got)
	}
}

// A typing event is dispatched synchronously into the throttle (the
// HTTP call runs on a goroutine), so CanSend going false is the proof
// that one was sent, and staying true that none was.
func TestComposeVim_TypingIndicatorOnlyWhileTyping(t *testing.T) {
	a := vimApp(t)
	a.SetTypingEnabled(true)
	wireTypingSender(a, func(string) {})
	a.SetMode(ModeInsert)
	_ = a.compose.Focus()
	vimKeys(a, "<esc>hhlwbx")
	if !a.typingOut.CanSend() {
		t.Fatal("vim commands sent a typing event")
	}
	vimKeys(a, "ix")
	if a.typingOut.CanSend() {
		t.Fatal("typing in INSERT sent no typing event")
	}
}

// VISUAL can end without a key reaching handleInsertMode: the text is
// replaced from outside. The status bar must follow.
func TestComposeVim_LabelFollowsAVisualEndedFromOutside(t *testing.T) {
	inVisual := func(t *testing.T) *App {
		t.Helper()
		a := vimApp(t)
		vimKeys(a, "iabc<esc>v")
		if modeLabel(a) != "VISUAL" {
			t.Fatalf("precondition: label %s", modeLabel(a))
		}
		return a
	}
	t.Run("bracketed paste", func(t *testing.T) {
		a := inVisual(t)
		a.Update(tea.PasteMsg{Content: "XY"})
		if a.compose.Value() != "abXYc" || modeLabel(a) != "C-NORMAL" {
			t.Errorf("after paste: draft %q label %s", a.compose.Value(), modeLabel(a))
		}
	})
	t.Run("$EDITOR returns", func(t *testing.T) {
		a := inVisual(t)
		a.setEditorForTest()
		f, err := os.CreateTemp(t.TempDir(), "slk-compose-*.md")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("edited\n"); err != nil {
			t.Fatal(err)
		}
		f.Close()
		a.Update(EditorFinishedMsg{Panel: PanelMessages, Path: f.Name()})
		if a.compose.Value() != "edited" || modeLabel(a) != "C-NORMAL" {
			t.Errorf("after the editor: draft %q label %s", a.compose.Value(), modeLabel(a))
		}
	})
	t.Run("draft switch", func(t *testing.T) {
		a := inVisual(t)
		a.Update(ChannelSelectedMsg{ID: "C2", Name: "random", Type: "channel"})
		if a.activeChannelID != "C2" || modeLabel(a) != "C-NORMAL" {
			t.Errorf("after switching to C2: channel %s label %s", a.activeChannelID, modeLabel(a))
		}
	})
}

func TestComposeVim_PickerEscClosesThePickerFirst(t *testing.T) {
	a := vimApp(t)
	a.compose.SetUsers([]mentionpicker.User{{ID: "U9", DisplayName: "Zed", Username: "zed"}})
	vimKeys(a, "ihi @")
	if !a.compose.IsMentionActive() {
		t.Fatal("precondition: @ opens the mention picker")
	}
	vimKeys(a, "<esc>")
	if a.compose.IsMentionActive() {
		t.Fatal("Esc should close the picker")
	}
	if composeVimMode(t, &a.compose) != vimedit.Insert || modeLabel(a) != "INSERT" {
		t.Errorf("closing the picker left INSERT: label %s", modeLabel(a))
	}
	vimKeys(a, "<esc>")
	if modeLabel(a) != "C-NORMAL" {
		t.Errorf("second Esc: label %s", modeLabel(a))
	}
}

func TestComposeVim_ThreadComposer(t *testing.T) {
	a := vimApp(t)
	showThread(t, a)
	a.SetMode(ModeInsert)
	vimKeys(a, "a reply<esc>")
	if composeVimMode(t, &a.threadCompose) != vimedit.Normal || modeLabel(a) != "C-NORMAL" {
		t.Fatalf("thread Esc: label %s", modeLabel(a))
	}
	if a.compose.Value() != "" {
		t.Error("the channel composer was touched")
	}
	vimKeys(a, "0cwthe<esc><c-o>")
	if !a.threadCompose.Broadcast() {
		t.Error("ctrl+o still toggles broadcast in compose NORMAL")
	}
	cmd := vimKeys(a, "<enter>")
	replies := deliveredMsgs[SendThreadReplyMsg](t, cmd)
	if len(replies) != 1 || replies[0].Text != "the reply" || !replies[0].Broadcast || replies[0].ThreadTS != "10.0" {
		t.Fatalf("thread send = %+v", replies)
	}
}

func TestComposeVim_RegisterSharedAcrossComposers(t *testing.T) {
	a := vimApp(t)
	vimKeys(a, "ishared<esc>0yiw<esc>")
	showThread(t, a)
	a.SetMode(ModeInsert)
	vimKeys(a, "<esc>p")
	if got := a.threadCompose.Value(); got != "shared" {
		t.Errorf("put in the thread composer = %q", got)
	}
}

func TestComposeVim_EditMessage(t *testing.T) {
	newEditApp := func(t *testing.T) *App {
		t.Helper()
		a := newTestApp(t, normalOpts()...)
		a.SetComposeVimMode(true)
		focusMessages(t, a)
		a.currentUserID = "U1"
		a.compose.SetValue("my draft")
		vimKeys(a, "E")
		if !a.editing.IsActive() || a.mode != ModeInsert {
			t.Fatalf("E did not begin an edit: mode %v", a.mode)
		}
		if composeVimMode(t, &a.compose) != vimedit.Insert || modeLabel(a) != "INSERT" {
			t.Fatalf("E lands in INSERT: label %s", modeLabel(a))
		}
		return a
	}

	t.Run("Esc to NORMAL, edit, Enter commits", func(t *testing.T) {
		a := newEditApp(t)
		original := a.compose.Value()
		vimKeys(a, "<esc>")
		if !a.editing.IsActive() || modeLabel(a) != "C-NORMAL" {
			t.Fatalf("Esc in an edit's INSERT: editing %v label %s", a.editing.IsActive(), modeLabel(a))
		}
		vimKeys(a, "0x")
		cmd := vimKeys(a, "<enter>")
		edits := deliveredMsgs[EditMessageMsg](t, cmd)
		if len(edits) != 1 || edits[0].NewText != original[1:] {
			t.Fatalf("Enter from NORMAL committed %+v, want %q", edits, original[1:])
		}
	})

	t.Run("Esc Esc cancels and restores the draft", func(t *testing.T) {
		a := newEditApp(t)
		vimKeys(a, "<esc>dd<esc>")
		if a.editing.IsActive() || a.mode != ModeNormal {
			t.Fatalf("second Esc should cancel the edit: editing %v mode %v", a.editing.IsActive(), a.mode)
		}
		if got := a.compose.Value(); got != "my draft" {
			t.Errorf("draft after cancel = %q", got)
		}
		vimKeys(a, "i<esc>u")
		if got := a.compose.Value(); got != "my draft" {
			t.Errorf("u after a cancelled edit brought back %q", got)
		}
	})
}

func TestComposeVim_CtrlUClearsInNormal(t *testing.T) {
	a := vimApp(t)
	vimKeys(a, "iabc<esc><c-u>")
	if a.compose.Value() != "" || a.mode != ModeInsert || modeLabel(a) != "C-NORMAL" {
		t.Errorf("ctrl+u in NORMAL: draft %q mode %v label %s", a.compose.Value(), a.mode, modeLabel(a))
	}
}
