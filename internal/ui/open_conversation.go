// internal/ui/open_conversation.go
//
// Opening a DM or group DM by its user IDs, shared by the new-message
// picker and /msg.
package ui

import tea "charm.land/bubbletea/v2"

// openConversation opens (or finds) the DM or group DM with userIDs: a
// fresh in-flight request, whose NewMessageOpenedMsg switches to the
// conversation (reducer_new_message.go) and drops late results of
// cancelled ones. prefill, when not empty, is offered as the
// conversation's draft once it opens. Shared by the new-message picker
// and /msg.
func (a *App) openConversation(userIDs []string, prefill string) tea.Cmd {
	a.newMessageInFlightID++
	a.newMessageCancelled = false
	a.newMessagePrefill = prefill
	return teaCmd(a.channels.OpenConversation(userIDs, a.newMessageInFlightID))
}
