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
	"strings"

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

// newKeySequences builds the App's sequence tables from its KeyMap and
// leader layer: the root holds the leader and ctrl+w prefixes (the
// leader first, so it wins a collision); the leader layer's
// window_commands and ctrl+w share one window-command table.
// Descriptions are the KeyMap bindings' own, so a leader row in the
// help overlay sorts next to the chord it mirrors. Rebuilt whenever the
// KeyMap or the layer changes (SetLeader, SetKeyBindings,
// SetLeaderLayer).
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

	return keyseq.New(
		keyseq.Binding{Keys: km.Leader.Keys(), Next: a.newLeaderLayer(windows)},
		keyseq.Binding{Keys: km.WindowPrefix.Keys(), Hint: windowsLabel, Next: windows},
	)
}

// insertOnly names the KeyMap bindings only Insert mode handles; a
// Normal-mode leader on the same key leaves them alone.
var insertOnly = []string{"ToggleBroadcast", "OpenInEditor"}

// chordTails names the KeyMap bindings whose key only follows g (the p
// of gp). A leader on the same key leaves them alone too: the pending g
// is handled before the leader, so gp still works.
var chordTails = []string{"ViewInChannel"}

// notDisplaceable names the KeyMap bindings a leader on the same key
// does not take a Normal-mode action from: the leader itself, the
// sequence prefixes (still reachable through the leader layer, e.g.
// ctrl+w as <leader>w), the Insert-only keys and the chord tails.
var notDisplaceable = slices.Concat([]string{"Leader", "WindowPrefix"}, insertOnly, chordTails)

// displacedBindings returns the Normal-mode KeyMap bindings whose key
// the leader takes over, in KeyMap order. More than one when the key
// means different things in different panels (l is focus-right, and
// may also be the sidebar's sidebar_open).
func displacedBindings(km KeyMap, leader string) []key.Binding {
	var out []key.Binding
	v := reflect.ValueOf(km)
	for i := range v.NumField() {
		if slices.Contains(notDisplaceable, v.Type().Field(i).Name) {
			continue
		}
		if b, ok := v.Field(i).Interface().(key.Binding); ok && slices.Contains(b.Keys(), leader) {
			out = append(out, b)
		}
	}
	return out
}

// helpKeyMap is km as the help overlay describes it once leader owns its
// key in Normal mode. A binding on the leader's key loses that key: its
// row is relabelled with the keys it keeps ("Y/C" → "C" for a Y leader)
// or dropped when none are left. A keyless chord row under the leader's
// key ("ctrl+w s / :sp" for a ctrl+w leader) is dropped too. The leader
// layer's rows say what those keys do now. A chord tail keeps its row
// ("gp" with a p leader): its key follows g, which the leader leaves.
// A leader on g itself takes the chord, and the chord tails' rows go.
func helpKeyMap(km KeyMap, leader string) KeyMap {
	// Read before the loop, which clears Top for a g leader.
	chordTaken := slices.Contains(km.Top.Keys(), leader)
	v := reflect.ValueOf(&km).Elem()
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		b, ok := v.Field(i).Interface().(key.Binding)
		if !ok || name == "Leader" || slices.Contains(insertOnly, name) {
			continue
		}
		if slices.Contains(chordTails, name) {
			if chordTaken {
				v.Field(i).Set(reflect.ValueOf(key.Binding{}))
			}
			continue
		}
		h := b.Help()
		switch keys := b.Keys(); {
		case slices.Contains(keys, leader):
			kept := slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return k == leader })
			b = key.Binding{}
			if len(kept) > 0 {
				b = key.NewBinding(key.WithKeys(kept...), key.WithHelp(strings.Join(kept, "/"), h.Desc))
			}
		case len(keys) == 0 && (h.Key == leader || strings.HasPrefix(h.Key, leader+" ")):
			b = key.Binding{}
		default:
			continue
		}
		v.Field(i).Set(reflect.ValueOf(b))
	}
	return km
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
// cancelled by Esc or an unbound key) the resting hint returns, unless
// the action it ran left another key pending (a key from the context
// menu that arms the leader, gg or a mark, or the context menu itself),
// whose hint stays.
func (a *App) pressKeySeq(msg tea.KeyMsg) (tea.Cmd, bool) {
	act, consumed := a.pendingSeq.Press(a.sequences, msg.String())
	if !consumed {
		return nil, false
	}
	var cmd tea.Cmd
	if act != nil {
		cmd = act(msg)
	}
	switch {
	case a.pendingSeq.Pending():
		a.statusbar.SetKeyHint(a.pendingSeq.Hint())
	case a.pendingHintSource() == nil:
		a.statusbar.SetHelpHint(a.defaultHelpHint())
	}
	return cmd, true
}

// helpEntries is the help overlay's content: the KeyMap's rows as the
// leader leaves them (helpKeyMap) plus the leader layer's, generated
// from the table under the configured leader.
func (a *App) helpEntries() []help.Entry {
	leader := a.keys.Leader.Keys()[0]
	var rows []help.Entry
	if b, ok := a.sequences.Lookup(leader); ok && b.Next != nil {
		for _, e := range b.Next.Help(leader) {
			rows = append(rows, help.Entry{Key: e.Keys, Desc: e.Desc})
		}
	}
	return help.FromKeyMap(helpKeyMap(a.keys, leader), rows...)
}
