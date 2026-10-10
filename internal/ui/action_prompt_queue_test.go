package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// arrive delivers an ephemeral over the real Update path and returns the
// messages its commands produced.
func arrive(t *testing.T, a *App, msg messages.MessageItem) []tea.Msg {
	t.Helper()
	_, cmd := a.Update(NewMessageMsg{ChannelID: "C1", Message: msg})
	return drainBatch(cmd)
}

func dialogShows(a *App, text string) bool {
	return a.confirmPrompt.IsVisible() && strings.Contains(a.confirmPrompt.View(), text)
}

func TestEphemeralWithButtonsOpensDialog(t *testing.T) {
	a, _ := actionApp(t, nil)
	arrive(t, a, mentionEphemeral(ephTS))
	if a.mode != ModeConfirm || !dialogShows(a, "[2] Dismiss") {
		t.Errorf("mode = %v; the dialog should open for an ephemeral with buttons", a.mode)
	}
}

func TestEphemeralWithoutButtonsDoesNotOpen(t *testing.T) {
	a, _ := actionApp(t, nil)
	msg := mentionEphemeral(ephTS)
	msg.LegacyAttachments = nil
	arrive(t, a, msg)
	if a.confirmPrompt.IsVisible() {
		t.Error("no buttons, no dialog")
	}
}

func TestTypingDefersDialogWithOneToast(t *testing.T) {
	a, _ := actionApp(t, nil)
	a.SetMode(ModeInsert)
	a.compose.SetValue("half a thought")

	if got := toastText(arrive(t, a, mentionEphemeral(ephTS))); got != "Slackbot: press b to respond" {
		t.Errorf("toast = %q", got)
	}
	if a.mode != ModeInsert || a.confirmPrompt.IsVisible() {
		t.Fatal("the dialog must not interrupt typing")
	}
	if got := toastText(drainBatch(a.tryOpenActionPrompt())); got != "" {
		t.Errorf("the waiting toast repeated: %q", got)
	}

	pressKey(t, a, keyCode(tea.KeyEscape)) // leave insert mode
	if !dialogShows(a, "[2] Dismiss") {
		t.Error("leaving insert mode should open the waiting dialog")
	}
	if a.compose.Value() != "half a thought" {
		t.Error("the draft must survive")
	}
}

func TestEmptyComposerInInsertOpensDialog(t *testing.T) {
	a, _ := actionApp(t, nil)
	a.SetMode(ModeInsert)
	arrive(t, a, mentionEphemeral(ephTS))
	if !dialogShows(a, "[2] Dismiss") {
		t.Error("an empty composer is not typing: the dialog should open")
	}
}

// Review Focus 2: another modal is up -> wait, then open when it closes.
func TestDialogWaitsForOtherModal(t *testing.T) {
	a, _ := actionApp(t, nil)
	a.openQuitConfirm()
	arrive(t, a, mentionEphemeral(ephTS))
	if !dialogShows(a, "Quit slk?") {
		t.Fatal("the quit prompt must stay on top")
	}
	pressKey(t, a, keyPress('n'))
	if !dialogShows(a, "[2] Dismiss") {
		t.Error("the action dialog should open once the quit prompt closes")
	}
}

func TestQueueOpensInOrder(t *testing.T) {
	a, _ := actionApp(t, nil)
	first, second := mentionEphemeral("5.000000"), mentionEphemeral("6.000000")
	second.UserName = "Giphy"
	arrive(t, a, first)
	arrive(t, a, second)
	if !dialogShows(a, "Slackbot") {
		t.Fatal("the first ephemeral should be showing")
	}
	pressKey(t, a, keyCode(tea.KeyEscape))
	if !dialogShows(a, "Giphy") {
		t.Error("closing the first should open the second")
	}
}

// Review Focus 3: a queued message no longer in any pane is skipped.
func TestQueueSkipsMessageNoLongerShown(t *testing.T) {
	a, _ := actionApp(t, nil)
	a.openQuitConfirm()
	gone, kept := mentionEphemeral("5.000000"), mentionEphemeral("6.000000")
	kept.UserName = "Giphy"
	arrive(t, a, gone)
	arrive(t, a, kept)
	a.messagepane.RemoveMessageByTS("5.000000") // e.g. the user switched away and back
	pressKey(t, a, keyPress('n'))
	if !dialogShows(a, "Giphy") {
		t.Error("the missing message should be skipped and the next one opened")
	}
}

// Review Focus 4: deleting the shown message closes its dialog, and no
// later key presses a button of a message that no longer exists.
func TestDeleteClosesItsDialog(t *testing.T) {
	a, calls := actionApp(t, nil)
	arrive(t, a, mentionEphemeral(ephTS))
	_, _ = a.Update(WSMessageDeletedMsg{ChannelID: "C1", TS: ephTS})
	if a.confirmPrompt.IsVisible() || a.mode != ModeNormal {
		t.Fatal("deleting the message should close its dialog")
	}
	pressKey(t, a, keyPress('2'))
	if len(*calls) != 0 {
		t.Error("a key after the delete pressed a button")
	}
}

func TestDeleteDropsQueuedMessage(t *testing.T) {
	a, _ := actionApp(t, nil)
	a.openQuitConfirm()
	arrive(t, a, mentionEphemeral(ephTS))
	_, _ = a.Update(WSMessageDeletedMsg{ChannelID: "C1", TS: ephTS})
	pressKey(t, a, keyPress('n'))
	if a.confirmPrompt.IsVisible() {
		t.Error("a deleted message must not open once the quit prompt closes")
	}
}

func TestDeleteOfOtherMessageLeavesQuitPrompt(t *testing.T) {
	a, _ := actionApp(t, nil)
	a.openQuitConfirm()
	_, _ = a.Update(WSMessageDeletedMsg{ChannelID: "C1", TS: "9.000000"})
	if !dialogShows(a, "Quit slk?") {
		t.Error("an unrelated delete must not close the quit prompt")
	}
}

func TestEphemeralReplyInOpenThreadOpensDialog(t *testing.T) {
	a, _ := actionApp(t, nil)
	openThreadPanel(a, "C1", "1.000000")
	reply := mentionEphemeral(ephTS)
	reply.ThreadTS = "1.000000"
	arrive(t, a, reply)
	if !dialogShows(a, "[2] Dismiss") {
		t.Error("an ephemeral reply in the open thread should open its dialog")
	}
}
