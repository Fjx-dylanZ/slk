// internal/ui/action_prompt.go
//
// The action dialog: the confirm prompt showing an ephemeral message's
// legacy buttons. Building the choice and pressing live here; the reducer
// arm for press results is in reducer_action_prompt.go.
package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/bubbles/confirmprompt"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// actionPromptRef identifies an ephemeral message with buttons.
type actionPromptRef struct {
	channelID, ts, threadTS string
}

func (r actionPromptRef) key() string { return r.channelID + "/" + r.ts }

// actionPressedMsg reports a button press; err is nil on success, when
// Slack answers by deleting the ephemeral over the WebSocket.
type actionPressedMsg struct {
	label string
	err   error
}

// findActionMessage looks ref's message up in the pane showing it: the
// open thread for a reply, otherwise any window on the channel.
func (a *App) findActionMessage(ref actionPromptRef) (messages.MessageItem, bool) {
	if ref.threadTS != "" && ref.threadTS != ref.ts {
		if a.threadVisible && a.threadPanel.ChannelID() == ref.channelID && a.threadPanel.ThreadTS() == ref.threadTS {
			for _, r := range a.threadPanel.Replies() {
				if r.TS == ref.ts {
					return r, true
				}
			}
		}
		return messages.MessageItem{}, false
	}
	for _, mm := range a.modelsForChannel(ref.channelID) {
		for _, m := range mm.Messages() {
			if m.TS == ref.ts {
				return m, true
			}
		}
	}
	return messages.MessageItem{}, false
}

// openActionPrompt opens the dialog for ref's buttons. False when the
// message is no longer shown or has no buttons.
func (a *App) openActionPrompt(ref actionPromptRef) bool {
	msg, ok := a.findActionMessage(ref)
	if !ok || !messages.HasButtonActions(msg) {
		return false
	}
	if a.mode == ModeInsert {
		a.compose.Blur()
		a.threadCompose.Blur()
	}
	a.prepareConfirmPrompt()
	a.confirmPrompt.OpenChoice(a.actionChoice(ref, msg))
	a.shownPrompt = &ref
	a.SetMode(ModeConfirm)
	return true
}

// actionChoice is the dialog for msg: its sender, its text, one button per
// legacy button with number keys 1-9, nothing highlighted, and unbound keys
// ignored.
func (a *App) actionChoice(ref actionPromptRef, msg messages.MessageItem) confirmprompt.Choice {
	choice := &confirmprompt.Choice{Title: msg.UserName, Body: a.actionBody(msg), Default: -1}
	for i, ba := range messages.ButtonActions(msg) {
		b := confirmprompt.Button{Label: ba.Action.Text}
		if i < 9 {
			b.Keys = key.NewBinding(key.WithKeys(strconv.Itoa(i + 1)))
		}
		switch {
		case ba.Action.URL != "":
			url := ba.Action.URL
			b.Action = func() tea.Msg { return OpenLinkMsg{URL: url} }
		case ba.Action.Confirm != nil:
			b.Then = a.actionConfirmChoice(ref, ba, choice)
		default:
			b.Action = a.pressAction(ref, ba)
		}
		choice.Buttons = append(choice.Buttons, b)
	}
	return *choice
}

// actionConfirmChoice is Slack's confirmation step for ba. Its dismiss
// button returns to back; its OK button presses.
func (a *App) actionConfirmChoice(ref actionPromptRef, ba messages.ButtonAction, back *confirmprompt.Choice) *confirmprompt.Choice {
	c := ba.Action.Confirm
	title, dismiss, ok := c.Title, c.DismissText, c.OKText
	if title == "" {
		title = "Are you sure?"
	}
	if dismiss == "" {
		dismiss = "Cancel"
	}
	if ok == "" {
		ok = "Okay"
	}
	return &confirmprompt.Choice{
		Title:   title,
		Body:    c.Text,
		Default: -1,
		Buttons: []confirmprompt.Button{
			{Label: dismiss, Keys: key.NewBinding(key.WithKeys("1")), Then: back},
			{Label: ok, Keys: key.NewBinding(key.WithKeys("2")), Action: a.pressAction(ref, ba)},
		},
	}
}

// actionBody is the message text, then each attachment's pretext and text,
// flattened to plain text.
func (a *App) actionBody(msg messages.MessageItem) string {
	user := func(id string) (string, bool) {
		n, ok := a.userNames[id]
		return n, ok && n != ""
	}
	parts := []string{messages.FlattenMrkdwn(messages.MessageTextSource(msg), user, nil)}
	for _, att := range msg.LegacyAttachments {
		for _, s := range []string{att.Pretext, att.Text} {
			if s != "" {
				parts = append(parts, messages.FlattenMrkdwn(s, user, nil))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// pressAction is a button's action: press it through the interaction
// service, off the UI goroutine, and report the result.
func (a *App) pressAction(ref actionPromptRef, ba messages.ButtonAction) func() tea.Msg {
	svc := a.interactions
	att, act := ba.Attachment, ba.Action
	return func() tea.Msg {
		err := svc.PressAttachmentAction(ids.ChannelID(ref.channelID), ids.MessageTS(ref.ts),
			att.ID, att.CallbackID, act, true)
		return actionPressedMsg{label: act.Text, err: err}
	}
}
