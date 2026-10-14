// internal/ui/mode_reactions_view.go
//
// Reactions-view key handler. Forwards normalised keys to the
// reactionsview overlay (esc/q/L close, up/down scroll, / filters),
// then drops back to Normal mode when the overlay reports itself
// invisible. enter and backspace are normalised too: the filter uses
// them while it is typed.
package ui

import (
	tea "charm.land/bubbletea/v2"
)

func handleReactionsViewMode(a *App, msg tea.KeyMsg) tea.Cmd {
	a.reactionsView.HandleKey(normalizeFinderKey(msg))
	if !a.reactionsView.IsVisible() {
		a.SetMode(ModeNormal)
	}
	return nil
}
