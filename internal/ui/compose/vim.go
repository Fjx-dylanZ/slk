// internal/ui/compose/vim.go
//
// The composer's half of [compose] vim_mode: an adapter between the
// textarea and the vimedit engine. The engine owns the vim state; the
// textarea stays the text's owner and its renderer. In vim INSERT the
// adapter steps aside, so typing, the pickers, paste and attachments run
// exactly as without vim mode.
package compose

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"

	"github.com/gammons/slk/internal/bubbles/vimedit"
	"github.com/gammons/slk/internal/ui/styles"
)

// SetVimMode turns vim editing on (reg is the unnamed register, shared
// with any other composer handed the same one) or off (nil).
func (m *Model) SetVimMode(reg *vimedit.Register) {
	if reg == nil {
		m.vim = nil
		m.dirty()
		return
	}
	m.vim = vimedit.New(reg)
	if m.input.Focused() {
		m.vim.EnterInsert(m.vimBuffer())
	}
	m.dirty()
}

// VimMode reports the vim sub-mode; ok is false when vim mode is off.
func (m *Model) VimMode() (mode vimedit.Mode, ok bool) {
	if m.vim == nil {
		return vimedit.Insert, false
	}
	return m.vim.Mode(), true
}

// VimNormal reports whether keys are vim commands rather than text:
// vim mode is on and the composer is in NORMAL, VISUAL or V-LINE.
func (m *Model) VimNormal() bool {
	return m.vim != nil && m.vim.Mode() != vimedit.Insert
}

// VimBusy reports whether a vim command is half typed or a selection is
// up, so Enter should cancel that rather than send.
func (m *Model) VimBusy() bool {
	if m.vim == nil {
		return false
	}
	mode := m.vim.Mode()
	return m.vim.Pending() || mode == vimedit.Visual || mode == vimedit.VisualLine
}

// VimEscape gives Esc to the vim engine and reports whether it took it:
// INSERT goes to NORMAL, VISUAL ends, a half-typed command is dropped.
// False (vim off, or idle NORMAL) means Esc should leave the composer.
// The caller closes any open picker first.
func (m *Model) VimEscape() bool {
	if m.vim == nil {
		return false
	}
	b, ok := m.vim.Handle(m.vimBuffer(), vimedit.Key{Name: vimedit.KeyEsc})
	if ok {
		m.applyVim(b)
		m.dirty()
	}
	return ok
}

// vimBuffer is the textarea's text and cursor as the engine sees them.
func (m Model) vimBuffer() vimedit.Buffer {
	return vimedit.Buffer{Text: m.input.Value(), Cursor: m.cursorPosition()}
}

// vimReset tells the engine the text was replaced from outside it, and
// puts the cursor where its sub-mode allows.
func (m *Model) vimReset() {
	if m.vim == nil {
		return
	}
	b := m.vimBuffer()
	m.vim.Reset(b)
	if n := m.vim.Normalize(b); n.Cursor != b.Cursor {
		m.setCursorOffset(n.Cursor)
	}
}

// vimUpdate handles msg outside vim INSERT, reporting false when the
// textarea path should run instead. Every key is the engine's there, so
// none reaches the textarea; a bracketed paste is one undoable step.
func (m Model) vimUpdate(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.vim == nil || m.vim.Mode() == vimedit.Insert {
		return m, nil, false
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		b, _ := m.vim.Handle(m.vimBuffer(), vimKey(msg))
		cmd := m.applyVim(b)
		m.dirty()
		return m, cmd, true
	case tea.KeyMsg: // a release: nothing to do
		return m, nil, true
	case tea.PasteMsg:
		m.vim.Checkpoint(m.vimBuffer())
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		b := m.vimBuffer()
		if n := m.vim.Normalize(b); n.Cursor != b.Cursor {
			m.setCursorOffset(n.Cursor)
		}
		m.autoGrow()
		m.dirty()
		return m, cmd, true
	}
	return m, nil, false
}

// vimKey translates a key press for the engine.
func vimKey(msg tea.KeyPressMsg) vimedit.Key {
	k := msg.Key()
	mod := k.Mod &^ (tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock)
	switch {
	case k.Code == tea.KeyEscape:
		return vimedit.Key{Name: vimedit.KeyEsc}
	case k.Code == 'r' && mod == tea.ModCtrl:
		return vimedit.Key{Name: vimedit.KeyCtrlR}
	case mod&^tea.ModShift != 0:
		return vimedit.Key{}
	}
	switch k.Code {
	case tea.KeyBackspace:
		return vimedit.Key{Name: vimedit.KeyBackspace}
	case tea.KeyDelete:
		return vimedit.Key{Name: vimedit.KeyDelete}
	case tea.KeyLeft:
		return vimedit.Key{Name: vimedit.KeyLeft}
	case tea.KeyRight:
		return vimedit.Key{Name: vimedit.KeyRight}
	case tea.KeyUp:
		return vimedit.Key{Name: vimedit.KeyUp}
	case tea.KeyDown:
		return vimedit.Key{Name: vimedit.KeyDown}
	case tea.KeyHome:
		return vimedit.Key{Name: vimedit.KeyHome}
	case tea.KeyEnd:
		return vimedit.Key{Name: vimedit.KeyEnd}
	}
	return vimedit.Key{Text: k.Text}
}

// vimRepositionMsg makes the textarea's Update re-run its viewport
// scroll after the adapter moved the cursor; the textarea ignores the
// message itself.
type vimRepositionMsg struct{}

// applyVim writes the engine's buffer back into the textarea.
func (m *Model) applyVim(b vimedit.Buffer) tea.Cmd {
	if b.Text != m.input.Value() {
		m.input.SetValue(b.Text)
		// Picker sessions index the old text: a stale offset into a
		// shorter value is the slice-bounds panic closePickers exists for.
		m.closePickers()
	}
	m.setCursorOffset(b.Cursor)
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(vimRepositionMsg{})
	m.autoGrow()
	return cmd
}

// setCursorOffset moves the textarea cursor to byte offset off of its
// value. The textarea has no row setter: CursorUp/CursorDown walk
// visual rows (each call moves at least one), then SetCursorColumn
// places the column, in runes as the textarea counts them.
func (m *Model) setCursorOffset(off int) {
	val := m.input.Value()
	off = max(0, min(off, len(val)))
	row := strings.Count(val[:off], "\n")
	lineStart := strings.LastIndexByte(val[:off], '\n') + 1
	col := utf8.RuneCountInString(val[lineStart:off])
	limit := len(val) + 2 // a visual row per rune at most, plus slack
	for i := 0; m.input.Line() < row && i < limit; i++ {
		m.input.CursorDown()
	}
	for i := 0; m.input.Line() > row && i < limit; i++ {
		m.input.CursorUp()
	}
	m.input.SetCursorColumn(col)
}

// inputView is the textarea's view, with the vim visual selection
// painted over it when there is one.
func (m Model) inputView(focused bool) string {
	view := m.input.View()
	if !focused || m.vim == nil {
		return view
	}
	// Checked before vimBuffer, which builds the whole value: this runs
	// on every frame.
	if mode := m.vim.Mode(); mode != vimedit.Visual && mode != vimedit.VisualLine {
		return view
	}
	b := m.vimBuffer()
	from, to, ok := m.vim.Selection(b)
	if !ok {
		return view
	}
	return paintSelection(view, b.Text, from, to, b.Cursor, m.input.Width(), m.input.ScrollYOffset(), styles.SelectionStyle())
}

// paintSelection styles bytes [from, to) of text on view, the
// textarea's rendering of text at width scrolled by yOffset rows,
// leaving the cursor's cell as the textarea drew it. An empty line
// inside the selection shows one painted cell, as in vim.
//
// The rows each logical line wraps into come from the textarea itself
// (rowStarts), so the painting follows its wrapping exactly. Only lines
// that can wrap pay for that; the walk stops below the last visible row.
func paintSelection(view, text string, from, to, cursor, width, yOffset int, style lipgloss.Style) string {
	rows := strings.Split(view, "\n")
	var scratch *textarea.Model
	row := 0
	for lineStart := 0; lineStart <= len(text) && row-yOffset < len(rows); {
		lineEnd := len(text)
		if i := strings.IndexByte(text[lineStart:], '\n'); i >= 0 {
			lineEnd = lineStart + i
		}
		line := text[lineStart:lineEnd]
		starts := singleRow
		if !fitsOneRow(line, width) {
			if scratch == nil {
				s := scratchTextarea(width)
				scratch = &s
			}
			starts = rowStarts(scratch, line)
		}
		for k, rs := range starts {
			re := len(line)
			if k+1 < len(starts) {
				re = starts[k+1]
			}
			i := row - yOffset
			row++
			if i < 0 || i >= len(rows) {
				continue
			}
			bs, be := lineStart+rs, lineStart+re
			a, z := max(from, bs), min(to, be)
			var ranges []lipgloss.Range
			switch {
			case a < z:
				ca := cellWidth(text[bs:a])
				cz := ca + cellWidth(text[a:z])
				ranges = splitAtCursor(ca, cz, text, bs, a, z, cursor, style)
			case line == "" && from <= lineStart && to > lineEnd && cursor != lineStart:
				ranges = []lipgloss.Range{lipgloss.NewRange(0, 1, style)}
			}
			if len(ranges) > 0 {
				rows[i] = lipgloss.StyleRanges(rows[i], ranges...)
			}
		}
		lineStart = lineEnd + 1
	}
	return strings.Join(rows, "\n")
}

// singleRow is the row starts of a line that does not wrap.
var singleRow = []int{0}

// fitsOneRow reports whether the textarea draws line on one row at
// width. Its wrap only starts a new row once a row's width plus the
// pending word reaches the width (white space counting one cell each),
// so a line narrower than that is never wrapped. Lines that might wrap
// go to rowStarts; TestFitsOneRowAgreesWithTheTextarea holds the two
// in step.
func fitsOneRow(line string, width int) bool {
	return cellWidth(line) < width
}

// splitAtCursor is the cell ranges [ca, cz) covering bytes [a, z) of a
// row starting at byte bs, minus the cursor's cell.
func splitAtCursor(ca, cz int, text string, bs, a, z, cursor int, style lipgloss.Style) []lipgloss.Range {
	if cursor < a || cursor >= z {
		return []lipgloss.Range{lipgloss.NewRange(ca, cz, style)}
	}
	cc := cellWidth(text[bs:cursor])
	_, rest, _, _ := uniseg.FirstGraphemeClusterInString(text[cursor:], -1)
	cw := cellWidth(text[cursor : len(text)-len(rest)])
	var out []lipgloss.Range
	if cc > ca {
		out = append(out, lipgloss.NewRange(ca, cc, style))
	}
	if cc+cw < cz {
		out = append(out, lipgloss.NewRange(cc+cw, cz, style))
	}
	return out
}

// cellWidth is the display width the textarea gives s: it draws every
// white space as one blank cell.
func cellWidth(s string) int {
	return uniseg.StringWidth(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s))
}

// scratchTextarea is a textarea configured like the composer's (no
// prompt, no line numbers, no frame) whose wrapping rowStarts reads.
func scratchTextarea(width int) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.SetWidth(width)
	return ta
}

// rowStarts is the byte offset each soft-wrapped row of line starts at,
// read from the textarea's own LineInfo so it matches its rendering.
func rowStarts(ta *textarea.Model, line string) []int {
	ta.SetValue(line)
	runes := []rune(line)
	starts := []int{0}
	off := 0 // byte offset of rune c
	prev := 0
	for c := 0; ; {
		ta.SetCursorColumn(c)
		li := ta.LineInfo()
		next := li.StartColumn + li.Width
		if li.Width <= 0 || next > len(runes) || next <= c {
			return starts
		}
		off += len(string(runes[prev:next]))
		prev = next
		starts = append(starts, off)
		c = next
	}
}
