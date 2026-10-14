package core

import (
	"fmt"
	"slices"
	"strings"
)

// The leader layer's vocabulary, shared by the config layer (which
// validates a [keys.leader_map] against it) and the TUI (which builds
// the layer from it and binds each action name to what it does).
//
// A sequence is the keys typed after the leader, as slk spells keys
// ("t", "space", "ctrl+x"), separated by single spaces: "f f" is f
// twice.

// LeaderAction is one action the leader layer can run: its
// [keys.leader_map] name and the sequences it has when the map leaves
// it unset (none: not in the default layer).
type LeaderAction struct {
	Name     string
	Defaults []string
}

// LeaderActions lists every leader action, in the order the which-key
// hint shows them. The default layer mirrors the ctrl chords: the
// leader stands in for ctrl (w is ctrl+w, t is ctrl+t, …).
func LeaderActions() []LeaderAction {
	return []LeaderAction{
		{Name: "window_commands", Defaults: []string{"w"}},
		{Name: "channel_finder", Defaults: []string{"t"}},
		{Name: "workspace_search", Defaults: []string{"f"}},
		{Name: "toggle_activity", Defaults: []string{"a"}},
		{Name: "new_message", Defaults: []string{"n"}},
		{Name: "set_status", Defaults: []string{"s"}},
		{Name: "switch_theme", Defaults: []string{"y"}},
		{Name: "set_default_theme", Defaults: []string{"Y"}},
		{Name: "toggle_sidebar", Defaults: []string{"b"}},
		{Name: "toggle_thread", Defaults: []string{"]"}},
		{Name: "help"},
		{Name: "close_thread"},
		{Name: "nav_back"},
		{Name: "nav_forward"},
		{Name: "next_unread"},
		{Name: "prev_unread"},
		{Name: "next_theme"},
		{Name: "prev_theme"},
		{Name: "command_mode"},
		{Name: "quit"},
	}
}

// LeaderSeq is one sequence of the effective leader layer: the action
// it runs, its keys, and whether the user configured it (Explicit) or
// it is the action's default.
type LeaderSeq struct {
	Action   string
	Keys     []string
	Explicit bool
}

// DefaultLeaderLayer is the layer with no [keys.leader_map].
func DefaultLeaderLayer() []LeaderSeq {
	return withLeaderDefaults(nil, nil)
}

// ResolveLeaderMap returns the effective leader layer for m, a
// [keys.leader_map]: an action m sets has exactly m's sequences ([]
// removes it from the layer); any other keeps its defaults, except a
// default that collides with a configured sequence (the same keys, or
// one beginning the other), which is dropped: explicit beats implicit,
// and the action keeps its ctrl chord. A sequence repeated within one
// action is kept once. normalizeKey, when not nil, validates and
// respells each key of a configured sequence (the config layer's key
// rules); nil takes keys as written.
//
// The error, which names the action first ("help: …"), is for an
// unknown action, an empty sequence, a key normalizeKey rejects, or two
// configured sequences that collide: with no timeout, a sequence that
// begins another would always win, leaving the longer one impossible
// to type.
func ResolveLeaderMap(m map[string][]string, normalizeKey func(string) (string, error)) ([]LeaderSeq, error) {
	actions := LeaderActions()
	var unknown []string
	for name := range m {
		if !slices.ContainsFunc(actions, func(a LeaderAction) bool { return a.Name == name }) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		names := make([]string, len(actions))
		for i, a := range actions {
			names[i] = a.Name
		}
		return nil, fmt.Errorf("%s: not a leader action (one of %s)", unknown[0], strings.Join(names, ", "))
	}
	var explicit []LeaderSeq
	for _, a := range actions {
		seqs, ok := m[a.Name]
		if !ok {
			continue
		}
		for _, s := range seqs {
			keys := strings.Fields(s)
			if len(keys) == 0 {
				return nil, fmt.Errorf("%s: an empty sequence", a.Name)
			}
			if normalizeKey != nil {
				for i, k := range keys {
					nk, err := normalizeKey(k)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", a.Name, err)
					}
					keys[i] = nk
				}
			}
			seq := LeaderSeq{Action: a.Name, Keys: keys, Explicit: true}
			if i := slices.IndexFunc(explicit, func(o LeaderSeq) bool { return overlap(o.Keys, keys) }); i >= 0 {
				o := explicit[i]
				switch {
				case o.Action == a.Name && slices.Equal(o.Keys, keys):
					continue // a repeat
				case slices.Equal(o.Keys, keys):
					return nil, fmt.Errorf("%s: %q is also bound to %s", a.Name, strings.Join(keys, " "), o.Action)
				default:
					return nil, fmt.Errorf("%s: %q and %s's %q overlap: one begins the other, and with no timeout the longer could never be typed",
						a.Name, strings.Join(keys, " "), o.Action, strings.Join(o.Keys, " "))
				}
			}
			explicit = append(explicit, seq)
		}
	}
	return withLeaderDefaults(m, explicit), nil
}

// withLeaderDefaults merges explicit (the sequences of the actions m
// sets) with the defaults of the actions m leaves unset, in
// LeaderActions order, dropping each default that collides with an
// explicit sequence. The result is never nil, even with every action
// removed.
func withLeaderDefaults(m map[string][]string, explicit []LeaderSeq) []LeaderSeq {
	out := []LeaderSeq{}
	for _, a := range LeaderActions() {
		if _, set := m[a.Name]; set {
			for _, s := range explicit {
				if s.Action == a.Name {
					out = append(out, s)
				}
			}
			continue
		}
		for _, d := range a.Defaults {
			keys := strings.Fields(d)
			if !slices.ContainsFunc(explicit, func(e LeaderSeq) bool { return overlap(e.Keys, keys) }) {
				out = append(out, LeaderSeq{Action: a.Name, Keys: keys})
			}
		}
	}
	return out
}

// overlap reports whether a and b are equal or one begins the other.
func overlap(a, b []string) bool {
	n := min(len(a), len(b))
	return slices.Equal(a[:n], b[:n])
}
