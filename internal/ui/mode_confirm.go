// internal/ui/mode_confirm.go
//
// Confirm-mode key handler. The prompt closes itself on any key, so mode drops
// back once it reports hidden.
package ui

import (
	tea "charm.land/bubbletea/v2"
)

func handleConfirmMode(a *App, msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	a.confirmPrompt, cmd = a.confirmPrompt.Update(msg)
	return a.afterConfirmPrompt(cmd)
}

// afterConfirmPrompt runs after every key or click the confirm prompt
// handles: once the prompt reports hidden, mode drops back to normal.
func (a *App) afterConfirmPrompt(cmd tea.Cmd) tea.Cmd {
	if !a.confirmPrompt.IsVisible() {
		a.shownPrompt = nil
		a.SetMode(ModeNormal)
		return tea.Batch(cmd, a.tryOpenActionPrompt())
	}
	return cmd
}
