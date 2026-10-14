// internal/ui/keyactions.go
//
// Normal-mode actions whose keys [keys] can set (leader-keys design §8).
// Each is scoped to a panel: while that panel has focus its binding wins
// over the global bindings on the same key (sidebar_open = ["enter",
// "l"] makes l open in the sidebar and still move focus elsewhere);
// with focus anywhere else the action does not exist. A key sequence
// still comes first: the leader and ctrl+w keep their jobs, and
// <leader><leader> hands a displaced key back to its action here.
package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// keyAction is one configurable action: its [keys] name, its KeyMap
// binding, the panel it is scoped to and what it does.
type keyAction struct {
	name    string
	binding func(*KeyMap) *key.Binding
	scope   Panel
	run     func(*App) tea.Cmd
}

// keyActions is the table of configurable actions. Adding one: a row
// here, its binding in KeyMap, and its *[]string field in config.Keys
// (cmd/slk tests that the two lists agree).
var keyActions = []keyAction{
	{
		name:    "sidebar_open",
		binding: func(km *KeyMap) *key.Binding { return &km.SidebarOpen },
		scope:   PanelSidebar,
		run:     (*App).openSidebarRow,
	},
	{
		name:    "sidebar_toggle_section",
		binding: func(km *KeyMap) *key.Binding { return &km.ToggleSection },
		scope:   PanelSidebar,
		run: func(a *App) tea.Cmd {
			a.sidebar.ToggleCollapseSelected()
			return nil
		},
	},
}

// KeyActionNames lists the [keys] names SetKeyBindings knows.
func KeyActionNames() []string {
	names := make([]string, len(keyActions))
	for i, act := range keyActions {
		names[i] = act.name
	}
	return names
}

// SetKeyBindings rebinds configurable actions by [keys] name. Each key
// list replaces the action's keys, spelled as slk matches keys and
// validated by the config layer; an empty list unbinds the action and
// drops its help row. An action missing from b keeps its keys, and a
// name not in KeyActionNames is ignored.
func (a *App) SetKeyBindings(b map[string][]string) {
	defaults := DefaultKeyMap()
	for _, act := range keyActions {
		keys, ok := b[act.name]
		if !ok {
			continue
		}
		bp := act.binding(&a.keys)
		*bp = key.Binding{}
		if len(keys) > 0 {
			desc := act.binding(&defaults).Help().Desc
			*bp = key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(keys, "/"), desc))
		}
	}
	a.sequences = a.newKeySequences()
	a.pendingSeq.Reset()
}

// handleScopedKey runs the configurable action scoped to the focused
// panel whose binding matches msg. ok is false when none does, and the
// key goes on to the global bindings.
func (a *App) handleScopedKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	for _, act := range keyActions {
		if act.scope == a.focusedPanel && key.Matches(msg, *act.binding(&a.keys)) {
			return act.run(a), true
		}
	}
	return nil, false
}
