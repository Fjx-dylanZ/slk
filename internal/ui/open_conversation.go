// internal/ui/open_conversation.go
//
// Opening a DM or group DM by its user IDs, shared by the new-message
// picker and /msg.
package ui

import tea "charm.land/bubbletea/v2"

// msgPrefill is what /msg asks of an open: text is offered as the
// conversation's draft once it opens, and origin is where /msg was typed,
// so a failed open can give the command back. The zero value, which the
// new-message picker passes, asks for neither.
type msgPrefill struct {
	text   string
	origin commandOrigin
}

// openConversation opens (or finds) the DM or group DM with userIDs: a
// fresh in-flight request, whose NewMessageOpenedMsg switches to the
// conversation (reducer_new_message.go) and drops late results of
// cancelled ones. Shared by the new-message picker and /msg.
func (a *App) openConversation(userIDs []string, prefill msgPrefill) tea.Cmd {
	a.newMessageInFlightID++
	a.newMessageCancelled = false
	a.newMessagePrefill = prefill
	return teaCmd(a.channels.OpenConversation(userIDs, a.newMessageInFlightID))
}
