// internal/ui/keyhints.go
//
// The key-hints menu (adaptive key hints design): a which-key panel
// listing what may follow a pending Normal-mode key, opened after a
// delay, and the context menu the key_hints leader action opens at
// once. The menu is a view of the pending state, not a mode: keys keep
// going where they went (handleNormalMode), and after each one
// syncKeyHints makes the menu follow. The drawing is view_keyhints.go;
// the context menu's table is keyscopes.go.
package ui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/bubbles/keyhints"
)

// defaultKeyHintsDelay is how long a pending key waits before the menu
// appears when [keys] key_hints_delay is unset: LazyVim's timeoutlen,
// which it lowered from vim's 1000 ms to bring which-key up quickly. A
// fluent sequence (keys ~100 ms apart) never shows the menu; a pause
// does.
const defaultKeyHintsDelay = 300 * time.Millisecond

// keyHintsDesc describes the key_hints leader action in the menu and
// the help overlay.
const keyHintsDesc = "show keys for this pane"

// keyHintSource is a pending key state the menu can show: one per
// prefix (the keyseq tables, gg, m, '). Each lists what may follow from
// where its keys are defined, so the menu cannot drift from dispatch.
type keyHintSource interface {
	// keyHints is the menu's title (the keys typed so far) and a row per
	// key that may follow. No rows: nothing to show.
	keyHints(a *App) (title string, rows []keyhints.Row)
}

// pendingHintSource returns the key state pending now, in
// handleNormalMode's order, or nil.
func (a *App) pendingHintSource() keyHintSource {
	switch {
	case a.pendingSeq.Pending():
		return seqHints{}
	case a.pendingTop:
		return topHints{}
	case a.pendingMark:
		return markHints{}
	case a.pendingJumpMark:
		return markHints{jump: true}
	}
	return nil
}

// seqHints is a pending keyseq sequence: the leader layer and its
// groups, the window commands, the context menu. The title is the keys
// typed (or the context menu's title) and the label of the group they
// opened; the rows are the armed table's items.
type seqHints struct{}

func (seqHints) keyHints(a *App) (string, []keyhints.Row) {
	s := &a.pendingSeq
	title := s.Title() // the context menu's, which is armed, not typed
	if typed := s.Typed(); len(typed) > 0 {
		title = keyhints.Keys(typed...)
	}
	if l := s.Label(); l != "" && l != groupHint {
		title += " +" + l
	}
	items := s.Next().Items()
	rows := make([]keyhints.Row, len(items))
	for i, it := range items {
		rows[i] = keyhints.Row{Keys: it.Keys, Desc: it.Desc, Group: it.Group}
	}
	return title, rows
}

// topHints is the first g of a g chord: the KeyMap's Top binding (gg),
// then the chordTail rows of keyScopes that apply where focus is (gp in
// the thread pane), as handleNormalMode's pendingTop arm takes them.
type topHints struct{}

func (topHints) keyHints(a *App) (string, []keyhints.Row) {
	top := a.keys.Top
	rows := []keyhints.Row{{Keys: top.Keys(), Desc: top.Help().Desc}}
	ctx := a.hintContext()
	for _, s := range keyScopes {
		if s.kind == chordTail && s.applies(a, ctx) {
			b := s.binding(&a.keys)
			rows = append(rows, keyhints.Row{Keys: b.Keys(), Desc: b.Help().Desc})
		}
	}
	return top.Keys()[0], rows
}

// markHints is m (set a mark) or, with jump, ' (jump to one; only
// pending with [marks] show_jump_overlay off): the workspace's marks,
// each with its target as the marks list shows it, and for a jump the
// back-jump when one is recorded. m with no marks has no rows, so the
// menu stays closed and the status hint says it all.
type markHints struct{ jump bool }

func (h markHints) keyHints(a *App) (string, []keyhints.Row) {
	b := a.keys.MarkSet
	if h.jump {
		b = a.keys.JumpMark
	}
	marks, _ := a.marksRows() // a failed read still lists the session's marks
	rows := make([]keyhints.Row, 0, len(marks)+1)
	for _, m := range marks {
		desc := "#" + m.ChannelName
		if p := m.Preview(); p != "" {
			desc += "  " + p
		}
		rows = append(rows, keyhints.Row{Keys: []string{m.Letter}, Desc: desc})
	}
	if h.jump && a.backJump != nil {
		rows = append(rows, keyhints.Row{Keys: a.keys.JumpMark.Keys(), Desc: a.keys.MarkBackJump.Help().Desc})
	}
	return b.Keys()[0], rows
}

// keyHintsTickMsg opens the menu once a pending key has waited the
// delay, if seq still names the latest key (reduceKeyHints).
type keyHintsTickMsg struct{ seq uint64 }

// syncKeyHints makes the menu follow the pending key state after a key
// (App.handleKey): with nothing pending it closes; open, it shows the
// new level at once (which-key: once open it stays open, across groups
// and Backspace); otherwise, with the menu on, it schedules the tick
// that opens it after the delay, now when the delay is 0. Every key
// that leaves the menu closed bumps keyHintsSeq, so only the latest
// key's tick can open it: a fast `\ w l` never shows it.
func (a *App) syncKeyHints() tea.Cmd {
	src := a.pendingHintSource()
	switch {
	case src == nil:
		a.closeKeyHints()
		return nil
	case a.keyHints.IsVisible():
		a.showKeyHints(src)
		return nil
	case !a.keyHintsOn:
		return nil
	case a.keyHintsDelay <= 0:
		a.showKeyHints(src)
		return nil
	}
	a.keyHintsSeq++
	seq := a.keyHintsSeq
	return tea.Tick(a.keyHintsDelay, func(time.Time) tea.Msg { return keyHintsTickMsg{seq: seq} })
}

// showKeyHints opens the menu on src's rows, or closes it when src has
// none.
func (a *App) showKeyHints(src keyHintSource) {
	title, rows := src.keyHints(a)
	if len(rows) == 0 {
		a.keyHints.Close()
		return
	}
	a.keyHints.Open(title, rows)
}

// closeKeyHints closes the menu and makes any tick in flight stale.
func (a *App) closeKeyHints() {
	a.keyHints.Close()
	a.keyHintsSeq++
}

// pageKeyHints turns the open menu's page on the half-page keys
// (ctrl+d / ctrl+u) when it pages, and reports whether it did. A menu
// that fits leaves them to their usual job.
func (a *App) pageKeyHints(msg tea.KeyMsg) bool {
	if !a.keyHints.IsVisible() {
		return false
	}
	delta := 0
	switch {
	case key.Matches(msg, a.keys.HalfPageDown):
		delta = 1
	case key.Matches(msg, a.keys.HalfPageUp):
		delta = -1
	default:
		return false
	}
	w, h := a.keyHintsArea()
	return a.keyHints.Page(delta, w, h)
}

// openContextHints is the key_hints leader action: the menu of the keys
// that act where focus is (contextKeyMap), open at once whatever the
// delay or [keys] key_hints. Its keys run as typed in Normal mode;
// Esc and Backspace close it.
func (a *App) openContextHints() tea.Cmd {
	m, title := a.contextKeyMap()
	a.pendingSeq.Arm(m, title)
	a.showKeyHints(seqHints{})
	return nil
}

// SetKeyHintsEnabled turns the menu that follows a pending key on or
// off ([keys] key_hints). Off, the status bar's hint is the only one;
// the key_hints action still opens its menu.
func (a *App) SetKeyHintsEnabled(on bool) {
	a.keyHintsOn = on
}

// SetKeyHintsDelay sets how long a pending key waits before the menu
// appears ([keys] key_hints_delay); 0 shows it with the key.
func (a *App) SetKeyHintsDelay(d time.Duration) {
	a.keyHintsDelay = d
}
