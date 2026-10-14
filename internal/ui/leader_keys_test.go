// internal/ui/leader_keys_test.go
//
// Leader-key dispatch (leader-keys design §3, §4, §6): the leader layer
// from the keyseq table, its which-key hint, cancellation, configured
// leaders, and the leader staying out of every mode but Normal.
package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/help"
	"github.com/gammons/slk/internal/ui/themeswitcher"
	"github.com/gammons/slk/internal/ui/wintree"
)

// dispatchKeys sends keys through dispatchModeKey in order and returns
// the last one's command: the leading keys of a sequence, typically.
func dispatchKeys(a *App, keys ...tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		cmd = dispatchModeKey(a, k)
	}
	return cmd
}

// leaderKey is the default leader's key message.
var leaderKey = keyPress('\\')

// armLeader presses the leader (plus any further keys) as a case setup,
// failing the case if no sequence is left pending.
func armLeader(more ...tea.KeyMsg) func(*testing.T, *App) {
	return func(t *testing.T, a *App) {
		t.Helper()
		_ = dispatchKeys(a, append([]tea.KeyMsg{leaderKey}, more...)...)
		if !a.pendingSeq.Pending() {
			t.Fatal("precondition: the leader did not arm a sequence")
		}
	}
}

// seqDone asserts the sequence is over and the resting hint is back.
func seqDone(t *testing.T, a *App) {
	t.Helper()
	if a.pendingSeq.Pending() {
		t.Error("sequence still pending")
	}
	if got := statusbarText(a); !strings.Contains(got, "? for keybindings") {
		t.Errorf("statusbar = %q, want the resting help hint back", got)
	}
}

func TestLeaderKeys(t *testing.T) {
	runKeyCases(t, ModeNormal, []keyCase{
		{
			name:     `\ arms the leader layer and lists it in the status bar`,
			opts:     normalOpts(),
			key:      leaderKey,
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, cmd tea.Cmd) {
				if !a.pendingSeq.Pending() {
					t.Fatal("leader did not arm a sequence")
				}
				hint := stripANSI(a.statusbar.View(200))
				for _, want := range []string{`\ …`, "w window", "t channel", "f search", "] thread"} {
					if !strings.Contains(hint, want) {
						t.Errorf("hint %q lacks %q", hint, want)
					}
				}
				if cmd != nil {
					t.Errorf("cmd = %T, want nil", cmd)
				}
			},
		},
		{
			name:     `\ t opens the channel finder`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      keyPress('t'),
			wantMode: ModeChannelFinder,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if !a.channelFinder.IsVisible() {
					t.Error("channel finder not visible")
				}
				if a.pendingSeq.Pending() {
					t.Error("sequence still pending")
				}
			},
		},
		{
			name:     `\ f opens workspace search`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      keyPress('f'),
			wantMode: ModeWorkspaceSearch,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if !a.searchResults.IsVisible() {
					t.Error("workspace search not visible")
				}
			},
		},
		{
			name:     `\ a opens the Activity view`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      keyPress('a'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, cmd tea.Cmd) {
				seqDone(t, a)
				if cmd == nil {
					t.Fatal("cmd = nil, want ActivityViewActivatedMsg")
				}
				if _, ok := cmd().(ActivityViewActivatedMsg); !ok {
					t.Errorf("cmd() = %T, want ActivityViewActivatedMsg", cmd())
				}
			},
		},
		{
			name:     `\ n starts a new message`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      keyPress('n'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, cmd tea.Cmd) {
				if cmd == nil {
					t.Fatal("cmd = nil, want EnterNewMessageMsg")
				}
				if _, ok := cmd().(EnterNewMessageMsg); !ok {
					t.Errorf("cmd() = %T, want EnterNewMessageMsg", cmd())
				}
			},
		},
		{
			name:     `\ s opens the presence menu`,
			opts:     activeTeamOpts(),
			setup:    armLeader(),
			key:      keyPress('s'),
			wantMode: ModePresenceMenu,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if !a.presenceMenu.IsVisible() {
					t.Error("presence menu not visible")
				}
			},
		},
		{
			name:     `\ y opens the per-workspace theme switcher`,
			opts:     activeTeamOpts(),
			setup:    armLeader(),
			key:      keyPress('y'),
			wantMode: ModeThemeSwitcher,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if got := a.themeSwitcher.Scope(); got != themeswitcher.ScopeWorkspace {
					t.Errorf("scope = %v, want ScopeWorkspace", got)
				}
				if got := a.themeSwitcher.HeaderText(); got != "Theme for alpha" {
					t.Errorf("header = %q, want %q", got, "Theme for alpha")
				}
			},
		},
		{
			name:     `\ Y opens the default-theme switcher`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      keyPress('Y'),
			wantMode: ModeThemeSwitcher,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if got := a.themeSwitcher.Scope(); got != themeswitcher.ScopeGlobal {
					t.Errorf("scope = %v, want ScopeGlobal", got)
				}
			},
		},
		{
			name:     `\ b toggles the sidebar`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      keyPress('b'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if a.sidebarVisible {
					t.Error("sidebar still visible")
				}
				seqDone(t, a)
			},
		},
		{
			name: `\ ] closes the thread`,
			opts: normalOpts(),
			setup: func(t *testing.T, a *App) {
				focusThreadPanel(t, a)
				armLeader()(t, a)
			},
			key:      keyPress(']'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if a.threadVisible {
					t.Error("thread still visible")
				}
			},
		},
		{
			name:     `\ w arms the window commands and lists them`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      keyPress('w'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if !a.pendingSeq.Pending() {
					t.Fatal("w did not arm the window commands")
				}
				hint := stripANSI(a.statusbar.View(200))
				for _, want := range []string{`\ w …`, "h/j/k/l focus", "s split", "q/c close", "o only"} {
					if !strings.Contains(hint, want) {
						t.Errorf("hint %q lacks %q", hint, want)
					}
				}
			},
		},
		{
			name:     `\ w v splits like ctrl+w v`,
			opts:     []testOpt{withSize(200, 50)},
			setup:    armLeader(keyPress('w')),
			key:      keyPress('v'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if got := a.wins.Len(); got != 2 {
					t.Errorf("window count = %d, want 2", got)
				}
				seqDone(t, a)
			},
		},
		{
			name:     `\ w q closes the focused window like ctrl+w q`,
			opts:     []testOpt{withSize(200, 50), withWindowSplit(wintree.SplitSideBySide)},
			setup:    armLeader(keyPress('w')),
			key:      keyPress('q'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if got := a.wins.Len(); got != 1 {
					t.Errorf("window count = %d, want 1", got)
				}
			},
		},
		{
			// Esc must not reach the Escape arm, which would close the
			// thread.
			name: "esc cancels the leader silently",
			opts: normalOpts(),
			setup: func(t *testing.T, a *App) {
				focusThreadPanel(t, a)
				armLeader()(t, a)
			},
			key:      keyCode(tea.KeyEscape),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, cmd tea.Cmd) {
				seqDone(t, a)
				if !a.threadVisible {
					t.Error("esc after the leader closed the thread")
				}
				if cmd != nil {
					t.Errorf("cmd = %T, want nil", cmd)
				}
			},
		},
		{
			// An unbound key is not processed on its own: q would close
			// the thread, j would move the selection.
			name: "an unbound key cancels the leader without doing its own job",
			opts: normalOpts(),
			setup: func(t *testing.T, a *App) {
				focusThreadPanel(t, a)
				armLeader()(t, a)
			},
			key:      keyPress('q'),
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, cmd tea.Cmd) {
				seqDone(t, a)
				if !a.threadVisible {
					t.Error("q after the leader closed the thread")
				}
				if cmd != nil {
					t.Errorf("cmd = %T, want nil", cmd)
				}
				// The cancel spent the leader: q now does its own job.
				_ = dispatchModeKey(a, keyPress('q'))
				if a.threadVisible {
					t.Error("q after a cancelled sequence did not close the thread")
				}
			},
		},
		{
			name:     `\ \ does nothing when the leader displaces no binding`,
			opts:     normalOpts(),
			setup:    armLeader(),
			key:      leaderKey,
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				seqDone(t, a)
			},
		},
		{
			name: "a mode change resets a pending leader",
			opts: normalOpts(),
			setup: func(t *testing.T, a *App) {
				armLeader()(t, a)
				a.SetMode(ModeConfirm) // e.g. the global ctrl+c prompt
				a.SetMode(ModeNormal)
			},
			key:      keyPress('t'), // must not complete \ t
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if a.channelFinder.IsVisible() {
					t.Error("t completed a sequence the mode change should have reset")
				}
				seqDone(t, a)
			},
		},
		{
			name: "a pending gg hands the leader on, which arms",
			opts: normalOpts(),
			setup: func(t *testing.T, a *App) {
				_ = dispatchModeKey(a, keyPress('g'))
				if !a.pendingTop {
					t.Fatal("precondition: g did not arm gg")
				}
			},
			key:      leaderKey,
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if a.pendingTop || !a.pendingSeq.Pending() {
					t.Errorf("pendingTop = %v, sequence pending = %v; want gg cancelled and the leader armed",
						a.pendingTop, a.pendingSeq.Pending())
				}
			},
		},
		{
			name: "reaction navigation swallows the leader",
			opts: normalOpts(),
			setup: func(t *testing.T, a *App) {
				focusThreadPanel(t, a)
				a.threadPanel.EnterReactionNav()
				if !a.threadPanel.ReactionNavActive() {
					t.Fatal("precondition: reaction nav not active")
				}
			},
			key:      leaderKey,
			wantMode: ModeNormal,
			assert: func(t *testing.T, a *App, _ tea.Cmd) {
				if a.pendingSeq.Pending() {
					t.Error("the leader armed inside reaction navigation")
				}
			},
		},
	})
}

// The leader belongs to Normal mode: everywhere else the key is just a
// key. Driven through the real Update chain.
func TestLeaderIgnoredOutsideNormalMode(t *testing.T) {
	t.Run("insert mode types it", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.Update(keyPress('i'))
		if a.mode != ModeInsert {
			t.Fatalf("precondition: mode = %v, want insert", a.mode)
		}
		a.Update(leaderKey)
		a.Update(keyPress('t'))
		if a.pendingSeq.Pending() || a.mode != ModeInsert || a.channelFinder.IsVisible() {
			t.Fatalf("pending = %v, mode = %v, finder = %v; want the keys typed",
				a.pendingSeq.Pending(), a.mode, a.channelFinder.IsVisible())
		}
		if got := a.compose.Value(); got != `\t` {
			t.Errorf("compose = %q, want %q", got, `\t`)
		}
	})
	t.Run("a modal with text input filters on it", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.Update(keyMod('t', tea.ModCtrl))
		if a.mode != ModeChannelFinder {
			t.Fatalf("precondition: mode = %v, want the channel finder", a.mode)
		}
		a.Update(leaderKey)
		a.Update(keyPress('f'))
		if a.pendingSeq.Pending() || a.mode != ModeChannelFinder {
			t.Fatalf("pending = %v, mode = %v; want the finder still open", a.pendingSeq.Pending(), a.mode)
		}
		if got := a.channelFinder.Query(); got != `\f` {
			t.Errorf("finder query = %q, want %q", got, `\f`)
		}
	})
	t.Run("the command prompt takes it", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.Update(keyPress(':'))
		a.Update(leaderKey)
		if a.pendingSeq.Pending() || a.cmdline != `\` {
			t.Fatalf("pending = %v, cmdline = %q; want the key on the prompt", a.pendingSeq.Pending(), a.cmdline)
		}
	})
}

// A configured leader replaces the default; the displaced binding comes
// back as <leader><leader>.
func TestConfiguredLeader(t *testing.T) {
	t.Run("space leads and space space toggles the section", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.SetLeader("space")
		a.focusedPanel = PanelSidebar
		section := selectSidebarSectionHeader(t, a)
		collapsed := a.sidebar.IsCollapsed(section)

		_ = dispatchModeKey(a, keyPress(' '))
		if !a.pendingSeq.Pending() {
			t.Fatal("space did not arm the leader")
		}
		if a.sidebar.IsCollapsed(section) != collapsed {
			t.Fatal("space as the leader toggled the section on its own")
		}
		if hint := stripANSI(a.statusbar.View(200)); !strings.Contains(hint, "space …  space toggle section  w window") {
			t.Errorf("hint %q does not offer the displaced action first", hint)
		}
		_ = dispatchModeKey(a, keyPress(' '))
		if a.sidebar.IsCollapsed(section) == collapsed {
			t.Fatal("space space did not toggle the section")
		}
		seqDone(t, a)

		// Enter on a header still toggles it too.
		section = selectSidebarSectionHeader(t, a)
		collapsed = a.sidebar.IsCollapsed(section)
		_ = dispatchModeKey(a, keyCode(tea.KeyEnter))
		if a.sidebar.IsCollapsed(section) == collapsed {
			t.Fatal("enter on a section header no longer toggles it")
		}
	})
	t.Run("space t opens the channel finder", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.SetLeader("space")
		_ = dispatchKeys(a, keyPress(' '), keyPress('t'))
		if a.mode != ModeChannelFinder {
			t.Fatalf("mode = %v, want the channel finder", a.mode)
		}
	})
	t.Run("the default leader is a plain key again", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.SetLeader("space")
		_ = dispatchModeKey(a, leaderKey)
		if a.pendingSeq.Pending() {
			t.Fatal(`\ still arms a sequence after the leader moved to space`)
		}
	})
	t.Run("the displaced binding wins over a layer letter", func(t *testing.T) {
		// b is RespondButtons and the layer's sidebar toggle: b b is
		// the former, so the sidebar stays; the layer still works.
		a := newTestApp(t, normalOpts()...)
		a.SetLeader("b")
		_ = dispatchModeKey(a, keyPress('b'))
		hint := stripANSI(a.statusbar.View(200))
		if !strings.Contains(hint, "b …  b respond to message buttons  w window") {
			t.Errorf("hint %q does not offer the displaced action first", hint)
		}
		// The layer's own b (sidebar) can never be reached: b b is
		// taken, so neither the hint nor the help may offer it.
		if strings.Contains(hint, "b sidebar") {
			t.Errorf("hint %q offers the unreachable layer entry", hint)
		}
		if rows := a.helpEntries(); slices.Contains(rows, help.Entry{Key: "b b", Desc: "toggle sidebar"}) {
			t.Error(`help lists the unreachable "b b  toggle sidebar"`)
		}
		_ = dispatchModeKey(a, keyPress('b'))
		if !a.sidebarVisible {
			t.Fatal("b b ran the layer's sidebar toggle, not the displaced binding")
		}
		seqDone(t, a)
		_ = dispatchKeys(a, keyPress('b'), keyPress('t'))
		if a.mode != ModeChannelFinder {
			t.Fatalf("b t: mode = %v, want the channel finder", a.mode)
		}
	})
	t.Run("q q closes the thread", func(t *testing.T) {
		a := newTestApp(t, normalOpts()...)
		a.SetLeader("q")
		focusThreadPanel(t, a)
		_ = dispatchModeKey(a, keyPress('q'))
		if !a.threadVisible {
			t.Fatal("q as the leader closed the thread on its own")
		}
		_ = dispatchModeKey(a, keyPress('q'))
		if a.threadVisible {
			t.Fatal("q q did not close the thread")
		}
	})
	t.Run("ctrl+w as the leader keeps the window commands on w", func(t *testing.T) {
		a := newTestApp(t, withSize(200, 50))
		a.SetLeader("ctrl+w")
		ctrlW := keyMod('w', tea.ModCtrl)
		_ = dispatchModeKey(a, ctrlW)
		if hint := stripANSI(a.statusbar.View(200)); !strings.Contains(hint, "ctrl+w …  w window") {
			t.Fatalf("hint %q: ctrl+w should arm the leader layer", hint)
		}
		_ = dispatchKeys(a, keyPress('w'), keyPress('v'))
		if got := a.wins.Len(); got != 2 {
			t.Fatalf("ctrl+w w v: window count = %d, want 2", got)
		}
	})
}

func TestDisplacedBindings(t *testing.T) {
	km := DefaultKeyMap()
	for _, tc := range []struct {
		key       string
		wantDescs []string // nil = nothing displaced
	}{
		{`\`, nil},
		{"space", []string{"toggle section"}},
		{"q", []string{"close thread view"}},
		{"ctrl+t", []string{"switch channel"}},
		{"l", []string{"right"}},
		{"ctrl+w", nil}, // a sequence prefix, reachable as <leader>w
		{"ctrl+o", nil}, // insert mode only
		{"ctrl+e", nil}, // insert mode only
	} {
		var got []string
		for _, b := range displacedBindings(km, tc.key) {
			got = append(got, b.Help().Desc)
		}
		if !slices.Equal(got, tc.wantDescs) {
			t.Errorf("displacedBindings(%q) = %q, want %q", tc.key, got, tc.wantDescs)
		}
	}
	// notDisplaceable names KeyMap fields; a renamed field would
	// silently drop out of it.
	for _, name := range notDisplaceable {
		if _, ok := reflect.TypeOf(km).FieldByName(name); !ok {
			t.Errorf("notDisplaceable names %q, which is not a KeyMap field", name)
		}
	}
}

func TestHelpEntriesFollowTheLeader(t *testing.T) {
	cases := []struct {
		leader string
		want   []help.Entry
		absent []help.Entry
	}{
		{
			leader: "",
			want: []help.Entry{
				{Key: `\`, Desc: leaderHelpDesc},
				{Key: `\ t`, Desc: "switch channel"},
				{Key: `\ w`, Desc: "window commands"},
				{Key: `\ w h/j/k/l`, Desc: "focus pane in direction"},
				{Key: `\ w q/c`, Desc: "close window"},
				{Key: `\ Y`, Desc: "set default theme"},
				{Key: "ctrl+w h/j/k/l", Desc: "focus pane in direction"},
			},
		},
		{
			// space's own row is gone: space now leads, and the layer's
			// "space space" row says what toggles the section.
			leader: "space",
			want: []help.Entry{
				{Key: "space", Desc: leaderHelpDesc},
				{Key: "space space", Desc: "toggle section"},
				{Key: "space t", Desc: "switch channel"},
				{Key: "space w s", Desc: "split window"},
			},
			absent: []help.Entry{
				{Key: `\ t`, Desc: "switch channel"},
				{Key: "space", Desc: "toggle section"},
			},
		},
		{
			// The ctrl+w chords move under ctrl+w w; ctrl+w s is the
			// layer's status menu now.
			leader: "ctrl+w",
			want: []help.Entry{
				{Key: "ctrl+w", Desc: leaderHelpDesc},
				{Key: "ctrl+w w", Desc: "window commands"},
				{Key: "ctrl+w w s", Desc: "split window"},
				{Key: "ctrl+w w h/j/k/l", Desc: "focus pane in direction"},
				{Key: "ctrl+w s", Desc: "set status"},
			},
			absent: []help.Entry{
				{Key: "ctrl+w", Desc: "window commands"},
				{Key: "ctrl+w s / :sp", Desc: "split window"},
				{Key: "ctrl+w h/j/k/l", Desc: "focus pane in direction"},
				{Key: "ctrl+w q / :q", Desc: "close window"},
				{Key: "ctrl+w o / :only", Desc: "close other windows"},
				{Key: "ctrl+w w", Desc: "cycle windows"},
			},
		},
		{
			// A binding with another key keeps a row for it.
			leader: "Y",
			want: []help.Entry{
				{Key: "C", Desc: "copy permalink"},
				{Key: "Y Y", Desc: "copy permalink"},
			},
			absent: []help.Entry{{Key: "Y/C", Desc: "copy permalink"}},
		},
		{
			// Insert mode's ctrl+o is untouched by a Normal-mode leader.
			leader: "ctrl+o",
			want: []help.Entry{
				{Key: "ctrl+o / alt+enter", Desc: "also send reply to channel"},
				{Key: "ctrl+o t", Desc: "switch channel"},
			},
		},
	}
	for _, tc := range cases {
		t.Run("leader "+tc.leader, func(t *testing.T) {
			a := newTestApp(t)
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
			// A leader row sorts next to the chord it mirrors.
			i := slices.Index(got, help.Entry{Key: "ctrl+t", Desc: "switch channel"})
			if i < 0 || i+3 > len(got) || !slices.ContainsFunc(got[i:i+3], func(e help.Entry) bool {
				return strings.HasSuffix(e.Key, " t") && e.Desc == "switch channel"
			}) {
				t.Errorf("the leader's switch-channel row is not next to ctrl+t's")
			}
		})
	}
}
