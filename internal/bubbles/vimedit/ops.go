package vimedit

import (
	"strings"
	"unicode"
)

// operate applies op to sp. rec: record it for `.`.
func (e *Editor) operate(st *state, op string, sp span, rec bool) {
	switch op {
	case "d":
		e.deleteSpan(st, sp)
		e.finish(rec)
	case "c":
		e.changeSpan(st, sp, rec)
	case "y":
		e.yankSpan(st, sp)
		e.finish(false)
	default:
		e.cancel()
	}
}

// linesOp is dd / cc / yy: count lines from the cursor's.
func (e *Editor) linesOp(st *state) {
	t := st.t
	l, last, n := t.lineOf(st.cur), t.lineCount()-1, e.total()
	if n > 1 && l == last {
		e.cancel()
		return
	}
	e.operate(st, e.op, span{from: st.cur, to: t.startOfLine(min(l+n-1, last)), lines: true}, true)
}

// lineRange is the bytes of lines l1..l2 without the final line break.
func (t *text) lineRange(l1, l2 int) (int, int) {
	return t.byteOf(t.startOfLine(l1)), t.byteOf(t.endOfLine(l2))
}

// spanText is the text sp covers and its byte range (for lines: the
// lines' text, without the final line break).
func (t *text) spanText(sp span) (from, to int, s string) {
	if sp.lines {
		from, to = t.lineRange(t.lineOf(sp.from), t.lineOf(sp.to))
	} else {
		from, to = t.byteOf(sp.from), t.byteOf(sp.to)
	}
	return from, to, t.s[from:to]
}

func (e *Editor) deleteSpan(st *state, sp span) {
	t := st.t
	if !sp.lines {
		from, to, s := t.spanText(sp)
		if from == to {
			return
		}
		*e.reg = Register{Text: s}
		st.replace(from, to, "")
		st.cur = st.t.clampNormal(st.t.cellAt(from))
		return
	}
	l1, l2, last := t.lineOf(sp.from), t.lineOf(sp.to), t.lineCount()-1
	_, _, s := t.spanText(sp)
	*e.reg = Register{Text: s, Linewise: true}
	var from, to int
	switch {
	case l2 < last:
		from, to = t.byteOf(t.startOfLine(l1)), t.byteOf(t.startOfLine(l2+1))
	case l1 > 0:
		from, to = t.byteOf(t.endOfLine(l1-1)), len(t.s)
	default:
		from, to = 0, len(t.s)
	}
	st.replace(from, to, "")
	st.cur = st.t.firstNonBlank(min(l1, st.t.lineCount()-1))
}

func (e *Editor) changeSpan(st *state, sp span, rec bool) {
	from, to, s := st.t.spanText(sp)
	if sp.lines || from != to {
		*e.reg = Register{Text: s, Linewise: sp.lines}
	}
	st.replace(from, to, "")
	e.startInsert(st, st.t.cellAt(from), 1, false, rec)
}

func (e *Editor) yankSpan(st *state, sp span) {
	_, _, s := st.t.spanText(sp)
	if sp.lines || s != "" {
		*e.reg = Register{Text: s, Linewise: sp.lines}
	}
	if sp.from < st.cur {
		st.cur = st.t.clampNormal(sp.from)
	}
}

// shorthands are the one-key commands that are an operator and a motion.
var shorthands = map[string][2]string{
	"x": {"d", "l"}, "X": {"d", "h"}, "D": {"d", "$"}, "C": {"c", "$"}, "s": {"c", "l"},
}

// normalCmd runs a NORMAL-mode command that is not a motion or an
// operator.
func (e *Editor) normalCmd(st *state, tok string) {
	t, cur, n := st.t, st.cur, e.total()
	l := t.lineOf(cur)
	switch tok {
	case "x", "X", "D", "C", "s":
		sh := shorthands[tok]
		if sh[0] == "c" && t.emptyLineAt(cur) {
			e.startInsert(st, cur, 1, false, true) // nothing to change: just type
			return
		}
		e.op = sh[0]
		m, _ := e.motion(st, sh[1])
		e.applyMotion(st, m)
	case "S":
		e.op = "c"
		e.linesOp(st)
	case "Y":
		e.op = "y"
		e.linesOp(st)
	case "r":
		e.pend = "r"
	case "~":
		e.toggleCase(st)
	case "J":
		if l >= t.lineCount()-1 {
			e.cancel()
			return
		}
		e.join(st, l, max(1, n-1))
		e.finish(true)
	case "p", "P":
		e.put(st, tok == "p", n)
		e.finish(true)
	case "u":
		e.undoStep(st)
	case "<c-r>":
		e.redoStep(st)
	case ".":
		e.repeat(st)
	case "i":
		e.startInsert(st, cur, n, false, true)
	case "a":
		p := cur
		if !t.emptyLineAt(cur) {
			p++
		}
		e.startInsert(st, p, n, false, true)
	case "I":
		e.startInsert(st, t.firstNonBlank(l), n, false, true)
	case "A":
		e.startInsert(st, t.endOfLine(l), n, false, true)
	case "o", "O":
		at := t.byteOf(t.startOfLine(l))
		if tok == "o" {
			at = t.byteOf(t.endOfLine(l))
		}
		st.replace(at, at, "\n")
		p := at
		if tok == "o" {
			p++
		}
		e.startInsert(st, st.t.cellAt(p), n, true, true)
	case "v":
		e.mode, e.anchor = Visual, t.byteOf(cur)
		e.cancel()
	case "V":
		e.mode, e.anchor = VisualLine, t.byteOf(cur)
		e.cancel()
	default:
		e.cancel()
	}
}

// toggleCase is ~: swap the case of count characters, cursor after them.
func (e *Editor) toggleCase(st *state) {
	t, cur := st.t, st.cur
	le := t.lineEnd(cur)
	if cur >= le {
		e.cancel()
		return
	}
	end := min(le, cur+e.total())
	from, to := t.byteOf(cur), t.byteOf(end)
	repl := strings.Map(swapCase, t.s[from:to])
	st.replace(from, to, repl)
	st.cur = st.t.clampNormal(st.t.cellAt(from + len(repl)))
	e.finish(true)
}

func swapCase(r rune) rune {
	switch {
	case unicode.IsUpper(r):
		return unicode.ToLower(r)
	case unicode.IsLower(r):
		return unicode.ToUpper(r)
	}
	return r
}

// replaceChars is r{c}: replace count characters, all on the cursor's
// line, cursor on the last.
func (e *Editor) replaceChars(st *state, c string) {
	t, cur, n := st.t, st.cur, e.total()
	if cur+n > t.lineEnd(cur) {
		e.cancel()
		return
	}
	from, to := t.byteOf(cur), t.byteOf(cur+n)
	st.replace(from, to, strings.Repeat(c, n))
	st.cur = st.t.cellAt(from + len(c)*(n-1))
	e.finish(true)
}

// join is J: joins lines times into line l, one space between unless the
// line ends in a blank, the next starts with ')' or is empty; the
// cursor goes to the last join.
func (e *Editor) join(st *state, l, times int) {
	for range times {
		t := st.t
		if l >= t.lineCount()-1 {
			break
		}
		nl := t.endOfLine(l)
		k := nl + 1
		ke := t.endOfLine(l + 1)
		for k < ke && t.class(k, false) == 0 {
			k++
		}
		sep := " "
		if k >= ke || t.cell(k) == ")" || nl == t.startOfLine(l) || t.class(nl-1, false) == 0 {
			sep = ""
		}
		at := t.byteOf(nl)
		st.replace(at, t.byteOf(k), sep)
		st.cur = st.t.cellAt(at)
	}
	st.cur = st.t.clampNormal(st.cur)
}

// put is p / P: the register count times, after or before the cursor
// (below or above the line for a linewise register).
func (e *Editor) put(st *state, after bool, n int) {
	r := *e.reg
	if r.Text == "" && !r.Linewise {
		return
	}
	t, cur := st.t, st.cur
	if r.Linewise {
		block := repeatCapped(r.Text, "\n", n)
		l := t.lineOf(cur)
		var at, first int
		if after {
			at = t.byteOf(t.endOfLine(l))
			st.replace(at, at, "\n"+block)
			first = at + 1
		} else {
			at = t.byteOf(t.startOfLine(l))
			st.replace(at, at, block+"\n")
			first = at
		}
		st.cur = st.t.firstNonBlank(st.t.lineOf(st.t.cellAt(first)))
		return
	}
	block := repeatCapped(r.Text, "", n)
	at := t.byteOf(cur)
	if after && !t.emptyLineAt(cur) {
		at = t.byteOf(cur + 1)
	}
	st.replace(at, at, block)
	if strings.Contains(r.Text, "\n") {
		st.cur = st.t.cellAt(at)
	} else {
		st.cur = st.t.cellAt(at+len(block)) - 1
	}
	st.cur = st.t.clampNormal(st.cur)
}

// repeatCapped is n copies of s joined by sep, stopping once the result
// reaches maxRepeatBytes.
func repeatCapped(s, sep string, n int) string {
	var b strings.Builder
	for i := range n {
		if b.Len() >= maxRepeatBytes {
			break
		}
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(s)
	}
	return b.String()
}

// visualSpan is the selection: charwise from anchor to cursor
// inclusive (an empty line's selected line break included, as in vim),
// or every line between them.
func (e *Editor) visualSpan(st *state) span {
	a := st.t.cellAt(e.anchor)
	s, x := min(a, st.cur), max(a, st.cur)
	if e.mode == VisualLine {
		return span{from: s, to: x, lines: true}
	}
	return span{from: s, to: min(x+1, st.t.n())}
}

// endVisual leaves VISUAL for a change made from it, which is not
// repeatable: `.` afterwards does nothing.
func (e *Editor) endVisual() {
	e.mode = Normal
	e.dot = nil
}

func (e *Editor) visualOperator(st *state, op string) {
	sp := e.visualSpan(st)
	if op == "y" {
		e.mode = Normal
	} else {
		e.endVisual()
	}
	e.operate(st, op, sp, false)
	if op == "y" {
		st.cur = st.t.clampNormal(sp.from)
	}
}

// visualCmd runs a VISUAL / V-LINE command other than a motion or d c y.
func (e *Editor) visualCmd(st *state, tok string) {
	sp := e.visualSpan(st)
	lines := span{from: sp.from, to: max(sp.from, sp.to-1), lines: true}
	if sp.lines {
		lines = sp
	}
	switch tok {
	case "v", "V":
		want := Visual
		if tok == "V" {
			want = VisualLine
		}
		if e.mode == want {
			e.mode = Normal
		} else {
			e.mode = want
		}
		e.cancel()
	case "o":
		a := st.t.cellAt(e.anchor)
		e.anchor = st.t.byteOf(st.cur)
		st.cur = a
		e.cancel()
	case "x":
		e.visualOperator(st, "d")
	case "s":
		e.visualOperator(st, "c")
	case "X", "D":
		e.endVisual()
		e.operate(st, "d", lines, false)
	case "C", "S", "R":
		e.endVisual()
		e.operate(st, "c", lines, false)
	case "Y":
		e.mode = Normal
		e.operate(st, "y", lines, false)
		st.cur = st.t.clampNormal(sp.from)
	case "p", "P":
		e.endVisual()
		e.visualPut(st, sp)
		e.cancel()
	case "~", "u", "U":
		e.endVisual()
		from, to, s := st.t.spanText(sp)
		var repl string
		switch tok {
		case "~":
			repl = strings.Map(swapCase, s)
		case "u":
			repl = strings.ToLower(s)
		default:
			repl = strings.ToUpper(s)
		}
		st.replace(from, to, repl)
		st.cur = st.t.clampNormal(st.t.cellAt(from))
		e.cancel()
	case "J":
		e.endVisual()
		l1, l2 := st.t.lineOf(lines.from), st.t.lineOf(lines.to)
		if l1 >= st.t.lineCount()-1 {
			e.cancel()
			return
		}
		e.join(st, l1, max(1, l2-l1))
		e.cancel()
	case "r":
		e.pend = "r"
	default:
		e.cancel()
	}
}

// visualReplace is r{c} in VISUAL: every selected character becomes c.
func (e *Editor) visualReplace(st *state, c string) {
	sp := e.visualSpan(st)
	e.endVisual()
	from, to, _ := st.t.spanText(sp)
	t := st.t
	var b strings.Builder
	for i := t.cellAt(from); i < t.n() && t.byteOf(i) < to; i++ {
		if t.isNL(i) {
			b.WriteString(t.cell(i))
		} else {
			b.WriteString(c)
		}
	}
	st.replace(from, to, b.String())
	st.cur = st.t.clampNormal(st.t.cellAt(from))
	e.cancel()
}

// visualPut is p in VISUAL: the selection is replaced by the register,
// and the register takes the replaced text, as in vim.
func (e *Editor) visualPut(st *state, sp span) {
	r := *e.reg
	if r.Text == "" && !r.Linewise {
		return
	}
	from, to, old := st.t.spanText(sp)
	repl := r.Text
	if r.Linewise && !sp.lines {
		repl = "\n" + r.Text + "\n"
	}
	*e.reg = Register{Text: old, Linewise: sp.lines}
	st.replace(from, to, repl)
	if !r.Linewise && !sp.lines && !strings.Contains(repl, "\n") && repl != "" {
		st.cur = st.t.clampNormal(st.t.cellAt(from+len(repl)) - 1)
		return
	}
	at := from
	if r.Linewise && !sp.lines {
		at++
	}
	st.cur = st.t.firstNonBlank(st.t.lineOf(st.t.cellAt(at)))
}

// selectObject makes a text object the visual selection.
func (e *Editor) selectObject(st *state, sp span) {
	if sp.lines {
		e.mode = VisualLine
		e.anchor = st.t.byteOf(sp.from)
		st.cur = st.t.clampNormal(sp.to)
	} else {
		if sp.to <= sp.from {
			e.cancel()
			return
		}
		e.mode = Visual
		e.anchor = st.t.byteOf(sp.from)
		st.cur = sp.to - 1
	}
	e.cancel()
}
