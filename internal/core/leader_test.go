package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// layerString renders a layer as "action=keys" items, in order.
func layerString(seqs []LeaderSeq) []string {
	var out []string
	for _, s := range seqs {
		item := s.Action + "=" + strings.Join(s.Keys, " ")
		if s.Explicit {
			item += "!"
		}
		out = append(out, item)
	}
	return out
}

func TestDefaultLeaderLayer(t *testing.T) {
	want := []string{"window_commands=w", "channel_finder=t", "workspace_search=f", "toggle_activity=a",
		"new_message=n", "set_status=s", "switch_theme=y", "set_default_theme=Y", "toggle_sidebar=b", "toggle_thread=]"}
	if got := layerString(DefaultLeaderLayer()); !slices.Equal(got, want) {
		t.Fatalf("default layer = %q, want %q", got, want)
	}
	// The defaults must not collide with each other: nothing resolves
	// them against one another.
	defaults := DefaultLeaderLayer()
	for i, a := range defaults {
		for _, b := range defaults[i+1:] {
			if overlap(a.Keys, b.Keys) {
				t.Errorf("defaults %s=%q and %s=%q overlap", a.Action, a.Keys, b.Action, b.Keys)
			}
		}
	}
}

func TestResolveLeaderMap(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    map[string][]string
		want []string
	}{
		{"nil keeps the defaults", nil, layerString(DefaultLeaderLayer())},
		{
			"an empty list removes an action",
			map[string][]string{"set_status": {}},
			[]string{"window_commands=w", "channel_finder=t", "workspace_search=f", "toggle_activity=a",
				"new_message=n", "switch_theme=y", "set_default_theme=Y", "toggle_sidebar=b", "toggle_thread=]"},
		},
		{
			"a set action replaces its defaults",
			map[string][]string{"channel_finder": {"p"}, "help": {"?"}},
			[]string{"window_commands=w", "channel_finder=p!", "workspace_search=f", "toggle_activity=a",
				"new_message=n", "set_status=s", "switch_theme=y", "set_default_theme=Y", "toggle_sidebar=b", "toggle_thread=]", "help=?!"},
		},
		{
			// workspace_search's default f collides with f f: dropped.
			"a configured sequence drops a colliding default",
			map[string][]string{"channel_finder": {"f f"}},
			[]string{"window_commands=w", "channel_finder=f f!", "toggle_activity=a",
				"new_message=n", "set_status=s", "switch_theme=y", "set_default_theme=Y", "toggle_sidebar=b", "toggle_thread=]"},
		},
		{
			"the nvim example",
			map[string][]string{
				"channel_finder":   {"f f", "space"},
				"workspace_search": {"/", "s g"},
				"toggle_sidebar":   {"e"},
				"help":             {"?"},
				"close_thread":     {"q"},
			},
			[]string{"window_commands=w", "channel_finder=f f!", "channel_finder=space!", "workspace_search=/!", "workspace_search=s g!",
				"toggle_activity=a", "new_message=n", "switch_theme=y", "set_default_theme=Y", "toggle_sidebar=e!", "toggle_thread=]",
				"help=?!", "close_thread=q!"},
		},
		{"a repeat is kept once", map[string][]string{"help": {"?", "?"}},
			append(layerString(DefaultLeaderLayer()), "help=?!")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveLeaderMap(tc.m, nil)
			if err != nil {
				t.Fatal(err)
			}
			if gs := layerString(got); !slices.Equal(gs, tc.want) {
				t.Fatalf("layer =\n%q\nwant\n%q", gs, tc.want)
			}
		})
	}
}

func TestResolveLeaderMapErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		m      map[string][]string
		prefix string // the error starts with it (the action named)
	}{
		{"unknown action", map[string][]string{"open_everything": {"o"}}, "open_everything: not a leader action"},
		{"empty sequence", map[string][]string{"help": {""}}, "help: an empty sequence"},
		{"blank sequence", map[string][]string{"help": {"  "}}, "help: an empty sequence"},
		{"the same sequence on two actions", map[string][]string{"help": {"h"}, "quit": {"h"}}, `quit: "h" is also bound to help`},
		{"a prefix of another action's", map[string][]string{"channel_finder": {"f f"}, "workspace_search": {"f"}}, `workspace_search: "f" and channel_finder's "f f" overlap`},
		{"an extension of another action's", map[string][]string{"channel_finder": {"f"}, "workspace_search": {"f g"}}, `workspace_search: "f g" and channel_finder's "f" overlap`},
		{"a prefix within one action", map[string][]string{"help": {"h", "h h"}}, `help: "h h" and help's "h" overlap`},
		{"after the window prefix", map[string][]string{"window_commands": {"w"}, "help": {"w h"}}, `help: "w h" and window_commands's "w" overlap`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveLeaderMap(tc.m, nil)
			if err == nil || !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Fatalf("error = %v, want one starting %q", err, tc.prefix)
			}
		})
	}
}

// normalizeKey respells each key before collisions are judged, and its
// error names the action.
func TestResolveLeaderMapNormalizes(t *testing.T) {
	lower := func(k string) (string, error) {
		if k == "bad" {
			return "", errors.New("bad key")
		}
		return strings.ToLower(k), nil
	}
	got, err := ResolveLeaderMap(map[string][]string{"help": {"H  X"}}, lower)
	if err != nil {
		t.Fatal(err)
	}
	if last := got[len(got)-1]; last.Action != "help" || !slices.Equal(last.Keys, []string{"h", "x"}) {
		t.Fatalf("help = %+v, want keys [h x]", last)
	}
	if _, err := ResolveLeaderMap(map[string][]string{"help": {"H"}, "quit": {"h"}}, lower); err == nil {
		t.Fatal("H and h collide once normalized, but resolved without error")
	}
	if _, err := ResolveLeaderMap(map[string][]string{"help": {"f bad"}}, lower); err == nil || err.Error() != "help: bad key" {
		t.Fatalf("error = %v, want %q", err, "help: bad key")
	}
}

// Removing every action is an empty layer, not nil: nil means "no map,
// keep the defaults" to the TUI.
func TestResolveLeaderMapEverythingRemoved(t *testing.T) {
	m := map[string][]string{}
	for _, a := range LeaderActions() {
		m[a.Name] = []string{}
	}
	got, err := ResolveLeaderMap(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("layer = %#v, want an empty, non-nil layer", got)
	}
}
