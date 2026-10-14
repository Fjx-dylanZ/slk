package vimedit

// kind is how an operator treats a motion's end: up to it, through it,
// or every line between.
type kind uint8

const (
	exclusive kind = iota
	inclusive
	linewise
)

type motion struct {
	pos      int
	kind     kind
	ok       bool
	keepWant bool // j/k: keep aiming at want
	want     int
	wantEnd  bool // $: aim at the end of every line
}

func fail() motion { return motion{} }

// span is the text an operator acts on: cells [from, to), or with lines
// every line from from's through to's.
type span struct {
	from, to int
	lines    bool
}

// motion computes the motion tok from the cursor, reporting false when
// tok is not a motion key.
func (e *Editor) motion(st *state, tok string) (motion, bool) {
	t, cur, n := st.t, st.cur, e.total()
	l := t.lineOf(cur)
	switch tok {
	case "h":
		ls := t.lineStart(cur)
		if cur <= ls {
			return fail(), true
		}
		return motion{pos: max(ls, cur-n), ok: true}, true
	case "l":
		last := t.lineEnd(cur)
		if e.op == "" {
			last-- // NORMAL and VISUAL stop on the last character
		}
		if cur >= last {
			return fail(), true
		}
		return motion{pos: min(last, cur+n), ok: true}, true
	case "j", "k":
		target := l + n
		if tok == "k" {
			target = l - n
		}
		if (tok == "j" && l >= t.lineCount()-1) || (tok == "k" && l == 0) {
			return fail(), true
		}
		target = max(0, min(target, t.lineCount()-1))
		want := e.wantCol(st)
		return motion{pos: t.cellAtCol(target, want), kind: linewise, ok: true, keepWant: true, want: want}, true
	case "w", "W", "e", "E":
		return e.wordMotion(t, cur, n, tok), true
	case "b", "B":
		return motion{pos: t.bckWord(cur, n, tok == "B", false), ok: true}, true
	case "0", "<home>":
		return motion{pos: t.lineStart(cur), ok: true}, true
	case "^":
		return motion{pos: t.firstNonBlank(l), ok: true}, true
	case "$":
		target := min(l+n-1, t.lineCount()-1)
		s, end := t.startOfLine(target), t.endOfLine(target)
		pos := s
		if end > s {
			pos = end - 1
		}
		return motion{pos: pos, kind: inclusive, ok: true, wantEnd: true}, true
	case "G":
		return e.gotoLine(st, t.lineCount()-1), true
	case ";", ",":
		cmd := e.find.cmd
		if cmd == "" {
			return fail(), true
		}
		if tok == "," {
			cmd = map[string]string{"f": "F", "F": "f", "t": "T", "T": "t"}[cmd]
		}
		return e.findMotion(st, cmd, e.find.target, true), true
	}
	return motion{}, false
}

// gotoLine is G / gg: the counted line, else line def, at its first
// non-blank.
func (e *Editor) gotoLine(st *state, def int) motion {
	l := def
	if e.hasCount() {
		l = e.total() - 1
	}
	l = max(0, min(l, st.t.lineCount()-1))
	return motion{pos: st.t.firstNonBlank(l), kind: linewise, ok: true}
}

// findMotion is f/F/t/T: the count-th target on the cursor's line.
// repeat (; and ,) with no count keeps t/T from sticking on the match
// next to the cursor; a counted repeat does not skip it (vim's cpo-;).
func (e *Editor) findMotion(st *state, cmd, target string, repeat bool) motion {
	t, cur, n := st.t, st.cur, e.total()
	ls, le := t.lineStart(cur), t.lineEnd(cur)
	p := cur
	switch cmd {
	case "f", "t":
		if cmd == "t" && repeat && n == 1 {
			p++
		}
		for range n {
			p++
			for p < le && t.cell(p) != target {
				p++
			}
			if p >= le {
				return fail()
			}
		}
		if cmd == "t" {
			p--
		}
		return motion{pos: p, kind: inclusive, ok: true}
	default: // F, T
		if cmd == "T" && repeat && n == 1 {
			p--
		}
		for range n {
			p--
			for p >= ls && t.cell(p) != target {
				p--
			}
			if p < ls {
				return fail()
			}
		}
		if cmd == "T" {
			p++
		}
		return motion{pos: p, ok: true}
	}
}

// wordMotion is vim's nv_wordcmd: w W e E, with cw acting as ce and the
// cursor kept off a line end.
func (e *Editor) wordMotion(t *text, cur, n int, tok string) motion {
	big := tok == "W" || tok == "E"
	wordEnd := tok == "e" || tok == "E"
	m := motion{ok: true}
	if wordEnd {
		m.kind = inclusive
	}
	stop := false
	if !wordEnd && e.op == "c" && cur < t.n() && !t.isNL(cur) {
		if t.class(cur, big) != 0 {
			wordEnd = true
			m.kind = inclusive
		}
		stop = true
	}
	var p int
	if wordEnd {
		p = t.endWord(cur, n, big, stop)
	} else {
		p = t.fwdWord(cur, n, big, e.op != "")
	}
	// adjust_cursor: never rest on the end of a non-empty line.
	if p > cur && p > t.lineStart(p) && (p >= t.n() || t.isNL(p)) {
		p--
		m.kind = inclusive
	}
	m.pos = p
	return m
}

// fwdWord is vim's fwd_word. eol (an operator is pending) stops the last
// word at the end of its line.
func (t *text) fwdWord(p, count int, big, eol bool) int {
	for count > 0 {
		count--
		sclass := t.class(p, big)
		lastLine := t.lineEnd(p) == t.n()
		i := t.inc(&p)
		if i == -1 || (i >= 1 && lastLine) {
			return p
		}
		if i >= 1 && eol && count == 0 {
			return p
		}
		if sclass != 0 {
			for t.class(p, big) == sclass {
				i = t.inc(&p)
				if i == -1 || (i >= 1 && eol && count == 0) {
					return p
				}
			}
		}
		for t.class(p, big) == 0 {
			if t.emptyLineAt(p) {
				break
			}
			i = t.inc(&p)
			if i == -1 || (i >= 1 && eol && count == 0) {
				return p
			}
		}
	}
	return p
}

// endWord is vim's end_word (empty lines never stop it). stop: cw, which
// on the last character of a word changes just that character.
func (t *text) endWord(p, count int, big, stop bool) int {
	for count > 0 {
		count--
		sclass := t.class(p, big)
		if t.inc(&p) == -1 {
			return p
		}
		switch {
		case sclass != 0 && t.class(p, big) == sclass:
			if t.skipChars(&p, sclass, big, 1) {
				return p
			}
		case !stop || sclass == 0:
			for t.class(p, big) == 0 {
				if t.inc(&p) == -1 {
					return p
				}
			}
			if t.skipChars(&p, t.class(p, big), big, 1) {
				return p
			}
		}
		t.dec(&p)
		stop = false
	}
	return p
}

// bckWord is vim's bck_word.
func (t *text) bckWord(p, count int, big, stop bool) int {
	for count > 0 {
		count--
		sclass := t.class(p, big)
		if t.dec(&p) == -1 {
			return p
		}
		if !stop || sclass == t.class(p, big) || sclass == 0 {
			emptyLine := false
			for t.class(p, big) == 0 {
				if t.emptyLineAt(p) {
					emptyLine = true
					break
				}
				if t.dec(&p) == -1 {
					return p
				}
			}
			if emptyLine {
				stop = false
				continue
			}
			if t.skipChars(&p, t.class(p, big), big, -1) {
				return p
			}
		}
		t.inc(&p)
		stop = false
	}
	return p
}

// bckendWord is vim's bckend_word (ge), never stopping at line ends.
func (t *text) bckendWord(p, count int, big bool) int {
	for count > 0 {
		count--
		sclass := t.class(p, big)
		if t.dec(&p) == -1 {
			return p
		}
		if sclass != 0 {
			for t.class(p, big) == sclass {
				if t.dec(&p) == -1 {
					return p
				}
			}
		}
		for t.class(p, big) == 0 {
			if t.emptyLineAt(p) {
				break
			}
			if t.dec(&p) == -1 {
				return p
			}
		}
	}
	return p
}

// motionSpan is the text an operator covers from cur along m, with
// vim's rule that an exclusive motion ending at the start of a later
// line stops at the end of the line before (and covers whole lines when
// it began in the indent).
func (t *text) motionSpan(cur int, m motion) span {
	s, x := cur, m.pos
	if x < s {
		s, x = x, s
	}
	switch m.kind {
	case linewise:
		return span{from: s, to: x, lines: true}
	case inclusive:
		if x < t.n() && !t.isNL(x) {
			x++
		}
		return span{from: s, to: x}
	}
	if x > s && x == t.lineStart(x) && t.lineOf(x) > t.lineOf(s) {
		if t.inIndent(s) {
			return span{from: s, to: x - 1, lines: true}
		}
		x--
	}
	return span{from: s, to: x}
}
