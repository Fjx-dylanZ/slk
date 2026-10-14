// Package vimedit is a vim-style modal editing engine over a plain text
// buffer: NORMAL / INSERT / VISUAL sub-modes, counts, motions, operators,
// text objects, an unnamed register, undo/redo and `.` repeat.
//
// It is a pure state machine. The host owns the text and the cursor and
// hands them in with every key as a Buffer; the Editor returns the new
// Buffer and keeps only the modal state between keys (sub-mode, the
// command being typed, the visual anchor, history). It knows nothing of
// bubbletea or of any widget: a host translates its key events into Key
// and writes the Buffer back to its own text input. In INSERT the
// Editor handles nothing but Esc, so the host's own typing (completion
// popups, paste, …) stays in charge there.
//
// Positions are byte offsets into Buffer.Text. Internally the buffer is
// split into grapheme clusters, so every cursor the Editor returns sits
// on a cluster boundary and no edit splits an emoji or a combining
// sequence.
package vimedit

import (
	"strings"
)

// Mode is the Editor's sub-mode.
type Mode uint8

const (
	// Insert: keys type text (the host's job); only Esc reaches the Editor.
	Insert Mode = iota
	// Normal: keys are commands.
	Normal
	// Visual: a charwise selection from the anchor to the cursor.
	Visual
	// VisualLine: a linewise selection.
	VisualLine
)

func (m Mode) String() string {
	switch m {
	case Insert:
		return "INSERT"
	case Normal:
		return "NORMAL"
	case Visual:
		return "VISUAL"
	case VisualLine:
		return "V-LINE"
	}
	return "UNKNOWN"
}

// Buffer is the text being edited and the cursor, a byte offset into it.
type Buffer struct {
	Text   string
	Cursor int
}

// Register is the unnamed register: the text the last delete, change or
// yank took, and whether it was whole lines. Editors handed the same
// Register share it, as vim's registers are shared between windows.
type Register struct {
	Text     string
	Linewise bool
}

// KeyName names the non-printing keys the Editor understands.
type KeyName uint8

const (
	KeyNone KeyName = iota
	KeyEsc
	KeyBackspace
	KeyDelete
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyCtrlR
)

// Key is one key press: Text is what a printable key types ("x", "A",
// " ", "😀"); a non-printing key has empty Text and a Name. A zero Key is
// a key the Editor has no use for (swallowed in NORMAL).
type Key struct {
	Text string
	Name KeyName
}

const (
	// undoLimit bounds the undo history.
	undoLimit = 100
	// maxCount caps a typed count, so `99999999p` cannot build a huge
	// string or spin a motion loop.
	maxCount = 999
	// maxRepeatBytes caps the text a counted put or insert repeat adds.
	maxRepeatBytes = 1 << 18
)

type snapshot struct {
	text   string
	cursor int // byte offset
}

// change is the last repeatable change, for `.`.
type change struct {
	count  int   // 0: none typed
	keys   []Key // the command, counts left out
	insert bool  // the keys opened INSERT and text was typed into it
	text   string
}

// session is an open INSERT: where it began and what opened it.
type session struct {
	undo    snapshot // state before the command that opened it
	base    string   // text when typing began
	at      int      // byte offset typing began at
	keys    []Key    // the opening command, for `.`; nil: not repeatable
	count   int
	repeat  int  // Esc inserts the typed text this many times in total
	newline bool // and each copy on its own line (o, O)
}

// Editor is the modal state of one text input.
type Editor struct {
	reg  *Register
	mode Mode

	// The command being typed.
	count, opCount int
	op             string // "d", "c" or "y"
	pend           string // waiting for a key: "g", "f", "F", "t", "T", "r", "i", "a"
	keys           []Key

	anchor int // visual anchor, byte offset

	// want is the display column j/k aim for, valid while wantSet and the
	// cursor is where the Editor last left it (lastCursor).
	want       int
	wantSet    bool
	wantKeep   bool
	lastCursor int

	find struct{ cmd, target string } // last f/F/t/T, for ; and ,

	undo, redo []snapshot
	pushes     int      // undo steps recorded, ever
	undoing    bool     // this key was u or ctrl+r
	before     snapshot // the buffer as this key found it

	ins *session
	dot *change
}

// New returns an Editor in NORMAL mode. reg is the unnamed register; nil
// gives the Editor one of its own.
func New(reg *Register) *Editor {
	if reg == nil {
		reg = &Register{}
	}
	return &Editor{reg: reg, mode: Normal, lastCursor: -1}
}

// Mode is the current sub-mode.
func (e *Editor) Mode() Mode { return e.mode }

// Pending reports whether a command is half typed: a count, an operator,
// or a key waiting for its argument (f, t, r, g, a text object).
func (e *Editor) Pending() bool {
	return e.count > 0 || e.opCount > 0 || e.op != "" || e.pend != ""
}

func (e *Editor) visual() bool { return e.mode == Visual || e.mode == VisualLine }

// Handle runs one key. It reports false when the key is not the
// Editor's: any key but Esc in INSERT (the host types it), and Esc in
// NORMAL with no command pending (the host leaves the editor). Every
// other key is consumed, unbound ones included.
func (e *Editor) Handle(b Buffer, k Key) (Buffer, bool) {
	if e.mode == Insert {
		if k.Name != KeyEsc {
			return b, false
		}
		st := e.load(b)
		e.leaveInsert(st, true)
		return e.store(st), true
	}
	st := e.load(b)
	if k.Name == KeyEsc {
		switch {
		case e.Pending():
			e.cancel()
		case e.visual():
			e.mode = Normal
			e.cancel()
		default:
			return b, false
		}
		return e.store(st), true
	}

	e.before = snapshot{text: st.t.s, cursor: st.t.byteOf(st.cur)}
	pushes := e.pushes
	e.undoing = false
	e.wantKeep = false
	e.dispatch(st, k)
	if e.mode != Insert && !e.undoing && e.pushes == pushes && st.t.s != e.before.text {
		e.pushUndo(e.before)
	}
	if !e.wantKeep && !e.Pending() {
		e.wantSet = false
	}
	return e.store(st), true
}

// EnterInsert starts INSERT at the buffer's cursor, as `i` does: the host
// calls it when its input gains focus. A session already open is kept.
func (e *Editor) EnterInsert(b Buffer) {
	if e.mode == Insert && e.ins != nil {
		return
	}
	e.cancel()
	e.mode = Insert
	snap := snapshot{text: b.Text, cursor: b.Cursor}
	e.ins = &session{undo: snap, base: b.Text, at: b.Cursor, keys: []Key{{Text: "i"}}, repeat: 1}
}

// Blur closes an open INSERT session (recording its undo step and `.`)
// and drops any half-typed command or selection: the host's input lost
// focus. The Editor rests in NORMAL until EnterInsert.
func (e *Editor) Blur(b Buffer) {
	if e.mode == Insert {
		e.leaveInsert(e.load(b), false)
	}
	e.mode = Normal
	e.cancel()
}

// Reset forgets the undo history and any half-typed command or
// selection: the host replaced the text from outside (another draft, a
// send, an external editor). An open INSERT session restarts on b.
func (e *Editor) Reset(b Buffer) {
	e.undo, e.redo = nil, nil
	e.cancel()
	e.wantSet = false
	if e.visual() {
		e.mode = Normal
	}
	if e.mode == Insert {
		snap := snapshot{text: b.Text, cursor: b.Cursor}
		e.ins = &session{undo: snap, base: b.Text, at: b.Cursor, keys: []Key{{Text: "i"}}, repeat: 1}
	}
}

// Checkpoint records b as an undo step before the host changes the text
// itself outside INSERT (a bracketed paste in NORMAL), and ends any
// selection. In INSERT it does nothing: the session covers it.
func (e *Editor) Checkpoint(b Buffer) {
	if e.mode == Insert {
		return
	}
	e.pushUndo(snapshot{text: b.Text, cursor: b.Cursor})
	e.mode = Normal
	e.cancel()
}

// Normalize moves the cursor onto a cluster boundary and, outside
// INSERT, off a line end NORMAL cannot rest on. Hosts call it after
// changing the text themselves.
func (e *Editor) Normalize(b Buffer) Buffer {
	t := newText(b.Text)
	p := t.cellAt(b.Cursor)
	if e.mode != Insert {
		p = t.clampNormal(p)
	}
	return Buffer{Text: b.Text, Cursor: t.byteOf(p)}
}

// Selection is the visual selection as a byte range [from, to) of
// b.Text; ok is false outside VISUAL and V-LINE. A linewise selection
// covers its lines' text, not the final line break.
func (e *Editor) Selection(b Buffer) (from, to int, ok bool) {
	if !e.visual() {
		return 0, 0, false
	}
	t := newText(b.Text)
	st := &state{t: t, cur: t.clampNormal(t.cellAt(b.Cursor))}
	sp := e.visualSpan(st)
	if sp.lines {
		return st.t.byteOf(st.t.lineStart(sp.from)), st.t.byteOf(st.t.lineEnd(sp.to)), true
	}
	return st.t.byteOf(sp.from), st.t.byteOf(sp.to), true
}

// state is the buffer while one key runs.
type state struct {
	t   *text
	cur int // cell index
}

// replace swaps bytes [from, to) for repl; the caller sets the cursor.
func (st *state) replace(from, to int, repl string) {
	st.t = newText(st.t.s[:from] + repl + st.t.s[to:])
}

func (e *Editor) load(b Buffer) *state {
	t := newText(b.Text)
	p := t.cellAt(b.Cursor)
	if e.mode != Insert {
		p = t.clampNormal(p)
	}
	if b.Cursor != e.lastCursor {
		e.wantSet = false
	}
	return &state{t: t, cur: p}
}

func (e *Editor) store(st *state) Buffer {
	out := Buffer{Text: st.t.s, Cursor: st.t.byteOf(st.cur)}
	e.lastCursor = out.Cursor
	return out
}

// cancel drops the command being typed.
func (e *Editor) cancel() {
	e.count, e.opCount = 0, 0
	e.op, e.pend = "", ""
	e.keys = nil
}

// finish ends a command, recording it for `.` when it changed the text.
func (e *Editor) finish(rec bool) {
	if rec {
		e.dot = &change{count: e.recCount(), keys: append([]Key(nil), e.keys...)}
	}
	e.cancel()
}

// total is the effective count: the counts before and after an operator
// multiply, each defaulting to 1.
func (e *Editor) total() int {
	return min(max(1, e.count)*max(1, e.opCount), maxCount)
}

func (e *Editor) hasCount() bool { return e.count > 0 || e.opCount > 0 }

func (e *Editor) recCount() int {
	if !e.hasCount() {
		return 0
	}
	return e.total()
}

func (e *Editor) pushUndo(s snapshot) {
	e.undo = append(e.undo, s)
	if len(e.undo) > undoLimit {
		e.undo = e.undo[len(e.undo)-undoLimit:]
	}
	e.redo = nil
	e.pushes++
}

// token maps a key to the command it is: printable text as typed, the
// named keys onto their vim equivalents.
func token(k Key) string {
	switch k.Name {
	case KeyLeft, KeyBackspace:
		return "h"
	case KeyRight:
		return "l"
	case KeyUp:
		return "k"
	case KeyDown:
		return "j"
	case KeyHome:
		return "<home>"
	case KeyEnd:
		return "$"
	case KeyDelete:
		return "x"
	case KeyCtrlR:
		return "<c-r>"
	case KeyNone:
		if k.Text == " " {
			return "l"
		}
		return k.Text
	}
	return ""
}

func (e *Editor) dispatch(st *state, k Key) {
	if e.pend != "" {
		e.keys = append(e.keys, k)
		e.resolve(st, k)
		return
	}
	tok := token(k)
	if tok == "" {
		e.cancel()
		return
	}
	if len(tok) == 1 && tok[0] >= '0' && tok[0] <= '9' && (tok != "0" || e.count > 0 || e.opCount > 0) {
		d := int(tok[0] - '0')
		if e.op != "" {
			e.opCount = min(e.opCount*10+d, maxCount)
		} else {
			e.count = min(e.count*10+d, maxCount)
		}
		return
	}
	e.keys = append(e.keys, k)
	visual := e.visual()

	switch tok {
	case "d", "c", "y":
		switch {
		case visual:
			e.visualOperator(st, tok)
		case e.op == "":
			e.op = tok
		case e.op == tok:
			e.linesOp(st)
		default:
			e.cancel()
		}
		return
	case "i", "a":
		if e.op != "" || visual {
			e.pend = tok
			return
		}
	case "g", "f", "F", "t", "T":
		e.pend = tok
		return
	}
	if m, ok := e.motion(st, tok); ok {
		e.applyMotion(st, m)
		return
	}
	switch {
	case e.op != "":
		e.cancel()
	case visual:
		e.visualCmd(st, tok)
	default:
		e.normalCmd(st, tok)
	}
}

// resolve completes a command waiting for one more key.
func (e *Editor) resolve(st *state, k Key) {
	p := e.pend
	e.pend = ""
	switch p {
	case "g":
		switch k.Text {
		case "g":
			e.applyMotion(st, e.gotoLine(st, 0))
		case "e", "E":
			e.applyMotion(st, motion{pos: st.t.bckendWord(st.cur, e.total(), k.Text == "E"), kind: inclusive, ok: true})
		default:
			e.cancel()
		}
	case "f", "F", "t", "T":
		if k.Text == "" {
			e.cancel()
			return
		}
		e.find.cmd, e.find.target = p, k.Text
		e.applyMotion(st, e.findMotion(st, p, k.Text, false))
	case "r":
		if k.Text == "" {
			e.cancel()
			return
		}
		if e.visual() {
			e.visualReplace(st, k.Text)
			return
		}
		e.replaceChars(st, k.Text)
	case "i", "a":
		sp, ok := st.t.object(st.cur, k.Text, p == "i")
		if !ok {
			e.cancel()
			return
		}
		if e.visual() {
			e.selectObject(st, sp)
			return
		}
		e.operate(st, e.op, sp, true)
	}
}

// applyMotion moves the cursor, or hands the motion's span to the
// pending operator.
func (e *Editor) applyMotion(st *state, m motion) {
	if !m.ok {
		e.cancel()
		return
	}
	if e.op != "" {
		e.operate(st, e.op, st.t.motionSpan(st.cur, m), true)
		return
	}
	st.cur = st.t.clampNormal(m.pos)
	switch {
	case m.keepWant:
		e.want, e.wantSet, e.wantKeep = m.want, true, true
	case m.wantEnd:
		e.want, e.wantSet, e.wantKeep = endCol, true, true
	}
	e.finish(false)
}

// wantCol is the display column j/k aim for.
func (e *Editor) wantCol(st *state) int {
	if e.wantSet {
		return e.want
	}
	return st.t.colOf(st.cur)
}

// startInsert opens INSERT at cell p. rec: the command is repeatable.
func (e *Editor) startInsert(st *state, p, repeat int, newline, rec bool) {
	st.cur = p
	s := &session{
		undo:    e.before,
		base:    st.t.s,
		at:      st.t.byteOf(p),
		count:   e.recCount(),
		repeat:  repeat,
		newline: newline,
	}
	if rec {
		s.keys = append([]Key(nil), e.keys...)
	}
	e.ins = s
	e.mode = Insert
	e.cancel()
}

// leaveInsert closes the INSERT session: Esc (esc) repeats the typed
// text for a counted insert and steps the cursor left, as vim does.
func (e *Editor) leaveInsert(st *state, esc bool) {
	s := e.ins
	e.ins = nil
	e.mode = Normal
	if s != nil {
		typed, pure := inserted(s.base, s.at, st.t.s)
		if esc && pure && typed != "" && s.repeat > 1 {
			piece := typed
			if s.newline {
				piece = "\n" + typed
			}
			extra := repeatCapped(piece, "", s.repeat-1)
			at := s.at + len(typed)
			st.replace(at, at, extra)
			st.cur = st.t.cellAt(at + len(extra))
		}
		if st.t.s != s.undo.text {
			e.pushUndo(s.undo)
		}
		if s.keys != nil && pure {
			e.dot = &change{count: s.count, keys: s.keys, insert: true, text: typed}
		} else {
			e.dot = nil
		}
	}
	if esc && st.cur > st.t.lineStart(st.cur) {
		st.cur--
	}
	st.cur = st.t.clampNormal(st.cur)
	e.cancel()
}

// inserted reports the text a session added when all it did was insert
// at byte offset at of base.
func inserted(base string, at int, final string) (string, bool) {
	if at < 0 || at > len(base) || len(final) < len(base) {
		return "", false
	}
	head, tail := base[:at], base[at:]
	if !strings.HasPrefix(final, head) || !strings.HasSuffix(final[at:], tail) {
		return "", false
	}
	return final[at : len(final)-len(tail)], true
}

// repeat runs the last change again; a typed count replaces its count.
func (e *Editor) repeat(st *state) {
	d := e.dot
	if d == nil {
		e.cancel()
		return
	}
	count := d.count
	if e.hasCount() {
		count = e.total()
	}
	e.cancel()
	e.count = count
	for _, k := range d.keys {
		e.dispatch(st, k)
	}
	if d.insert && e.mode == Insert {
		at := st.t.byteOf(st.cur)
		st.replace(at, at, d.text)
		st.cur = st.t.cellAt(at + len(d.text))
		e.leaveInsert(st, true)
	}
}

func (e *Editor) undoStep(st *state) {
	e.undoing = true
	for range e.total() {
		if len(e.undo) == 0 {
			break
		}
		s := e.undo[len(e.undo)-1]
		e.undo = e.undo[:len(e.undo)-1]
		e.redo = append(e.redo, snapshot{text: st.t.s, cursor: s.cursor})
		restore(st, s)
	}
	e.cancel()
}

func (e *Editor) redoStep(st *state) {
	e.undoing = true
	for range e.total() {
		if len(e.redo) == 0 {
			break
		}
		s := e.redo[len(e.redo)-1]
		e.redo = e.redo[:len(e.redo)-1]
		e.undo = append(e.undo, snapshot{text: st.t.s, cursor: s.cursor})
		restore(st, s)
	}
	e.cancel()
}

// restore puts snapshot s back, with the cursor at the start of the
// change being undone or redone, as in vim: where the two texts first
// differ, or the snapshot's cursor when that is earlier (so deleting one
// of a run of equal characters comes back on that character).
func restore(st *state, s snapshot) {
	diff := 0
	for diff < len(s.text) && diff < len(st.t.s) && s.text[diff] == st.t.s[diff] {
		diff++
	}
	st.t = newText(s.text)
	st.cur = st.t.clampNormal(st.t.cellAt(min(s.cursor, diff)))
}
