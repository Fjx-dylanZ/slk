// internal/ui/keyactions_test.go
//
// Configurable, panel-scoped actions (leader-keys design §8): the
// sidebar's sidebar_open (Enter) and sidebar_toggle_section (space),
// their defaults, configured keys, unbinding, the leader taking one of
// their keys, and the help overlay.
package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/help"
)

// neovimSidebar is the nvim-tree style configuration: l opens a row (or
// expands / collapses a section header), Enter too.
var neovimSidebar = map[string][]string{"sidebar_open": {"enter", "l"}}

// withKeyBindings applies b as a case setup, then runs then (if any).
func withKeyBindings(b map[string][]string, then func(*testing.T, *App)) func(*testing.T, *App) {
	return func(t *testing.T, a *App) {
		t.Helper()
		a.SetKeyBindings(b)
		if then != nil {
			then(t, a)
		}
	}
}

// onSidebarHeader focuses the sidebar with its cursor on a section
// header, recording the header and the collapse state a toggle would
// produce.
func onSidebarHeader(section *string, wantCollapsed *bool) func(*testing.T, *App) {
	return func(t *testing.T, a *App) {
		t.Helper()
		a.focusedPanel = PanelSidebar
		*section = selectSidebarSectionHeader(t, a)
		*wantCollapsed = !a.sidebar.IsCollapsed(*section)
	}
}

// onSidebarChannel focuses the sidebar with its cursor on channel id.
func onSidebarChannel(id string) func(*testing.T, *App) {
	return func(t *testing.T, a *App) {
		t.Helper()
		a.focusedPanel = PanelSidebar
		a.sidebar.SelectByID(id)
		if got := a.sidebar.SelectedID(); got != id {
			t.Fatalf("precondition: sidebar SelectedID = %q, want %q", got, id)
		}
		if _, ok := a.sidebar.IsSectionHeaderSelected(); ok {
			t.Fatal("precondition: cursor is on a header, not a channel")
		}
	}
}

// wantChannelSelected asserts cmd selects channel id.
func wantChannelSelected(t *testing.T, cmd tea.Cmd, id string) {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd = nil, want a ChannelSelectedMsg")
	}
	if msg, ok := cmd().(ChannelSelectedMsg); !ok || msg.ID != id {
		t.Fatalf("cmd() = %#v, want ChannelSelectedMsg for %s", cmd(), id)
	}
}

func TestSidebarKeyActions(t *testing.T) {
	var section string
	var wantCollapsed bool
	toggled := func(t *testing.T, a *App, cmd tea.Cmd) {
		t.Helper()
		if got := a.sidebar.IsCollapsed(section); got != wantCollapsed {
			t.Errorf("section %q collapsed = %v, want %v", section, got, wantCollapsed)
		}
		if cmd != nil {
			t.Errorf("cmd = %#v, want nil", cmd())
		}
		if a.focusedPanel != PanelSidebar {
			t.Errorf("focused panel = %v, want the sidebar", a.focusedPanel)
		}
	}
	untouched := func(t *testing.T, a *App, cmd tea.Cmd) {
		t.Helper()
		if got := a.sidebar.IsCollapsed(section); got == wantCollapsed {
			t.Errorf("section %q collapsed = %v: the key toggled it", section, got)
		}
		if cmd != nil {
			t.Errorf("cmd = %#v, want nil", cmd())
		}
		if a.focusedPanel != PanelSidebar {
			t.Errorf("focused panel = %v, want the sidebar", a.focusedPanel)
		}
	}
	focusedNext := func(want Panel) func(*testing.T, *App, tea.Cmd) {
		return func(t *testing.T, a *App, cmd tea.Cmd) {
			t.Helper()
			if a.focusedPanel != want {
				t.Errorf("focused panel = %v, want %v (FocusNext)", a.focusedPanel, want)
			}
			if cmd != nil {
				t.Errorf("cmd = %#v, want nil", cmd())
			}
		}
	}

	runKeyCases(t, ModeNormal, []keyCase{
		// Defaults: Enter opens, space toggles, l moves focus.
		{
			name:     "default: l in the sidebar moves focus to the messages",
			opts:     normalOpts(),
			setup:    onSidebarChannel("C2"),
			key:      keyPress('l'),
			wantMode: ModeNormal,
			assert:   focusedNext(PanelMessages),
		},
		{
			name:     "default: space toggles a section header",
			opts:     normalOpts(),
			setup:    onSidebarHeader(&section, &wantCollapsed),
			key:      keyPress(' '),
			wantMode: ModeNormal,
			assert:   toggled,
		},
		{
			name:     "default: enter opens a channel",
			opts:     normalOpts(),
			setup:    onSidebarChannel("C2"),
			key:      keyCode(tea.KeyEnter),
			wantMode: ModeNormal,
			assert:   func(t *testing.T, _ *App, cmd tea.Cmd) { wantChannelSelected(t, cmd, "C2") },
		},

		// sidebar_open = ["enter", "l"].
		{
			name:     "neovim: l on a section header toggles it",
			opts:     normalOpts(),
			setup:    withKeyBindings(neovimSidebar, onSidebarHeader(&section, &wantCollapsed)),
			key:      keyPress('l'),
			wantMode: ModeNormal,
			assert:   toggled,
		},
		{
			name:     "neovim: l on a channel opens it",
			opts:     normalOpts(),
			setup:    withKeyBindings(neovimSidebar, onSidebarChannel("C2")),
			key:      keyPress('l'),
			wantMode: ModeNormal,
			assert:   func(t *testing.T, _ *App, cmd tea.Cmd) { wantChannelSelected(t, cmd, "C2") },
		},
		{
			name: "neovim: l on the Threads row opens the Threads view",
			opts: normalOpts(),
			setup: withKeyBindings(neovimSidebar, func(t *testing.T, a *App) {
				a.focusedPanel = PanelSidebar
				a.sidebar.GoToTop()
				if !a.sidebar.IsThreadsSelected() {
					t.Fatal("precondition: the Threads row is not selected")
				}
			}),
			key:      keyPress('l'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, _ *App, cmd tea.Cmd) {
				if cmd == nil {
					t.Fatal("cmd = nil, want ThreadsViewActivatedMsg")
				}
				if _, ok := cmd().(ThreadsViewActivatedMsg); !ok {
					t.Errorf("cmd() = %T, want ThreadsViewActivatedMsg", cmd())
				}
			},
		},
		{
			name: "neovim: l on the Activity row opens the Activity view",
			opts: normalOpts(),
			setup: withKeyBindings(neovimSidebar, func(t *testing.T, a *App) {
				a.focusedPanel = PanelSidebar
				a.sidebar.SelectActivityRow()
				if !a.sidebar.IsActivitySelected() {
					t.Fatal("precondition: the Activity row is not selected")
				}
			}),
			key:      keyPress('l'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, _ *App, cmd tea.Cmd) {
				if cmd == nil {
					t.Fatal("cmd = nil, want ActivityViewActivatedMsg")
				}
				if _, ok := cmd().(ActivityViewActivatedMsg); !ok {
					t.Errorf("cmd() = %T, want ActivityViewActivatedMsg", cmd())
				}
			},
		},
		{
			name:     "neovim: enter still opens",
			opts:     normalOpts(),
			setup:    withKeyBindings(neovimSidebar, onSidebarChannel("C2")),
			key:      keyCode(tea.KeyEnter),
			wantMode: ModeNormal,
			assert:   func(t *testing.T, _ *App, cmd tea.Cmd) { wantChannelSelected(t, cmd, "C2") },
		},
		{
			name:     "neovim: l in the messages pane still moves focus",
			opts:     normalOpts(),
			setup:    withKeyBindings(neovimSidebar, func(t *testing.T, a *App) { focusMessages(t, a) }),
			key:      keyPress('l'),
			wantMode: ModeNormal,
			assert:   focusedNext(PanelSidebar),
		},
		{
			name: "neovim: enter in the messages pane still opens the thread",
			opts: normalOpts(),
			setup: withKeyBindings(neovimSidebar, func(t *testing.T, a *App) {
				focusMessages(t, a)
			}),
			key:      keyCode(tea.KeyEnter),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if !a.threadVisible {
					t.Error("enter in the messages pane did not open the thread")
				}
			},
		},

		// sidebar_open without enter: Enter in the sidebar is free.
		{
			name:     "sidebar_open = [l]: enter on a header does nothing",
			opts:     normalOpts(),
			setup:    withKeyBindings(map[string][]string{"sidebar_open": {"l"}}, onSidebarHeader(&section, &wantCollapsed)),
			key:      keyCode(tea.KeyEnter),
			wantMode: ModeNormal,
			assert:   untouched,
		},
		{
			name:     "sidebar_open = [l]: enter on a channel does nothing",
			opts:     normalOpts(),
			setup:    withKeyBindings(map[string][]string{"sidebar_open": {"l"}}, onSidebarChannel("C2")),
			key:      keyCode(tea.KeyEnter),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, cmd tea.Cmd) {
				if cmd != nil {
					t.Errorf("cmd = %#v, want nil", cmd())
				}
			},
		},

		// sidebar_toggle_section.
		{
			name:     "sidebar_toggle_section = []: space is free",
			opts:     normalOpts(),
			setup:    withKeyBindings(map[string][]string{"sidebar_toggle_section": {}}, onSidebarHeader(&section, &wantCollapsed)),
			key:      keyPress(' '),
			wantMode: ModeNormal,
			assert:   untouched,
		},
		{
			name:     "sidebar_toggle_section = []: enter still toggles a header",
			opts:     normalOpts(),
			setup:    withKeyBindings(map[string][]string{"sidebar_toggle_section": {}}, onSidebarHeader(&section, &wantCollapsed)),
			key:      keyCode(tea.KeyEnter),
			wantMode: ModeNormal,
			assert:   toggled,
		},
		{
			name:     "sidebar_toggle_section = [o]: o toggles a header",
			opts:     normalOpts(),
			setup:    withKeyBindings(map[string][]string{"sidebar_toggle_section": {"o"}}, onSidebarHeader(&section, &wantCollapsed)),
			key:      keyPress('o'),
			wantMode: ModeNormal,
			assert:   toggled,
		},
		{
			name:     "sidebar_toggle_section = [o]: space no longer toggles",
			opts:     normalOpts(),
			setup:    withKeyBindings(map[string][]string{"sidebar_toggle_section": {"o"}}, onSidebarHeader(&section, &wantCollapsed)),
			key:      keyPress(' '),
			wantMode: ModeNormal,
			assert:   untouched,
		},
		{
			// Outside the sidebar o is open-link again.
			name: "sidebar_toggle_section = [o]: o in the messages pane is not the sidebar's",
			opts: normalOpts(),
			setup: withKeyBindings(map[string][]string{"sidebar_toggle_section": {"o"}}, func(t *testing.T, a *App) {
				focusMessages(t, a)
				section = selectSidebarSectionHeader(t, a)
				wantCollapsed = !a.sidebar.IsCollapsed(section)
				a.focusedPanel = PanelMessages
			}),
			key:      keyPress('o'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if a.sidebar.IsCollapsed(section) == wantCollapsed {
					t.Error("o in the messages pane toggled a sidebar section")
				}
			},
		},
	})
}

// Opening a channel with a configured key goes through the real Update
// chain: the selection lands and focus moves to the messages pane.
func TestSidebarOpenKeyMovesFocusThroughUpdate(t *testing.T) {
	a := newTestApp(t, normalOpts()...)
	a.SetKeyBindings(neovimSidebar)
	onSidebarChannel("C2")(t, a)
	_, cmd := a.Update(keyPress('l'))
	if cmd == nil {
		t.Fatal("l on a channel returned no command")
	}
	msg, ok := cmd().(ChannelSelectedMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want ChannelSelectedMsg", cmd())
	}
	a.Update(msg)
	if a.activeChannelID != "C2" || a.focusedPanel != PanelMessages {
		t.Fatalf("active channel %q, focused %v; want C2 open and the messages pane focused", a.activeChannelID, a.focusedPanel)
	}
}

// The leader wins a sidebar key, and <leader><leader> reaches the
// sidebar action on whichever key it is configured, not on space.
func TestLeaderDisplacesConfiguredSidebarKeys(t *testing.T) {
	t.Run("leader x, sidebar_toggle_section [x]: x x toggles", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.SetKeyBindings(map[string][]string{"sidebar_toggle_section": {"x"}})
		a.SetLeader("x")
		var section string
		var want bool
		onSidebarHeader(&section, &want)(t, a)
		_ = dispatchModeKey(a, keyPress('x'))
		if hint := stripANSI(a.statusbar.View(200)); !strings.Contains(hint, "x …  x toggle section  w +windows") {
			t.Errorf("hint %q does not offer the displaced toggle", hint)
		}
		if a.sidebar.IsCollapsed(section) == want {
			t.Fatal("x as the leader toggled the section on its own")
		}
		_ = dispatchModeKey(a, keyPress('x'))
		if a.sidebar.IsCollapsed(section) != want {
			t.Fatal("x x did not toggle the section")
		}
	})
	t.Run("leader space, toggle moved to o: space space does nothing", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.SetKeyBindings(map[string][]string{"sidebar_toggle_section": {"o"}})
		a.SetLeader("space")
		var section string
		var want bool
		onSidebarHeader(&section, &want)(t, a)
		_ = dispatchModeKey(a, keyPress(' '))
		if hint := stripANSI(a.statusbar.View(200)); strings.Contains(hint, "toggle section") {
			t.Errorf("hint %q still offers a toggle on space", hint)
		}
		_ = dispatchModeKey(a, keyPress(' '))
		if a.sidebar.IsCollapsed(section) == want {
			t.Fatal("space space toggled a section that is no longer on space")
		}
		seqDone(t, a)
	})
	t.Run("leader l, sidebar_open [enter l]: l l opens in the sidebar, moves focus elsewhere", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.SetKeyBindings(neovimSidebar)
		a.SetLeader("l")
		onSidebarChannel("C2")(t, a)
		_ = dispatchModeKey(a, keyPress('l'))
		if hint := stripANSI(a.statusbar.View(200)); !strings.Contains(hint, "l …  l right; open row / toggle section (sidebar)") {
			t.Errorf("hint %q does not name both actions of l", hint)
		}
		wantChannelSelected(t, dispatchModeKey(a, keyPress('l')), "C2")

		focusMessages(t, a)
		_ = dispatchKeys(a, keyPress('l'), keyPress('l'))
		if a.focusedPanel != PanelSidebar {
			t.Fatalf("l l in the messages pane: focused %v, want the sidebar (focus right)", a.focusedPanel)
		}
	})
}

// The help overlay shows each sidebar action under its configured keys,
// and drops an unbound one.
func TestHelpFollowsSidebarKeys(t *testing.T) {
	open := help.Entry{Key: "enter", Desc: "open row / toggle section (sidebar)"}
	toggle := help.Entry{Key: "space", Desc: "toggle section"}
	for _, tc := range []struct {
		name     string
		bindings map[string][]string
		leader   string
		want     []help.Entry
		absent   []help.Entry
	}{
		{name: "defaults", want: []help.Entry{open, toggle}},
		{
			name:     "neovim",
			bindings: neovimSidebar,
			want:     []help.Entry{{Key: "enter/l", Desc: open.Desc}, toggle},
			absent:   []help.Entry{open},
		},
		{
			name:     "toggle unbound",
			bindings: map[string][]string{"sidebar_toggle_section": {}},
			want:     []help.Entry{open},
			absent:   []help.Entry{toggle},
		},
		{
			name:     "toggle on x, leader x",
			bindings: map[string][]string{"sidebar_toggle_section": {"x", "o"}},
			leader:   "x",
			want:     []help.Entry{{Key: "o", Desc: "toggle section"}, {Key: "x x", Desc: "toggle section"}},
			absent:   []help.Entry{toggle, {Key: "x/o", Desc: "toggle section"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t)
			a.SetKeyBindings(tc.bindings)
			a.SetLeader(tc.leader)
			got := a.helpEntries()
			for _, e := range tc.want {
				if !slices.Contains(got, e) {
					t.Errorf("help lacks %+v", e)
				}
			}
			for _, e := range tc.absent {
				if slices.Contains(got, e) {
					t.Errorf("help still lists %+v", e)
				}
			}
		})
	}
}

// An action left out of the map keeps its keys; unknown names are
// ignored.
func TestSetKeyBindingsLeavesOthersAlone(t *testing.T) {
	a := newTestApp(t)
	a.SetKeyBindings(map[string][]string{"sidebar_open": {"l"}, "no_such_action": {"z"}})
	if got := a.keys.ToggleSection.Keys(); !slices.Equal(got, []string{"space"}) {
		t.Errorf("ToggleSection keys = %q, want [space]", got)
	}
	if got := a.keys.SidebarOpen.Keys(); !slices.Equal(got, []string{"l"}) {
		t.Errorf("SidebarOpen keys = %q, want [l]", got)
	}
	if got := KeyActionNames(); !slices.Equal(got, []string{"sidebar_open", "sidebar_toggle_section"}) {
		t.Errorf("KeyActionNames = %q", got)
	}
}
