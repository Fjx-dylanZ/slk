// internal/ui/reducer_keyhints.go
//
// The key-hints menu's delay (keyhints.go): the tick of the latest key
// opens the menu, if a key is still pending and Normal mode still has
// the keyboard. Any other tick is stale.
package ui

import tea "charm.land/bubbletea/v2"

var reduceKeyHints reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	tick, ok := msg.(keyHintsTickMsg)
	if !ok {
		return nil, false
	}
	if tick.seq == a.keyHintsSeq && a.mode == ModeNormal {
		if src := a.pendingHintSource(); src != nil {
			a.showKeyHints(src)
		}
	}
	return nil, true
}
