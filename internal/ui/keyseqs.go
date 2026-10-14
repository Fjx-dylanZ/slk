// internal/ui/keyseqs.go
//
// Normal mode's multi-key sequences (leader-keys design): the ctrl+w
// window commands and the leader layer, as keyseq tables, plus the
// dispatch and which-key hint around them. One keyseq.Sequence
// (a.pendingSeq) is the pending state for every table here.
package ui

import (
	"reflect"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/bubbles/keyseq"
	"github.com/gammons/slk/internal/ui/help"
	"github.com/gammons/slk/internal/ui/wintree"
)

// run adapts an App action to a keyseq.Action.
func run(f func() tea.Cmd) keyseq.Action {
	return func(tea.KeyMsg) tea.Cmd { return f() }
}

// newKeySequences builds the App's sequence tables from its KeyMap: the
// root holds the leader and ctrl+w prefixes (the leader first, so it
// wins a collision); the leader layer's w and ctrl+w share one
// window-command table. Descriptions are the KeyMap bindings' own, so a
// leader row in the help overlay sorts next to the chord it mirrors.
// Rebuilt whenever the KeyMap changes (SetLeader).
func (a *App) newKeySequences() *keyseq.Map {
	km := a.keys
	desc := func(b key.Binding) string { return b.Help().Desc }
	focus := func(nd wintree.NavDir) keyseq.Action {
		return func(tea.KeyMsg) tea.Cmd { return a.focusPaneDir(nd) }
	}
	// The window commands (window-management design §4).
	windows := keyseq.New(
		keyseq.Binding{Keys: []string{"h", "left"}, Hint: "focus", Desc: desc(km.WinNavigate), Action: focus(wintree.NavLeft)},
		keyseq.Binding{Keys: []string{"j", "down"}, Hint: "focus", Desc: desc(km.WinNavigate), Action: focus(wintree.NavDown)},
		keyseq.Binding{Keys: []string{"k", "up"}, Hint: "focus", Desc: desc(km.WinNavigate), Action: focus(wintree.NavUp)},
		keyseq.Binding{Keys: []string{"l", "right"}, Hint: "focus", Desc: desc(km.WinNavigate), Action: focus(wintree.NavRight)},
		keyseq.Binding{Keys: []string{"s"}, Hint: "split", Desc: desc(km.WinSplit),
			Action: run(func() tea.Cmd { return a.splitWindow(wintree.SplitStacked) })},
		keyseq.Binding{Keys: []string{"v"}, Hint: "vsplit", Desc: desc(km.WinVSplit),
			Action: run(func() tea.Cmd { return a.splitWindow(wintree.SplitSideBySide) })},
		// vim: ctrl+w ctrl+w == ctrl+w w.
		keyseq.Binding{Keys: []string{"w", "ctrl+w"}, Hint: "cycle", Desc: desc(km.WinCycle), Action: run(a.cycleWindow)},
		keyseq.Binding{Keys: []string{"q"}, Hint: "close", Desc: desc(km.WinClose), Action: run(a.closeWindow)},
		keyseq.Binding{Keys: []string{"c"}, Hint: "close", Desc: desc(km.WinClose), Action: run(a.closeWindow)},
		keyseq.Binding{Keys: []string{"o"}, Hint: "only", Desc: desc(km.WinOnly),
			Action: run(func() tea.Cmd { a.onlyWindow(); return nil })},
	)

	// The leader layer: the leader stands in for ctrl, so each entry is
	// the letter of the ctrl chord it mirrors.
	var layer []keyseq.Binding
	leader := km.Leader.Keys()[0]
	if d, ok := displacedBinding(km, leader); ok {
		// <leader><leader> hands the key back to what it did before it
		// became the leader. First, so it wins over a layer letter.
		layer = append(layer, keyseq.Binding{Keys: []string{leader}, Hint: desc(d), Desc: desc(d),
			Action: func(msg tea.KeyMsg) tea.Cmd { return handleNormalKey(a, msg) }})
	}
	layer = append(layer,
		keyseq.Binding{Keys: []string{"w"}, Hint: "window", Desc: desc(km.WindowPrefix), Next: windows},
		keyseq.Binding{Keys: []string{"t"}, Hint: "channel", Desc: desc(km.FuzzyFinder), Action: run(a.openChannelFinder)},
		keyseq.Binding{Keys: []string{"f"}, Hint: "search", Desc: desc(km.WorkspaceSearch), Action: run(a.openWorkspaceSearch)},
		keyseq.Binding{Keys: []string{"a"}, Hint: "activity", Desc: desc(km.ActivityView), Action: run(a.toggleActivityView)},
		keyseq.Binding{Keys: []string{"n"}, Hint: "new msg", Desc: desc(km.NewMessage), Action: run(a.enterNewMessage)},
		keyseq.Binding{Keys: []string{"s"}, Hint: "status", Desc: desc(km.PresenceMenu), Action: run(a.openPresenceMenu)},
		keyseq.Binding{Keys: []string{"y"}, Hint: "theme", Desc: desc(km.ThemeSwitcher), Action: run(a.openWorkspaceThemeSwitcher)},
		keyseq.Binding{Keys: []string{"Y"}, Hint: "default theme", Desc: desc(km.ThemeSwitcherGlobal), Action: run(a.openGlobalThemeSwitcher)},
		keyseq.Binding{Keys: []string{"b"}, Hint: "sidebar", Desc: desc(km.ToggleSidebar),
			Action: run(func() tea.Cmd { a.ToggleSidebar(); return nil })},
		keyseq.Binding{Keys: []string{"]"}, Hint: "thread", Desc: desc(km.ToggleThread),
			Action: run(func() tea.Cmd { a.ToggleThread(); return nil })},
	)

	return keyseq.New(
		keyseq.Binding{Keys: km.Leader.Keys(), Next: keyseq.New(layer...)},
		keyseq.Binding{Keys: km.WindowPrefix.Keys(), Next: windows},
	)
}

// notDisplaceable names the KeyMap bindings a leader on the same key
// does not take a Normal-mode action from: the leader itself, the
// sequence prefixes (still reachable through the leader layer, e.g.
// ctrl+w as <leader>w), and keys only Insert mode handles.
var notDisplaceable = []string{"Leader", "WindowPrefix", "ToggleBroadcast", "OpenInEditor"}

// displacedBinding returns the Normal-mode KeyMap binding whose key the
// leader takes over, if any (the first, in KeyMap order).
func displacedBinding(km KeyMap, leader string) (key.Binding, bool) {
	v := reflect.ValueOf(km)
	for i := range v.NumField() {
		if slices.Contains(notDisplaceable, v.Type().Field(i).Name) {
			continue
		}
		if b, ok := v.Field(i).Interface().(key.Binding); ok && slices.Contains(b.Keys(), leader) {
			return b, true
		}
	}
	return key.Binding{}, false
}

// SetLeader makes k the leader key: a key string as slk matches keys
// ("\\", "space", "ctrl+x"), validated by the config layer ([keys]
// leader). "" keeps the current leader.
func (a *App) SetLeader(k string) {
	if k == "" {
		return
	}
	a.keys.Leader = key.NewBinding(key.WithKeys(k), key.WithHelp(k, leaderHelpDesc))
	a.sequences = a.newKeySequences()
	a.pendingSeq.Reset()
}

// pressKeySeq feeds msg to the pending sequence or, idle, to the root
// table. It reports false only for an idle key that starts no sequence,
// which the caller then handles on its own. While a sequence is pending
// the status bar lists what may follow; when it ends (completed,
// cancelled by Esc or an unbound key) the resting hint returns.
func (a *App) pressKeySeq(msg tea.KeyMsg) (tea.Cmd, bool) {
	act, consumed := a.pendingSeq.Press(a.sequences, msg.String())
	if !consumed {
		return nil, false
	}
	if a.pendingSeq.Pending() {
		a.statusbar.SetKeyHint(a.pendingSeq.Hint())
	} else {
		a.statusbar.SetHelpHint(a.defaultHelpHint())
	}
	if act == nil {
		return nil, true
	}
	return act(msg), true
}

// helpEntries is the help overlay's content: the KeyMap's rows plus the
// leader layer's, generated from the table under the configured leader.
func (a *App) helpEntries() []help.Entry {
	leader := a.keys.Leader.Keys()[0]
	var rows []help.Entry
	if b, ok := a.sequences.Lookup(leader); ok && b.Next != nil {
		for _, e := range b.Next.Help(leader) {
			rows = append(rows, help.Entry{Key: e.Keys, Desc: e.Desc})
		}
	}
	return help.FromKeyMap(a.keys, rows...)
}
