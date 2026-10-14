// internal/ui/leader_layer.go
//
// The leader layer (leader-keys design §9): which sequences after the
// leader run which action. core.LeaderActions names the actions and
// their default sequences; [keys.leader_map] changes them, resolved by
// the config layer into the effective layer (core.ResolveLeaderMap).
// This file binds each action name to what it does and builds the
// keyseq table: a sequence of several keys ("f f") makes the keys
// before its last a nested group the which-key hint shows as "f +…",
// or "f +find" with the label [keys.leader_groups] gives it.
package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/bubbles/keyseq"
	"github.com/gammons/slk/internal/core"
)

// leaderAction is what a leader-layer action does: its which-key label,
// the KeyMap binding whose description it shows (so its help row sorts
// next to that binding's), or desc for an action that mirrors no
// binding, and its handler. A nil run opens the window-command table
// instead (window_commands); its hint is then the group's default
// label.
type leaderAction struct {
	hint    string
	binding func(*KeyMap) *key.Binding
	desc    string
	run     func(*App) tea.Cmd
}

// windowsLabel is the window-command group's label: after ctrl+w, and
// after the leader's window_commands unless [keys.leader_groups]
// relabels it.
const windowsLabel = "windows"

// leaderActions binds every core.LeaderActions name to what it does
// (TestLeaderActionsMatchCore). All are global Normal-mode commands:
// nothing here depends on a selection or the focused pane. Not
// separate from keyActions by accident: those are keys scoped to a
// panel, configured as [keys] fields with KeyMap defaults; these are
// sequences in the leader layer, configured in [keys.leader_map] with
// core defaults, and no action is in both.
var leaderActions = map[string]leaderAction{
	"window_commands": {hint: windowsLabel, binding: func(km *KeyMap) *key.Binding { return &km.WindowPrefix }},
	"channel_finder": {hint: "channel", binding: func(km *KeyMap) *key.Binding { return &km.FuzzyFinder },
		run: (*App).openChannelFinder},
	"workspace_search": {hint: "search", binding: func(km *KeyMap) *key.Binding { return &km.WorkspaceSearch },
		run: (*App).openWorkspaceSearch},
	"toggle_activity": {hint: "activity", binding: func(km *KeyMap) *key.Binding { return &km.ActivityView },
		run: (*App).toggleActivityView},
	"new_message": {hint: "new msg", binding: func(km *KeyMap) *key.Binding { return &km.NewMessage },
		run: (*App).enterNewMessage},
	"set_status": {hint: "status", binding: func(km *KeyMap) *key.Binding { return &km.PresenceMenu },
		run: (*App).openPresenceMenu},
	"switch_theme": {hint: "theme", binding: func(km *KeyMap) *key.Binding { return &km.ThemeSwitcher },
		run: (*App).openWorkspaceThemeSwitcher},
	"set_default_theme": {hint: "default theme", binding: func(km *KeyMap) *key.Binding { return &km.ThemeSwitcherGlobal },
		run: (*App).openGlobalThemeSwitcher},
	"toggle_sidebar": {hint: "sidebar", binding: func(km *KeyMap) *key.Binding { return &km.ToggleSidebar },
		run: func(a *App) tea.Cmd { a.ToggleSidebar(); return nil }},
	"toggle_thread": {hint: "thread", binding: func(km *KeyMap) *key.Binding { return &km.ToggleThread },
		run: func(a *App) tea.Cmd { a.ToggleThread(); return nil }},
	"key_hints": {hint: "keys", desc: keyHintsDesc, run: (*App).openContextHints},
	"help": {hint: "help", binding: func(km *KeyMap) *key.Binding { return &km.Help },
		run: (*App).openHelp},
	"close_thread": {hint: "close thread", binding: func(km *KeyMap) *key.Binding { return &km.CloseThreadView },
		run: (*App).closeThreadView},
	"view_in_channel": {hint: "in channel", binding: func(km *KeyMap) *key.Binding { return &km.ViewInChannel },
		run: (*App).viewThreadInChannel},
	"nav_back": {hint: "back", binding: func(km *KeyMap) *key.Binding { return &km.NavBack },
		run: (*App).navigateBack},
	"nav_forward": {hint: "forward", binding: func(km *KeyMap) *key.Binding { return &km.NavForward },
		run: (*App).navigateForward},
	"next_unread": {hint: "next unread", binding: func(km *KeyMap) *key.Binding { return &km.NextUnread },
		run: func(a *App) tea.Cmd { return a.jumpToUnread(1) }},
	"prev_unread": {hint: "prev unread", binding: func(km *KeyMap) *key.Binding { return &km.PrevUnread },
		run: func(a *App) tea.Cmd { return a.jumpToUnread(-1) }},
	"next_theme": {hint: "next theme", binding: func(km *KeyMap) *key.Binding { return &km.ThemeNext },
		run: func(a *App) tea.Cmd { return a.cycleTheme(1) }},
	"prev_theme": {hint: "prev theme", binding: func(km *KeyMap) *key.Binding { return &km.ThemePrev },
		run: func(a *App) tea.Cmd { return a.cycleTheme(-1) }},
	"command_mode": {hint: "command", binding: func(km *KeyMap) *key.Binding { return &km.CommandMode },
		run: func(a *App) tea.Cmd { a.enterCommandMode(); return nil }},
	"quit": {hint: "quit", binding: func(km *KeyMap) *key.Binding { return &km.QuitConfirm },
		run: func(a *App) tea.Cmd { a.openQuitConfirm(); return nil }},
}

// groupHint is the which-key label of a nested group [keys.leader_groups]
// does not label ("f +…").
const groupHint = "…"

// SetLeaderLayer replaces the leader layer with seqs, the effective
// layer the config layer resolved from [keys.leader_map]. nil keeps the
// default layer (core.DefaultLeaderLayer).
func (a *App) SetLeaderLayer(seqs []core.LeaderSeq) {
	if seqs == nil {
		return
	}
	a.leaderSeqs = seqs
	a.sequences = a.newKeySequences()
	a.pendingSeq.Reset()
}

// SetLeaderGroups labels the leader layer's groups: labels maps a
// sequence after the leader, its keys joined by single spaces ("f",
// "s g"), to the label its group shows ("find" reads "+find"), as the
// config layer resolved [keys.leader_groups]. nil keeps the default
// labels (the window commands' "windows", "…" for the rest).
func (a *App) SetLeaderGroups(labels map[string]string) {
	if labels == nil {
		return
	}
	a.leaderGroups = labels
	a.sequences = a.newKeySequences()
	a.pendingSeq.Reset()
}

// newLeaderLayer builds the table the leader opens, windows being the
// window-command table window_commands opens.
//
// <leader><leader> hands a key the leader displaced back to its action
// (handleNormalKey decides which, by focused panel; the label names
// every action the key had). Explicit beats implicit: when a sequence
// the user configured starts with the leader's key, that sequence wins
// and the fallback is left out; otherwise the fallback wins over a
// default sequence starting with the leader's key, which is left out of
// the table, the hint and the help (its ctrl chord still works).
func (a *App) newLeaderLayer(windows *keyseq.Map) *keyseq.Map {
	km := a.keys
	seqs := a.leaderSeqs
	if seqs == nil {
		seqs = core.DefaultLeaderLayer()
	}
	leader := km.Leader.Keys()[0]
	var first []keyseq.Binding
	displaced := displacedBindings(km, leader)
	explicitOnLeader := slices.ContainsFunc(seqs, func(s core.LeaderSeq) bool { return s.Explicit && s.Keys[0] == leader })
	if len(displaced) > 0 && !explicitOnLeader {
		descs := make([]string, len(displaced))
		for i, b := range displaced {
			descs[i] = b.Help().Desc
		}
		label := strings.Join(descs, "; ")
		first = append(first, keyseq.Binding{Keys: []string{leader}, Hint: label, Desc: label,
			Action: func(msg tea.KeyMsg) tea.Cmd { return handleNormalKey(a, msg) }})
		seqs = slices.DeleteFunc(slices.Clone(seqs), func(s core.LeaderSeq) bool { return s.Keys[0] == leader })
	}
	var root layerNode
	for _, s := range seqs {
		root.insert(s.Keys, s.Action)
	}
	return a.layerMap(&root, nil, windows, first...)
}

// layerNode is a key in the layer's tree: a leaf runs action, a group
// (action "") continues with its children.
type layerNode struct {
	key      string
	action   string
	children []*layerNode
}

// insert adds the sequence keys → action under n. A sequence that
// collides with one inserted before (the config layer rejects those) is
// left out.
func (n *layerNode) insert(keys []string, action string) {
	cur := n
	for i, k := range keys {
		last := i == len(keys)-1
		j := slices.IndexFunc(cur.children, func(c *layerNode) bool { return c.key == k })
		if j < 0 {
			child := &layerNode{key: k}
			cur.children = append(cur.children, child)
			if last {
				child.action = action
				return
			}
			cur = child
			continue
		}
		if last || cur.children[j].action != "" {
			return
		}
		cur = cur.children[j]
	}
}

// layerMap turns n's children into a keyseq table after first. path is
// the sequence after the leader that leads to n, for the group labels.
func (a *App) layerMap(n *layerNode, path []string, windows *keyseq.Map, first ...keyseq.Binding) *keyseq.Map {
	bindings := first
	for _, c := range n.children {
		seq := append(slices.Clone(path), c.key)
		label := a.leaderGroups[strings.Join(seq, " ")]
		if c.action == "" {
			if label == "" {
				label = groupHint
			}
			bindings = append(bindings, keyseq.Binding{Keys: []string{c.key}, Hint: label, Next: a.layerMap(c, seq, windows)})
			continue
		}
		act, ok := leaderActions[c.action]
		if !ok {
			continue
		}
		desc := act.desc
		if act.binding != nil {
			desc = act.binding(&a.keys).Help().Desc
		}
		b := keyseq.Binding{Keys: []string{c.key}, Hint: act.hint, Desc: desc}
		if act.run == nil {
			b.Next = windows
			if label != "" {
				b.Hint = label
			}
		} else {
			run := act.run
			b.Action = func(tea.KeyMsg) tea.Cmd { return run(a) }
		}
		bindings = append(bindings, b)
	}
	return keyseq.New(bindings...)
}
