// internal/ui/keyhints_test.go
//
// The key-hints menu (adaptive key hints design) through the real
// Update chain: the delay and its tick, never for a fast sequence,
// levels, Backspace and Esc, group labels, ctrl+w, gg, marks, the
// key_hints context menu in each context, the disabled fallback,
// paging, sixel images under the menu, and the scope table's lockstep
// with the KeyMap.
package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/gammons/slk/internal/ui/workspace"
)

// testHintsDelay is a real but short delay, so a tick runs within the
// test (deliveredMsgs runs it on its timer).
const testHintsDelay = 20 * time.Millisecond

// menuApp is the normal fixture at 140x40 with the menu's delay short.
func menuApp(t *testing.T, opts ...testOpt) *App {
	t.Helper()
	a := newTestApp(t, append(append(normalOpts(), withSize(140, 40)), opts...)...)
	a.SetKeyHintsDelay(testHintsDelay)
	return a
}

// sendKey sends k through Update and returns its cmd.
func sendKey(a *App, k tea.KeyMsg) tea.Cmd {
	_, cmd := a.Update(k)
	return cmd
}

// menuText is the menu as the frame draws it, one stripped line per
// screen row of its box; "" when no menu is on screen.
func menuText(a *App) string {
	frame := a.View().Content
	_, r := a.keyHintsBox()
	if r.w == 0 {
		return ""
	}
	var out []string
	for _, l := range strings.Split(frame, "\n")[r.y : r.y+r.h] {
		out = append(out, ansi.Strip(ansi.Cut(l, r.x, r.x+r.w)))
	}
	return strings.Join(out, "\n")
}

// rowKeys are the menu's rows as "keys → desc", groups with their "+".
func rowKeys(a *App) []string {
	var out []string
	for _, r := range a.keyHints.Rows() {
		d := r.Desc
		if r.Group {
			d = "+" + d
		}
		out = append(out, strings.Join(r.Keys, "/")+" → "+d)
	}
	return out
}

func wantRows(t *testing.T, a *App, want ...string) {
	t.Helper()
	got := rowKeys(a)
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("menu lacks %q; rows %q", w, got)
		}
	}
}

func unwantedRows(t *testing.T, a *App, gone ...string) {
	t.Helper()
	got := rowKeys(a)
	for _, g := range gone {
		if slices.ContainsFunc(got, func(r string) bool { return strings.HasPrefix(r, g+" → ") }) {
			t.Errorf("menu lists %q; rows %q", g, got)
		}
	}
}

// The menu opens only when the leader's own delay tick arrives, and
// shows the layer under a title naming the leader.
func TestKeyHintsAfterTheDelay(t *testing.T) {
	a := menuApp(t)
	cmd := sendKey(a, leaderKey)
	if a.keyHints.IsVisible() || menuText(a) != "" {
		t.Fatal("the menu opened before the delay")
	}
	if h := hint(a); !strings.Contains(h, `\ …  w +windows`) {
		t.Fatalf("status hint %q, want the leader's continuations while the delay runs", h)
	}
	tick := deliveredMsgs[keyHintsTickMsg](t, cmd)[0]
	a.Update(tick)
	menu := menuText(a)
	for _, want := range []string{`╭─ \ ─`, "t → switch channel", "w → +windows", "? → show keys for this pane", "esc close  ⌫ back"} {
		if !strings.Contains(menu, want) {
			t.Errorf("menu lacks %q:\n%s", want, menu)
		}
	}
	// Plain actions come before the groups.
	if rows := rowKeys(a); rows[len(rows)-1] != "w → +windows" {
		t.Errorf("last row %q, want the windows group after the actions", rows[len(rows)-1])
	}
}

// A sequence typed faster than the delay never shows the menu: every
// key's tick but the last is stale, and the last finds nothing pending.
func TestKeyHintsNeverFlashForAFastSequence(t *testing.T) {
	a := menuApp(t)
	a.focusedPanel = PanelSidebar
	first := sendKey(a, leaderKey)
	second := sendKey(a, keyPress('w'))
	if third := sendKey(a, keyPress('l')); third != nil {
		if msgs := drainBatch(third); slices.ContainsFunc(msgs, func(m tea.Msg) bool { _, ok := m.(keyHintsTickMsg); return ok }) {
			t.Fatal("the completing key scheduled a menu tick")
		}
	}
	for _, tick := range deliveredMsgs[keyHintsTickMsg](t, first, second) {
		a.Update(tick)
		if a.keyHints.IsVisible() || menuText(a) != "" {
			t.Fatal("a stale tick opened the menu after the sequence completed")
		}
	}
	if a.focusedPanel != PanelMessages {
		t.Errorf(`\ w l: focus = %v, want the messages pane right of the sidebar`, a.focusedPanel)
	}
}

// Once open, the menu follows the sequence at once: a group, Backspace
// back to the root, Esc closing it with the sequence.
func TestKeyHintsFollowTheSequence(t *testing.T) {
	a := menuApp(t)
	a.Update(deliveredMsgs[keyHintsTickMsg](t, sendKey(a, leaderKey))[0])
	if cmd := sendKey(a, keyPress('w')); cmd != nil {
		t.Errorf("an open menu waited again for the group")
	}
	if got := a.keyHints.Title(); got != `\ w +windows` {
		t.Errorf("group title %q", got)
	}
	wantRows(t, a, "h/j/k/l → focus pane in direction", "q/c → close window", "s → split window")
	if !strings.Contains(menuText(a), `╭─ \ w +windows ─`) {
		t.Errorf("the group's title is not in the border:\n%s", menuText(a))
	}
	sendKey(a, keyCode(tea.KeyBackspace))
	if !a.pendingSeq.Pending() || a.keyHints.Title() != `\` {
		t.Fatalf("Backspace: pending %v title %q, want the leader level", a.pendingSeq.Pending(), a.keyHints.Title())
	}
	wantRows(t, a, "t → switch channel")
	sendKey(a, keyCode(tea.KeyEscape))
	if a.pendingSeq.Pending() || a.keyHints.IsVisible() || menuText(a) != "" {
		t.Fatal("Esc left the sequence or the menu up")
	}
	seqDone(t, a)
	// Backspace at the first level closes too.
	a.SetKeyHintsDelay(0)
	sendKey(a, leaderKey)
	sendKey(a, keyCode(tea.KeyBackspace))
	if a.pendingSeq.Pending() || a.keyHints.IsVisible() {
		t.Fatal("Backspace at the leader level left it pending")
	}
	// A mode change closes it with the sequence (ctrl+c's prompt).
	sendKey(a, leaderKey)
	sendKey(a, keyMod('c', tea.ModCtrl))
	if a.keyHints.IsVisible() || a.mode != ModeConfirm {
		t.Fatalf("ctrl+c: menu %v mode %v, want the quit prompt alone", a.keyHints.IsVisible(), a.mode)
	}
}

// [keys.leader_groups] labels nested user groups in the rows, the
// title and the status hint; groups sort after the plain rows.
func TestKeyHintsGroupLabels(t *testing.T) {
	a := nvimApp(t)
	a.width, a.height = 140, 40
	a.SetLeaderGroups(map[string]string{"f": "find", "s": "search"})
	a.SetKeyHintsDelay(0)
	typeTokens(t, a, "space")
	want := []string{
		"a → open activity", "e → toggle sidebar", "n → new message", "q → close thread view",
		"y → switch theme (per workspace)", "Y → set default theme", "/ → search workspace", "? → show keybindings",
		"] → toggle thread", "space → switch channel",
		"f → +find", "s → +search", "w → +windows",
	}
	if got := rowKeys(a); !slices.Equal(got, want) {
		t.Fatalf("rows\n got %q\nwant %q", got, want)
	}
	if h := hint(a); !strings.Contains(h, "space …  w +windows  f +find  space channel  / search  s +search") {
		t.Errorf("status hint %q lacks the labels", h)
	}
	typeTokens(t, a, "f")
	if a.keyHints.Title() != "␣ f +find" {
		t.Errorf("title %q, want ␣ f +find", a.keyHints.Title())
	}
	wantRows(t, a, "f → switch channel")
	if menu := menuText(a); !strings.Contains(menu, "╭─ ␣ f +find ─") || !strings.Contains(menu, "f → switch channel") {
		t.Errorf("the group's menu is not on screen:\n%s", menu)
	}
	typeTokens(t, a, "f")
	if a.mode != ModeChannelFinder || a.keyHints.IsVisible() {
		t.Fatalf("space f f: mode %v, menu %v", a.mode, a.keyHints.IsVisible())
	}
}

func TestKeyHintsCtrlW(t *testing.T) {
	a := menuApp(t)
	cmd := sendKey(a, keyMod('w', tea.ModCtrl))
	a.Update(deliveredMsgs[keyHintsTickMsg](t, cmd)[0])
	if a.keyHints.Title() != "ctrl+w +windows" {
		t.Errorf("title %q", a.keyHints.Title())
	}
	wantRows(t, a, "h/j/k/l → focus pane in direction", "v → vertical split window", "o → close other windows")
	if menu := menuText(a); !strings.Contains(menu, "╭─ ctrl+w +windows ─") {
		t.Errorf("the ctrl+w menu is not on screen:\n%s", menu)
	}
}

func TestKeyHintsTop(t *testing.T) {
	a := menuApp(t)
	focusMessages(t, a)
	a.Update(deliveredMsgs[keyHintsTickMsg](t, sendKey(a, keyPress('g')))[0])
	if a.keyHints.Title() != "g" || !slices.Equal(rowKeys(a), []string{"g → top"}) {
		t.Fatalf("g menu: title %q rows %q", a.keyHints.Title(), rowKeys(a))
	}
	if menu := menuText(a); !strings.Contains(menu, "g → top") {
		t.Fatalf("the g menu is not on screen:\n%s", menu)
	}
	sendKey(a, keyPress('g'))
	if a.pendingTop || a.keyHints.IsVisible() {
		t.Fatal("gg left the chord or the menu up")
	}
}

// m and ' list the workspace's marks with their targets; ' also offers
// the back-jump once one is recorded. With no marks m has no menu, and
// the status hint carries on.
func TestKeyHintsMarks(t *testing.T) {
	a := jumpMarkTestApp(t)
	a.width, a.height = 140, 40
	a.SetKeyHintsDelay(0)
	sendKey(a, keyPress('m'))
	if !a.pendingMark || a.keyHints.IsVisible() {
		t.Fatalf("m with no marks: pending %v, menu %v; want the chord without a menu", a.pendingMark, a.keyHints.IsVisible())
	}
	if h := hint(a); !strings.Contains(h, "m…") {
		t.Errorf("status hint %q", h)
	}
	sendKey(a, keyCode(tea.KeyEscape))

	for letter, m := range map[string]Mark{
		"a": {Location: Location{TeamID: "T1", ChannelID: "C2", MessageTS: "10.0"}, ChannelName: "ops", AuthorName: "alice", Excerpt: "deploy at noon"},
		"b": {Location: Location{TeamID: "T1", ChannelID: "C3", MessageTS: "20.0"}, ChannelName: "design"},
	} {
		m.Letter = letter
		if err := a.marks.Set("T1", letter, m); err != nil {
			t.Fatal(err)
		}
	}
	sendKey(a, keyPress('m'))
	if a.keyHints.Title() != "m" || !slices.Equal(rowKeys(a), []string{"a → #ops  alice: deploy at noon", "b → #design"}) {
		t.Fatalf("m menu: title %q rows %q", a.keyHints.Title(), rowKeys(a))
	}
	if menu := menuText(a); !strings.Contains(menu, "a → #ops  alice: deploy at noon") || !strings.Contains(menu, "b → #design") {
		t.Fatalf("the m menu is not on screen:\n%s", menu)
	}
	sendKey(a, keyCode(tea.KeyEscape))

	a.SetShowJumpOverlay(false)
	sendKey(a, keyPress('\''))
	if a.keyHints.Title() != "'" || slices.ContainsFunc(rowKeys(a), func(r string) bool { return strings.Contains(r, "back-jump") }) {
		t.Fatalf("' menu before any jump: title %q rows %q", a.keyHints.Title(), rowKeys(a))
	}
	sendKey(a, keyCode(tea.KeyEscape))
	a.Update(ChannelSelectedMsg{ID: "C1", Name: "one", Type: "channel"})
	sendKey(a, keyPress('\''))
	sendKey(a, keyPress('a')) // jump to a: records the back-jump
	if a.backJump == nil {
		t.Fatal("precondition: the jump recorded no back-jump")
	}
	sendKey(a, keyPress('\''))
	wantRows(t, a, "a → #ops  alice: deploy at noon", "'/` → back-jump to where you jumped from")
}

// A click on the workspace rail while ' waits for a mark cancels it, as
// a switch from the keyboard does (the digit cancels the chord first):
// the switch's SetMode(ModeNormal) closes the menu listing T1's marks
// with the chord, so a letter typed next does not jump to T2's mark of
// that letter.
func TestKeyHintsWorkspaceSwitchCancelsThePendingKey(t *testing.T) {
	a := jumpMarkTestApp(t)
	a.width, a.height = 120, 30
	a.SetWorkspaces([]workspace.WorkspaceItem{
		{ID: "T1", Name: "Acme", Initials: "AC"},
		{ID: "T2", Name: "Beta", Initials: "BE"},
	})
	a.setWorkspaceSwitcherForTest(func(teamID string) tea.Msg {
		return WorkspaceSwitchedMsg{TeamID: teamID, TeamName: "Beta"}
	})
	seedMark(t, a, "a", Location{TeamID: "T1", ChannelID: "C2", MessageTS: "10.0"})
	if err := a.marks.Set("T2", "a", Mark{Location: Location{TeamID: "T2", ChannelID: "C3", MessageTS: "20.0"}, Letter: "a"}); err != nil {
		t.Fatal(err)
	}
	a.SetShowJumpOverlay(false)
	a.SetKeyHintsDelay(0)
	_ = a.View() // lays out the rail for the click
	sendKey(a, keyPress('\''))
	if !a.keyHints.IsVisible() || !a.pendingJumpMark {
		t.Fatal("precondition: ' did not open the marks menu")
	}

	_, cmd := a.Update(tea.MouseClickMsg{X: 0, Y: 3, Button: tea.MouseLeft}) // T2's tile
	for _, msg := range drainBatch(cmd) {
		a.Update(msg)
	}
	if a.activeTeamID != "T2" {
		t.Fatalf("precondition: the click did not switch to T2 (active %q)", a.activeTeamID)
	}
	if a.pendingJumpMark || a.keyHints.IsVisible() {
		t.Fatalf("after the switch: ' pending %v, menu %v (rows %q); want both gone",
			a.pendingJumpMark, a.keyHints.IsVisible(), rowKeys(a))
	}
	seqDone(t, a)
	sendKey(a, keyPress('a'))
	if a.backJump != nil {
		t.Error("a after the switch jumped to T2's mark a")
	}
}

// key_hints = false: no tick, no menu; the status hint is the hint. The
// key_hints action still opens its menu, at once.
func TestKeyHintsDisabled(t *testing.T) {
	a := menuApp(t)
	a.SetKeyHintsEnabled(false)
	a.SetKeyHintsDelay(0)
	if cmd := sendKey(a, leaderKey); cmd != nil {
		t.Errorf("disabled, the leader still scheduled something")
	}
	if a.keyHints.IsVisible() || menuText(a) != "" {
		t.Fatal("disabled, the menu opened")
	}
	if h := hint(a); !strings.Contains(h, `\ …  w +windows  t channel`) {
		t.Errorf("status hint %q", h)
	}
	sendKey(a, keyPress('?'))
	if !a.keyHints.IsVisible() || !strings.HasPrefix(a.keyHints.Title(), "keys: ") {
		t.Fatalf("\\ ? with the menu off: visible %v title %q", a.keyHints.IsVisible(), a.keyHints.Title())
	}
}

// A terminal too small for one row draws no menu; the status hint stays.
func TestKeyHintsTooSmall(t *testing.T) {
	a := menuApp(t)
	a.width, a.height = 140, 3
	a.SetKeyHintsDelay(0)
	sendKey(a, leaderKey)
	if menuText(a) != "" {
		t.Fatal("a menu was drawn in a 3-row terminal")
	}
	if h := hint(a); !strings.Contains(h, `\ …`) {
		t.Errorf("status hint %q", h)
	}
}

// openContext opens the key_hints menu with \ ? and checks it opened at
// once.
func openContext(t *testing.T, a *App) {
	t.Helper()
	sendKey(a, leaderKey)
	if cmd := sendKey(a, keyPress('?')); cmd != nil {
		if slices.ContainsFunc(drainBatch(cmd), func(m tea.Msg) bool { _, ok := m.(keyHintsTickMsg); return ok }) {
			t.Error("the context menu waited for a tick")
		}
	}
	if !a.keyHints.IsVisible() {
		t.Fatal(`\ ?: no menu`)
	}
}

// The context menu lists what acts where focus is, and differs by pane
// and view.
func TestKeyHintsContextMenu(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  []testOpt
		setup func(t *testing.T, a *App)
		title string
		want  []string
		gone  []string
	}{
		{
			name:  "messages pane",
			setup: func(t *testing.T, a *App) { focusMessages(t, a) },
			title: "keys: messages",
			want: []string{"y → copy message", "E → edit message", "i → insert mode", "/ → search", "⏎ → open/confirm",
				"k/up → up", "g → +top", `\ → +leader`, "ctrl+w → +windows", "ctrl+t/ctrl+p → switch channel", "pgup → page up"},
			gone: []string{"S", "u", "esc", "n", "q", "␣", "ctrl+]"},
		},
		{
			name: "sidebar, configured keys",
			setup: func(t *testing.T, a *App) {
				a.SetKeyBindings(neovimSidebar)
				a.focusedPanel = PanelSidebar
			},
			title: "keys: sidebar",
			want:  []string{"enter/l → open row / toggle section (sidebar)", "space → toggle section", "right → right", "h/left → left", "/ → search"},
			gone:  []string{"y", "E", "⏎", "S", "l/right"},
		},
		{
			name:  "thread pane",
			setup: func(t *testing.T, a *App) { focusThreadPanel(t, a) },
			title: "keys: thread",
			want:  []string{"S → save thread", "y → copy message", "q → close thread view", "ctrl+] → toggle thread", "i → insert mode"},
			gone:  []string{"/", "⏎", "u"},
		},
		{
			name:  "Threads list",
			opts:  []testOpt{withThreadsView([]cache.ThreadSummary{{ChannelID: "C1", ThreadTS: "1.0", ParentText: "p"}}), withView(ViewThreads)},
			setup: func(t *testing.T, a *App) { a.focusedPanel = PanelMessages },
			title: "keys: threads",
			want:  []string{"⏎ → open/confirm", "j/down → down", "g → +top", "m → set mark"},
			gone:  []string{"y", "E", "/", "i", "u", "U"},
		},
		{
			name:  "Activity list",
			opts:  []testOpt{withView(ViewActivity)},
			setup: func(t *testing.T, a *App) { a.focusedPanel = PanelMessages },
			title: "keys: activity",
			want:  []string{"u → Activity: toggle unread-only", "U → mark unread", "⏎ → open/confirm", "G → bottom"},
			gone:  []string{"y", "g", "m", "/", "i"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := menuApp(t, tc.opts...)
			tc.setup(t, a)
			openContext(t, a)
			if a.keyHints.Title() != tc.title {
				t.Errorf("title %q, want %q", a.keyHints.Title(), tc.title)
			}
			// The menu draws keys with glyphs; the rows hold key names.
			want := make([]string, len(tc.want))
			for i, w := range tc.want {
				want[i] = strings.Replace(w, "⏎", "enter", 1)
			}
			wantRows(t, a, want...)
			gone := make([]string, len(tc.gone))
			for i, g := range tc.gone {
				gone[i] = strings.NewReplacer("⏎", "enter", "␣", "space").Replace(g)
			}
			unwantedRows(t, a, gone...)
			if !strings.Contains(menuText(a), "╭─ "+tc.title+" ─") {
				t.Errorf("menu not drawn under its title:\n%s", menuText(a))
			}
		})
	}
}

// A key from the context menu runs as typed in Normal mode; the leader
// there opens its own menu at once; anything unlisted closes the menu
// and does nothing; Esc closes it.
func TestKeyHintsContextMenuKeys(t *testing.T) {
	a := menuApp(t)
	focusMessages(t, a)
	openContext(t, a)
	sendKey(a, keyPress('i'))
	if a.mode != ModeInsert || a.keyHints.IsVisible() || a.pendingSeq.Pending() {
		t.Fatalf("i from the menu: mode %v, menu %v", a.mode, a.keyHints.IsVisible())
	}
	sendKey(a, keyCode(tea.KeyEscape))

	openContext(t, a)
	sendKey(a, leaderKey)
	if !a.keyHints.IsVisible() || a.keyHints.Title() != `\` || !a.pendingSeq.Pending() {
		t.Fatalf(`\ from the menu: menu %v title %q`, a.keyHints.IsVisible(), a.keyHints.Title())
	}
	if h := hint(a); !strings.Contains(h, `\ …  w +windows`) {
		t.Errorf("status hint %q, want the leader's", h)
	}
	sendKey(a, keyCode(tea.KeyEscape))

	a.focusedPanel = PanelSidebar
	openContext(t, a)
	sendKey(a, keyPress('D')) // delete message: not a sidebar key
	if a.keyHints.IsVisible() || a.pendingSeq.Pending() || a.mode != ModeNormal {
		t.Fatalf("D from the sidebar menu: menu %v, mode %v; want closed and nothing run", a.keyHints.IsVisible(), a.mode)
	}
	seqDone(t, a)
}

// A menu longer than the screen pages: ctrl+d / ctrl+u turn the page and
// the pending state stays; a menu that fits leaves them alone.
func TestKeyHintsPaging(t *testing.T) {
	a := menuApp(t)
	a.width, a.height = 60, 14
	focusMessages(t, a)
	openContext(t, a)
	if !strings.Contains(menuText(a), "ctrl+d/u 1/") {
		t.Fatalf("the context menu fits in 60x14, nothing to page:\n%s", menuText(a))
	}
	sendKey(a, keyMod('d', tea.ModCtrl))
	if !a.keyHints.IsVisible() || !a.pendingSeq.Pending() || !strings.Contains(menuText(a), "ctrl+d/u 2/") {
		t.Fatalf("ctrl+d did not turn the page:\n%s", menuText(a))
	}
	sendKey(a, keyMod('u', tea.ModCtrl))
	if !strings.Contains(menuText(a), "ctrl+d/u 1/") {
		t.Fatalf("ctrl+u did not turn back:\n%s", menuText(a))
	}
}

// Sixel images under the menu are withheld (they are painted over the
// text and would cover it); the others stay.
func TestKeyHintsWithholdSixelUnderTheMenu(t *testing.T) {
	a, _, _ := twoWindowApp(t)
	a.imgProtocol = imgpkg.ProtoSixel
	a.sixelFrames = imgpkg.NewSixelFrameStore()
	setupTwoWindowSixelImages(t, a)
	published := func() []imgpkg.SixelPlacement {
		v := a.View()
		f, ok := a.sixelFrames.Take(frameIDFromTitle(t, v.WindowTitle))
		if !ok {
			t.Fatal("no frame published")
		}
		return f.Placements
	}
	before := published()
	a.focusedPanel = PanelMessages
	a.SetKeyHintsDelay(0)
	openContext(t, a)
	_, r := a.keyHintsBox()
	after := published()
	// Whether p (1-based) covers a cell of the menu's box.
	under := func(p imgpkg.SixelPlacement) bool {
		rows := p.Row-1 < r.y+r.h && r.y < p.Row-1+p.Rows
		cols := p.Col-1 < r.x+r.w && r.x < p.Col-1+p.Cols
		return rows && cols
	}
	var withheld, kept int
	for _, p := range before {
		found := slices.ContainsFunc(after, func(q imgpkg.SixelPlacement) bool { return q.Key == p.Key && q.Row == p.Row && q.Col == p.Col })
		switch {
		case under(p) && found:
			t.Errorf("placement at %d,%d under the menu %+v was published", p.Row, p.Col, r)
		case under(p):
			withheld++
		case !found:
			t.Errorf("placement at %d,%d clear of the menu %+v was withheld", p.Row, p.Col, r)
		default:
			kept++
		}
	}
	if withheld == 0 || kept == 0 {
		t.Fatalf("fixture: %d withheld, %d kept of %d; want both kinds", withheld, kept, len(before))
	}
}

// Every Normal-mode KeyMap binding has exactly one place that says where
// it acts: a keyScopes row, or a keyActions row for the configurable
// sidebar keys. Keyless help-only rows and Insert-only keys have none,
// and no row names a binding without keys.
func TestKeyScopesCoverTheKeyMap(t *testing.T) {
	km := DefaultKeyMap()
	v := reflect.ValueOf(&km).Elem()
	rows := map[*key.Binding]int{}
	for _, s := range keyScopes {
		rows[s.binding(&km)]++
	}
	for _, act := range keyActions {
		rows[act.binding(&km)]++
	}
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		b := v.Field(i).Addr().Interface().(*key.Binding)
		n := rows[b]
		delete(rows, b)
		switch normal := len(b.Keys()) > 0 && !slices.Contains(insertOnly, name); {
		case normal && n == 0:
			t.Errorf("KeyMap.%s has no keyScopes row: say where it acts, so the key_hints menu lists it", name)
		case normal && n > 1:
			t.Errorf("KeyMap.%s has %d scope rows, want one", name, n)
		case !normal && n > 0:
			t.Errorf("KeyMap.%s is not a Normal-mode key but has a scope row", name)
		}
	}
	if len(rows) > 0 {
		t.Errorf("%d scope rows name no KeyMap field", len(rows))
	}
}

// The kept draw (keyHintsDraw) is always what a fresh draw would give:
// after a repeat frame, a level change, a page, a resize and a theme
// change, the frame equals the one drawn with the draw dropped.
func TestKeyHintsKeptDrawMatchesAFreshOne(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	a := menuApp(t)
	a.width, a.height = 60, 14
	a.SetKeyHintsDelay(0)
	focusMessages(t, a)
	check := func(step string) {
		t.Helper()
		kept := a.View().Content
		if menuText(a) == "" {
			t.Fatalf("%s: no menu", step)
		}
		a.keyHintsDraw = keyHintsDraw{}
		if fresh := a.View().Content; kept != fresh {
			t.Errorf("%s: the kept draw differs from a fresh one", step)
		}
	}
	sendKey(a, leaderKey)
	check("leader")
	check("a repeat frame")
	sendKey(a, keyPress('?'))
	check("the context menu")
	sendKey(a, keyMod('d', tea.ModCtrl))
	check("a page")
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	check("a resize")
	a.applyTheme("light")
	check("a theme")
}
