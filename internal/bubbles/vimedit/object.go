package vimedit

// object is the text object obj (the key after i or a) around cell cur;
// inner selects the i variant. ok is false when there is none.
func (t *text) object(cur int, obj string, inner bool) (span, bool) {
	switch obj {
	case "w":
		return t.wordObject(cur, false, inner)
	case "W":
		return t.wordObject(cur, true, inner)
	case `"`, "'", "`":
		return t.quoteObject(cur, obj, inner)
	case "(", ")", "b":
		return t.blockObject(cur, "(", ")", inner)
	case "[", "]":
		return t.blockObject(cur, "[", "]", inner)
	case "{", "}", "B":
		return t.blockObject(cur, "{", "}", inner)
	case "<", ">":
		return t.blockObject(cur, "<", ">", inner)
	}
	return span{}, false
}

// wordObject is iw/aw (iW/aW): the run of one class around the cursor
// on its line; aw adds the blanks after it, or before it when there are
// none after. On blanks, aw is the blanks and the word that follows.
func (t *text) wordObject(cur int, big, inner bool) (span, bool) {
	ls, le := t.lineStart(cur), t.lineEnd(cur)
	if le == ls {
		return span{}, false
	}
	cur = min(cur, le-1)
	cls := t.class(cur, big)
	s, e := cur, cur+1
	for s > ls && t.class(s-1, big) == cls {
		s--
	}
	for e < le && t.class(e, big) == cls {
		e++
	}
	if inner {
		return span{from: s, to: e}, true
	}
	if cls != 0 {
		trail := e
		for trail < le && t.class(trail, big) == 0 {
			trail++
		}
		if trail > e {
			return span{from: s, to: trail}, true
		}
		for s > ls && t.class(s-1, big) == 0 {
			s--
		}
		return span{from: s, to: e}, true
	}
	if e < le {
		next := t.class(e, big)
		for e < le && t.class(e, big) == next {
			e++
		}
	}
	return span{from: s, to: e}, true
}

// quoteObject is i"/a" and friends, on the cursor's line, found the way
// vim's current_quote does: on a quote, pair quotes from the start of
// the line; elsewhere, the quote before the cursor (else the first after
// it) and the next one. A backslash escapes a quote.
func (t *text) quoteObject(cur int, q string, inner bool) (span, bool) {
	ls, le := t.lineStart(cur), t.lineEnd(cur)
	if cur >= le {
		return span{}, false
	}
	escaped := func(i int) bool {
		n := 0
		for j := i - 1; j >= ls && t.cell(j) == `\`; j-- {
			n++
		}
		return n%2 == 1
	}
	next := func(i int) int {
		for ; i < le; i++ {
			if t.cell(i) == q && !escaped(i) {
				return i
			}
		}
		return -1
	}
	prev := func(i int) int {
		for ; i >= ls; i-- {
			if t.cell(i) == q && !escaped(i) {
				return i
			}
		}
		return -1
	}
	var start, end int
	if t.cell(cur) == q && !escaped(cur) {
		for i := ls; ; {
			start = next(i)
			if start < 0 || start > cur {
				return span{}, false
			}
			end = next(start + 1)
			if end < 0 {
				return span{}, false
			}
			if start <= cur && cur <= end {
				break
			}
			i = end + 1
		}
	} else {
		start = prev(cur - 1)
		if start < 0 {
			start = next(cur)
		}
		if start < 0 {
			return span{}, false
		}
		end = next(start + 1)
		if end < 0 {
			return span{}, false
		}
	}
	if inner {
		return span{from: start + 1, to: end}, true
	}
	e := end + 1
	for e < le && t.class(e, false) == 0 {
		e++
	}
	if e > end+1 {
		return span{from: start, to: e}, true
	}
	s := start
	for s > ls && t.class(s-1, false) == 0 {
		s--
	}
	return span{from: s, to: end + 1}, true
}

// blockObject is i(/a( and friends: the innermost open..close pair
// around the cursor, across lines. When the open bracket ends its line
// and the close bracket starts its own (after blanks), the inner object
// is the lines between, linewise, as in vim: `di{` keeps the braces,
// `ci{` leaves an empty line to type on.
func (t *text) blockObject(cur int, open, close string, inner bool) (span, bool) {
	start := -1
	if t.cell(cur) == open {
		start = cur
	} else {
		depth := 0
		for i := cur - 1; i >= 0; i-- {
			switch t.cell(i) {
			case close:
				depth++
			case open:
				if depth == 0 {
					start = i
				} else {
					depth--
				}
			}
			if start >= 0 {
				break
			}
		}
	}
	if start < 0 {
		return span{}, false
	}
	end, depth := -1, 0
	for i := start + 1; i < t.n() && end < 0; i++ {
		switch t.cell(i) {
		case open:
			depth++
		case close:
			if depth == 0 {
				end = i
			} else {
				depth--
			}
		}
	}
	if end < 0 {
		return span{}, false
	}
	if !inner {
		return span{from: start, to: end + 1}, true
	}
	s, e := start+1, end
	if s < e && t.isNL(s) {
		closeLine := t.lineStart(e)
		if closeLine > s+1 && t.blank(closeLine, e) {
			return span{from: s + 1, to: closeLine - 1, lines: true}, true
		}
		// The open bracket ends its line: the object starts on the
		// next one, as in vim (`di{` on "{\n  foo }" keeps "{\n").
		s++
	}
	return span{from: s, to: e}, true
}

// blank reports whether cells [from, to) are all blanks.
func (t *text) blank(from, to int) bool {
	for i := from; i < to; i++ {
		if t.class(i, false) != 0 {
			return false
		}
	}
	return true
}
