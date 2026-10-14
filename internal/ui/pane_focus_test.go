// internal/ui/pane_focus_test.go
//
// Directional pane focus (leader-keys design §5): h/j/k/l after ctrl+w
// or <leader>w move across the sidebar, the message region's windows
// and the thread by geometry. Every case runs through both prefixes.
package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/wintree"
)

// focusWindowAt focuses the i-th window in tree order, failing the
// test if it does not take.
func focusWindowAt(t *testing.T, a *App, i int) {
	t.Helper()
	_ = a.focusWindow(a.wins.Leaves()[i])
	if a.focusedWin != a.wins.Leaves()[i] {
		t.Fatalf("precondition: window %d did not take focus", i)
	}
}

func TestPaneFocusByDirection(t *testing.T) {
	type paneCase struct {
		name  string
		opts  []testOpt
		setup func(t *testing.T, a *App)
		dir   rune
		// wantPanel is the focused panel after the move. wantWin is
		// the focused window's index in the tree order captured after
		// setup (-1: not checked).
		wantPanel  Panel
		wantWin    int
		wantThread bool
	}
	wide := withSize(200, 50)
	// Room for two side-by-side windows next to an open thread.
	wider := withSize(260, 50)
	sidebar := func(t *testing.T, a *App) { a.focusedPanel = PanelSidebar }
	messages := func(t *testing.T, a *App) { a.focusedPanel = PanelMessages }
	thread := func(t *testing.T, a *App) { focusThreadPanel(t, a) }
	threadThenMessages := func(t *testing.T, a *App) { focusThreadPanel(t, a); a.focusedPanel = PanelMessages }
	onWindow := func(i int, then func(*testing.T, *App)) func(*testing.T, *App) {
		return func(t *testing.T, a *App) {
			focusWindowAt(t, a, i)
			then(t, a)
		}
	}
	vsplit := withWindowSplit(wintree.SplitSideBySide)
	split := withWindowSplit(wintree.SplitStacked)

	cases := []paneCase{
		// Sidebar, one window, thread.
		{name: "sidebar l: the message pane", opts: []testOpt{wide}, setup: sidebar, dir: 'l',
			wantPanel: PanelMessages, wantWin: 0},
		{name: "window h: the sidebar", opts: []testOpt{wide}, setup: messages, dir: 'h',
			wantPanel: PanelSidebar, wantWin: 0},
		{name: "sidebar h: nothing, the rail is not a pane", opts: []testOpt{wide}, setup: sidebar, dir: 'h',
			wantPanel: PanelSidebar, wantWin: 0},
		{name: "sidebar j: nothing below", opts: []testOpt{wide}, setup: sidebar, dir: 'j',
			wantPanel: PanelSidebar, wantWin: 0},
		{name: "window l: the thread", opts: []testOpt{wide}, setup: threadThenMessages, dir: 'l',
			wantPanel: PanelThread, wantWin: 0, wantThread: true},
		{name: "thread h: back to the window, thread kept", opts: []testOpt{wide}, setup: thread, dir: 'h',
			wantPanel: PanelMessages, wantWin: 0, wantThread: true},
		{name: "thread l: nothing right of it", opts: []testOpt{wide}, setup: thread, dir: 'l',
			wantPanel: PanelThread, wantWin: 0, wantThread: true},
		{name: "thread k: nothing above", opts: []testOpt{wide}, setup: thread, dir: 'k',
			wantPanel: PanelThread, wantWin: 0, wantThread: true},

		// Split windows.
		{name: "window h: the window to its left, as before", opts: []testOpt{wide, vsplit},
			setup: onWindow(1, messages), dir: 'h', wantPanel: PanelMessages, wantWin: 0},
		{name: "leftmost window h: the sidebar", opts: []testOpt{wide, vsplit},
			setup: onWindow(0, messages), dir: 'h', wantPanel: PanelSidebar, wantWin: 0},
		{name: "window j: the window below", opts: []testOpt{wide, split},
			setup: onWindow(0, messages), dir: 'j', wantPanel: PanelMessages, wantWin: 1},
		{name: "sidebar l: the focused window when it borders the sidebar", opts: []testOpt{wide, split},
			setup: onWindow(1, sidebar), dir: 'l', wantPanel: PanelMessages, wantWin: 1},
		{name: "sidebar l: the bordering window when the focused one does not", opts: []testOpt{wide, vsplit},
			setup: onWindow(1, sidebar), dir: 'l', wantPanel: PanelMessages, wantWin: 0},
		{name: "rightmost window l: the thread", opts: []testOpt{wider, vsplit},
			setup: onWindow(1, threadThenMessages), dir: 'l', wantPanel: PanelThread, wantWin: 1, wantThread: true},
		{name: "thread h: the focused window when it borders the thread", opts: []testOpt{wider, vsplit},
			setup: onWindow(1, thread), dir: 'h', wantPanel: PanelMessages, wantWin: 1, wantThread: true},
		{name: "thread h: another window takes focus and closes the thread (§7)", opts: []testOpt{wider, vsplit},
			setup: onWindow(0, thread), dir: 'h', wantPanel: PanelMessages, wantWin: 1},

		// Layouts and views.
		{name: "hidden sidebar: window h does nothing", opts: []testOpt{wide},
			setup: func(t *testing.T, a *App) { a.ToggleSidebar(); a.focusedPanel = PanelMessages },
			dir:   'h', wantPanel: PanelMessages, wantWin: 0},
		{name: "Threads view: the message region is one pane", opts: []testOpt{wide, vsplit,
			withThreadsView([]cache.ThreadSummary{{ChannelID: "C1", ThreadTS: "1.0", ReplyCount: 2}}),
			withView(ViewThreads)},
			setup: onWindow(1, messages), dir: 'h', wantPanel: PanelSidebar, wantWin: 1},
	}

	prefixes := []struct {
		name string
		keys []tea.KeyMsg
	}{
		{"ctrl+w", []tea.KeyMsg{keyMod('w', tea.ModCtrl)}},
		{`\ w`, []tea.KeyMsg{leaderKey, keyPress('w')}},
	}
	var keyCases []keyCase
	for _, pc := range cases {
		for _, p := range prefixes {
			var leaves []wintree.LeafID
			keyCases = append(keyCases, keyCase{
				name: p.name + " " + string(pc.dir) + ": " + pc.name,
				opts: pc.opts,
				setup: func(t *testing.T, a *App) {
					pc.setup(t, a)
					leaves = a.wins.Leaves()
					_ = dispatchKeys(a, p.keys...)
					if !a.pendingSeq.Pending() {
						t.Fatalf("precondition: %s did not arm the window commands", p.name)
					}
				},
				key:      keyPress(pc.dir),
				wantMode: ModeNormal,
				assert: func(t *testing.T, a *App, _ tea.Cmd) {
					if a.focusedPanel != pc.wantPanel {
						t.Errorf("focused panel = %v, want %v", a.focusedPanel, pc.wantPanel)
					}
					if pc.wantWin >= 0 && a.focusedWin != leaves[pc.wantWin] {
						t.Errorf("focused window = %v, want %v (index %d)", a.focusedWin, leaves[pc.wantWin], pc.wantWin)
					}
					if a.threadVisible != pc.wantThread {
						t.Errorf("thread visible = %v, want %v", a.threadVisible, pc.wantThread)
					}
					if a.pendingSeq.Pending() {
						t.Error("sequence still pending")
					}
				},
			})
		}
	}
	runKeyCases(t, ModeNormal, keyCases)
}

// In the stacked layout the thread keeps its place right of the message
// region: l brings it to the front, h takes the channel back, as Tab
// alternates them.
func TestPaneFocusStackedLayout(t *testing.T) {
	a := newTestApp(t, withSize(100, 40))
	focusThreadPanel(t, a)
	a.focusedPanel = PanelMessages
	if frame := a.computeFrame(); frame.ThreadWidth != 0 || frame.MsgWidth == 0 {
		t.Fatalf("precondition: want the stacked layout with the channel in front, got %+v", frame)
	}

	_ = dispatchKeys(a, leaderKey, keyPress('w'), keyPress('l'))
	if a.focusedPanel != PanelThread || !a.threadDrawnAlone() {
		t.Fatalf("\\ w l: panel = %v, thread alone = %v; want the thread in front", a.focusedPanel, a.threadDrawnAlone())
	}

	_ = dispatchKeys(a, keyMod('w', tea.ModCtrl), keyPress('h'))
	if a.focusedPanel != PanelMessages || a.threadDrawnAlone() || !a.threadVisible {
		t.Fatalf("ctrl+w h: panel = %v, thread alone = %v, open = %v; want the channel in front, thread kept",
			a.focusedPanel, a.threadDrawnAlone(), a.threadVisible)
	}
}

// With the stacked thread in front and focus on the sidebar, the pane
// right of the sidebar on screen is the thread: l enters it rather than
// pulling the hidden channel forward. Driven through Update, which is
// what records the thread as the pane in front.
func TestPaneFocusStackedThreadInFrontFromSidebar(t *testing.T) {
	a := newTestApp(t, withSize(100, 40))
	focusThreadPanel(t, a)
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 40}) // Update records the thread in front
	a.Update(keyCode(tea.KeyTab))                       // thread -> sidebar
	if a.focusedPanel != PanelSidebar || !a.threadDrawnAlone() {
		t.Fatalf("precondition: panel = %v, thread alone = %v; want the sidebar beside the stacked thread",
			a.focusedPanel, a.threadDrawnAlone())
	}
	for _, k := range []tea.KeyMsg{leaderKey, keyPress('w'), keyPress('l')} {
		a.Update(k)
	}
	if a.focusedPanel != PanelThread || !a.threadDrawnAlone() {
		t.Fatalf("\\ w l: panel = %v, thread alone = %v; want focus in the thread still in front",
			a.focusedPanel, a.threadDrawnAlone())
	}
}

// Moving to another window during an upload refuses with the existing
// toast; focus stays where it was.
func TestPaneFocusWindowMoveRefusedDuringUpload(t *testing.T) {
	a := newTestApp(t, withSize(200, 50), withWindowSplit(wintree.SplitSideBySide))
	focusWindowAt(t, a, 1)
	a.focusedPanel = PanelSidebar
	a.compose.SetUploading(true)
	before := a.focusedWin
	_ = dispatchKeys(a, leaderKey, keyPress('w'), keyPress('l')) // window 0 borders the sidebar
	if a.focusedWin != before || a.focusedPanel != PanelSidebar {
		t.Fatalf("focus moved during an upload: panel %v, window %v", a.focusedPanel, a.focusedWin)
	}
	if got := statusbarText(a); !strings.Contains(got, "Upload in progress") {
		t.Fatalf("statusbar = %q, want the upload toast", got)
	}
}
