// internal/ui/leader_layer_test.go
//
// The configurable leader layer (leader-keys design §9): the action
// registry against core's, the nvim example through the real Update
// chain, removing and replacing actions, nested-group hints, explicit
// sequences beating <leader><leader>, and the help rows.
package ui

import (
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/help"
)

// nvimLeaderMap is the [keys.leader_map] of wiki/Configuration.md's
// nvim example, keys already normalized.
var nvimLeaderMap = map[string][]string{
	"channel_finder":   {"f f", "space"},
	"workspace_search": {"/", "s g"},
	"toggle_sidebar":   {"e"},
	"help":             {"?"},
	"close_thread":     {"q"},
}

// withLeaderMap resolves m (normalized sequences) and installs it.
func withLeaderMap(t *testing.T, a *App, m map[string][]string) {
	t.Helper()
	layer, err := core.ResolveLeaderMap(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.SetLeaderLayer(layer)
}

// nvimApp is an App configured like the nvim example: leader space,
// sidebar_open on enter and l, and nvimLeaderMap.
func nvimApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t, normalOpts()...)
	a.SetLeader("space")
	a.SetKeyBindings(neovimSidebar)
	withLeaderMap(t, a, nvimLeaderMap)
	return a
}

// typeTokens sends keys through the real Update chain, spelled as
// tokens: "space", "esc" or a single character.
func typeTokens(t *testing.T, a *App, tokens ...string) {
	t.Helper()
	for _, tok := range tokens {
		switch tok {
		case "space":
			a.Update(keyPress(' '))
		case "esc":
			a.Update(keyCode(tea.KeyEscape))
		default:
			r := []rune(tok)
			if len(r) != 1 {
				t.Fatalf("typeTokens: cannot type %q", tok)
			}
			a.Update(keyPress(r[0]))
		}
	}
}

// hint is the status bar's which-key hint, wide enough to hold it all.
func hint(a *App) string { return stripANSI(a.statusbar.View(240)) }

func TestLeaderActionsMatchCore(t *testing.T) {
	var names []string
	for _, act := range core.LeaderActions() {
		names = append(names, act.Name)
	}
	slices.Sort(names)
	if got := slices.Sorted(maps.Keys(leaderActions)); !slices.Equal(got, names) {
		t.Fatalf("ui leader actions %q, core %q", got, names)
	}
}

// The nvim example, key by key, through Update.
func TestNvimLeaderLayer(t *testing.T) {
	var section string
	var collapsed bool
	for _, tc := range []struct {
		name   string
		setup  func(t *testing.T, a *App)
		keys   []string
		assert func(t *testing.T, a *App)
	}{
		{
			name: "space f f opens the channel finder",
			keys: []string{"space", "f", "f"},
			assert: func(t *testing.T, a *App) {
				if a.mode != ModeChannelFinder {
					t.Errorf("mode = %v, want the channel finder", a.mode)
				}
			},
		},
		{
			// An explicit "space" beats <leader><leader>, which would
			// have toggled the section under the sidebar cursor.
			name: "space space opens the channel finder, not the section toggle",
			setup: func(t *testing.T, a *App) {
				a.focusedPanel = PanelSidebar
				section = selectSidebarSectionHeader(t, a)
				collapsed = a.sidebar.IsCollapsed(section)
			},
			keys: []string{"space", "space"},
			assert: func(t *testing.T, a *App) {
				if a.mode != ModeChannelFinder {
					t.Errorf("mode = %v, want the channel finder", a.mode)
				}
				if a.sidebar.IsCollapsed(section) != collapsed {
					t.Errorf("space space toggled section %q", section)
				}
			},
		},
		{
			name: "space / opens workspace search",
			keys: []string{"space", "/"},
			assert: func(t *testing.T, a *App) {
				if a.mode != ModeWorkspaceSearch {
					t.Errorf("mode = %v, want workspace search", a.mode)
				}
			},
		},
		{
			name: "space s g opens workspace search",
			keys: []string{"space", "s", "g"},
			assert: func(t *testing.T, a *App) {
				if a.mode != ModeWorkspaceSearch {
					t.Errorf("mode = %v, want workspace search", a.mode)
				}
			},
		},
		{
			name: "space e toggles the sidebar",
			keys: []string{"space", "e"},
			assert: func(t *testing.T, a *App) {
				if a.sidebarVisible {
					t.Error("sidebar still visible")
				}
			},
		},
		{
			name: "space ? opens help",
			keys: []string{"space", "?"},
			assert: func(t *testing.T, a *App) {
				if a.mode != ModeHelp || !a.help.IsVisible() {
					t.Errorf("mode = %v, help visible %v; want help", a.mode, a.help.IsVisible())
				}
			},
		},
		{
			name:  "space q closes an open thread",
			setup: func(t *testing.T, a *App) { focusThreadPanel(t, a) },
			keys:  []string{"space", "q"},
			assert: func(t *testing.T, a *App) {
				if a.threadVisible {
					t.Error("thread still open")
				}
			},
		},
		{
			name:  "space w l still focuses right",
			setup: func(t *testing.T, a *App) { a.focusedPanel = PanelSidebar },
			keys:  []string{"space", "w", "l"},
			assert: func(t *testing.T, a *App) {
				if a.focusedPanel != PanelMessages {
					t.Errorf("focused %v, want the messages pane", a.focusedPanel)
				}
			},
		},
		{
			// The section toggle the leader took is still on Enter
			// (sidebar_open on a header).
			name: "enter on a section header still toggles it",
			setup: func(t *testing.T, a *App) {
				a.focusedPanel = PanelSidebar
				section := selectSidebarSectionHeader(t, a)
				was := a.sidebar.IsCollapsed(section)
				a.Update(keyCode(tea.KeyEnter))
				if a.sidebar.IsCollapsed(section) == was {
					t.Errorf("enter did not toggle %q", section)
				}
			},
		},
		{
			name: "space s (status) is gone: s is the workspace-search group",
			keys: []string{"space", "s", "esc"},
			assert: func(t *testing.T, a *App) {
				if a.mode != ModeNormal || a.presenceMenu.IsVisible() || a.pendingSeq.Pending() {
					t.Errorf("mode %v, presence menu %v, pending %v; want nothing open",
						a.mode, a.presenceMenu.IsVisible(), a.pendingSeq.Pending())
				}
			},
		},
		{
			name: "an unbound key inside a group cancels without side effects",
			setup: func(t *testing.T, a *App) {
				focusThreadPanel(t, a)
			},
			keys: []string{"space", "f", "q"},
			assert: func(t *testing.T, a *App) {
				if !a.threadVisible || a.pendingSeq.Pending() || a.mode != ModeNormal {
					t.Errorf("thread %v, pending %v, mode %v; want the q swallowed and the sequence over",
						a.threadVisible, a.pendingSeq.Pending(), a.mode)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := nvimApp(t)
			if tc.setup != nil {
				tc.setup(t, a)
			}
			typeTokens(t, a, tc.keys...)
			if tc.assert != nil {
				tc.assert(t, a)
			}
		})
	}
}

// The which-key hint shows an unlabelled group as "f +…" and, inside
// it, its continuations.
func TestNvimLeaderHints(t *testing.T) {
	a := nvimApp(t)
	typeTokens(t, a, "space")
	h := hint(a)
	for _, want := range []string{"space …  w +windows  f +…  space channel  / search  s +…  a activity", "e sidebar", "? help", "q close thread"} {
		if !strings.Contains(h, want) {
			t.Errorf("hint %q lacks %q", h, want)
		}
	}
	for _, gone := range []string{"t channel", "s status", "b sidebar", "toggle section"} {
		if strings.Contains(h, gone) {
			t.Errorf("hint %q still offers %q", h, gone)
		}
	}
	typeTokens(t, a, "s")
	if h := hint(a); !strings.Contains(h, "space s …  g search") {
		t.Errorf("group hint %q, want \"space s …  g search\"", h)
	}
	typeTokens(t, a, "esc")
	seqDone(t, a)
}

func TestNvimLeaderHelpRows(t *testing.T) {
	got := nvimApp(t).helpEntries()
	for _, e := range []help.Entry{
		{Key: "space f f", Desc: "switch channel"},
		{Key: "space space", Desc: "switch channel"},
		{Key: "space /", Desc: "search workspace"},
		{Key: "space s g", Desc: "search workspace"},
		{Key: "space e", Desc: "toggle sidebar"},
		{Key: "space ?", Desc: "show keybindings"},
		{Key: "space q", Desc: "close thread view"},
		{Key: "space w h/j/k/l", Desc: "focus pane in direction"},
		{Key: "enter/l", Desc: "open row / toggle section (sidebar)"},
	} {
		if !slices.Contains(got, e) {
			t.Errorf("help lacks %+v", e)
		}
	}
	for _, e := range []help.Entry{
		{Key: "space t", Desc: "switch channel"},
		{Key: "space f", Desc: "search workspace"},
		{Key: "space s", Desc: "set status"},
		{Key: "space b", Desc: "toggle sidebar"},
		{Key: "space space", Desc: "toggle section"},
		{Key: "space", Desc: "toggle section"},
	} {
		if slices.Contains(got, e) {
			t.Errorf("help still lists %+v", e)
		}
	}
}

// Removing and replacing actions, default leader.
func TestLeaderMapRemoveAndReplace(t *testing.T) {
	t.Run("[] removes an action; its ctrl chord still works", func(t *testing.T) {
		a := newTestApp(t, withSize(200, 50))
		withLeaderMap(t, a, map[string][]string{"window_commands": {}})
		typeTokens(t, a, `\`)
		if strings.Contains(hint(a), "w +windows") {
			t.Errorf("hint %q still offers w", hint(a))
		}
		typeTokens(t, a, "w", "v")
		if a.wins.Len() != 1 {
			t.Fatalf(`\ w v split the window after window_commands = []`)
		}
		_ = dispatchKeys(a, keyMod('w', tea.ModCtrl), keyPress('v'))
		if a.wins.Len() != 2 {
			t.Fatal("ctrl+w v no longer splits")
		}
	})
	t.Run("a set action loses its defaults", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		withLeaderMap(t, a, map[string][]string{"channel_finder": {"p"}})
		typeTokens(t, a, `\`, "t")
		if a.mode == ModeChannelFinder {
			t.Fatal(`\ t still opens the finder after channel_finder = ["p"]`)
		}
		typeTokens(t, a, `\`, "p")
		if a.mode != ModeChannelFinder {
			t.Fatalf(`\ p: mode = %v, want the channel finder`, a.mode)
		}
	})
	t.Run("a configured sequence drops another action's colliding default", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		withLeaderMap(t, a, map[string][]string{"channel_finder": {"f f"}})
		typeTokens(t, a, `\`, "f")
		if a.mode == ModeWorkspaceSearch || !a.pendingSeq.Pending() {
			t.Fatalf(`\ f: mode %v, pending %v; want the f group armed`, a.mode, a.pendingSeq.Pending())
		}
		typeTokens(t, a, "f")
		if a.mode != ModeChannelFinder {
			t.Fatalf(`\ f f: mode = %v, want the channel finder`, a.mode)
		}
	})
}

// The global commands the leader layer can take beyond the default
// layer, each bound to x.
func TestLeaderMapExtraActions(t *testing.T) {
	for _, tc := range []struct {
		action string
		setup  func(t *testing.T, a *App)
		check  func(t *testing.T, a *App, msgs []tea.Msg)
	}{
		{action: "help", check: func(t *testing.T, a *App, _ []tea.Msg) {
			if a.mode != ModeHelp {
				t.Errorf("mode = %v, want help", a.mode)
			}
		}},
		{action: "close_thread", setup: func(t *testing.T, a *App) { focusThreadPanel(t, a) },
			check: func(t *testing.T, a *App, _ []tea.Msg) {
				if a.threadVisible {
					t.Error("thread still open")
				}
			}},
		// The open thread is the active channel's: in place, the thread
		// kept beside the channel pane, which takes focus.
		{action: "view_in_channel", setup: func(t *testing.T, a *App) {
			a.setChannelLookupFuncForTest(func(ids.ChannelID) (string, string, bool) { return "general", "channel", true })
			focusThreadPanel(t, a)
		},
			check: func(t *testing.T, a *App, _ []tea.Msg) {
				if a.focusedPanel != PanelMessages || !a.threadVisible || a.threadPanel.ThreadTS() != "10.0" {
					t.Errorf("focus %v, thread visible %v (%s); want the channel pane focused, the thread kept", a.focusedPanel, a.threadVisible, a.threadPanel.ThreadTS())
				}
			}},
		{action: "quit", check: func(t *testing.T, a *App, _ []tea.Msg) {
			if a.mode != ModeConfirm {
				t.Errorf("mode = %v, want the quit prompt", a.mode)
			}
		}},
		{action: "command_mode", check: func(t *testing.T, a *App, _ []tea.Msg) {
			if a.mode != ModeCommand {
				t.Errorf("mode = %v, want command mode", a.mode)
			}
		}},
		{action: "next_unread", setup: func(t *testing.T, a *App) { seedUnreads(t, a) },
			check: func(t *testing.T, a *App, msgs []tea.Msg) {
				if !slices.ContainsFunc(msgs, func(m tea.Msg) bool { s, ok := m.(ChannelSelectedMsg); return ok && s.ID == "C2" }) {
					t.Errorf("messages %v, want C2 selected", msgs)
				}
			}},
		{action: "prev_unread", setup: func(t *testing.T, a *App) { seedUnreads(t, a) },
			check: func(t *testing.T, a *App, msgs []tea.Msg) {
				if !slices.ContainsFunc(msgs, func(m tea.Msg) bool { s, ok := m.(ChannelSelectedMsg); return ok && s.ID == "C4" }) {
					t.Errorf("messages %v, want C4 selected", msgs)
				}
			}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			a := newTestApp(t, unreadOpts()...)
			withLeaderMap(t, a, map[string][]string{tc.action: {"x"}})
			if tc.setup != nil {
				tc.setup(t, a)
			}
			a.Update(keyPress('\\'))
			if !strings.Contains(hint(a), "x "+leaderActions[tc.action].hint) {
				t.Errorf("hint %q does not offer x", hint(a))
			}
			_, cmd := a.Update(keyPress('x'))
			tc.check(t, a, drainBatch(cmd))
		})
	}
}
