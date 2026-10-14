package vimedit

import (
	"math"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// text is one snapshot of the buffer split into grapheme clusters
// ("cells"). Every position the editor works with is a cell index in
// [0, n]: n is the end of the buffer, and a newline is a cell of its own,
// so the newline that ends a line doubles as vim's "NUL past the end of
// the line" position. Working in cells is what keeps every motion and
// operator from splitting an emoji, a flag or a combining sequence.
type text struct {
	s     string
	off   []int // byte offset of each cell; off[n] == len(s)
	width []int // display width of each cell (uniseg, like the textarea)
	lines []int // cell index of each line's first cell
}

func newText(s string) *text {
	t := &text{s: s, lines: []int{0}}
	state := -1
	pos := 0
	for rest := s; len(rest) > 0; {
		var cluster string
		var w int
		cluster, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		t.off = append(t.off, pos)
		t.width = append(t.width, w)
		pos += len(cluster)
		if cluster == "\n" || cluster == "\r\n" {
			t.lines = append(t.lines, len(t.off))
		}
	}
	t.off = append(t.off, len(s))
	return t
}

// n is the number of cells; position n is the end of the buffer.
func (t *text) n() int { return len(t.off) - 1 }

// cell is the text of cell i ("" at or past the end).
func (t *text) cell(i int) string {
	if i < 0 || i >= t.n() {
		return ""
	}
	return t.s[t.off[i]:t.off[i+1]]
}

// isNL reports whether cell i is a line break.
func (t *text) isNL(i int) bool {
	c := t.cell(i)
	return c == "\n" || c == "\r\n"
}

// byteOf is the byte offset of cell i, clamped to the buffer.
func (t *text) byteOf(i int) int {
	if i <= 0 {
		return 0
	}
	if i >= t.n() {
		return len(t.s)
	}
	return t.off[i]
}

// cellAt is the cell containing byte offset b: an offset inside a
// cluster snaps to the cluster's start.
func (t *text) cellAt(b int) int {
	if b <= 0 {
		return 0
	}
	if b >= len(t.s) {
		return t.n()
	}
	return sort.Search(len(t.off), func(i int) bool { return t.off[i] > b }) - 1
}

func (t *text) lineCount() int { return len(t.lines) }

// lineOf is the 0-based line holding position i.
func (t *text) lineOf(i int) int {
	return sort.Search(len(t.lines), func(l int) bool { return t.lines[l] > i }) - 1
}

// startOfLine is line l's first cell.
func (t *text) startOfLine(l int) int { return t.lines[l] }

// endOfLine is the line break ending line l, or n for the last line.
func (t *text) endOfLine(l int) int {
	if l+1 < len(t.lines) {
		return t.lines[l+1] - 1
	}
	return t.n()
}

func (t *text) lineStart(i int) int { return t.startOfLine(t.lineOf(i)) }
func (t *text) lineEnd(i int) int   { return t.endOfLine(t.lineOf(i)) }

// emptyLineAt reports whether i is the start of an empty line (vim's
// "column 0 of an empty line").
func (t *text) emptyLineAt(i int) bool {
	return i == t.lineStart(i) && (i >= t.n() || t.isNL(i))
}

// class is vim's cls(): 0 for blanks (and line ends), otherwise the
// character class of cell i, every non-blank being one class for a WORD.
func (t *text) class(i int, big bool) int {
	if i < 0 || i >= t.n() {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(t.s[t.off[i]:])
	if unicode.IsSpace(r) {
		return 0
	}
	if big {
		return 1
	}
	return runeClass(r)
}

// runeClass follows vim's utf_class(): punctuation 1, word characters 2,
// emoji 3, and the big CJK scripts as classes of their own so a word
// motion stops where the script changes.
func runeClass(r rune) int {
	switch {
	case r < 0x100:
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return 2
		}
		return 1
	case isEmoji(r):
		return 3
	case unicode.Is(unicode.Han, r):
		return 0x4e00
	case unicode.Is(unicode.Hiragana, r):
		return 0x3040
	case unicode.Is(unicode.Katakana, r):
		return 0x30a0
	case unicode.Is(unicode.Hangul, r):
		return 0xac00
	case unicode.IsPunct(r) || unicode.IsSymbol(r):
		return 1
	}
	return 2
}

func isEmoji(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF)
}

// inc is vim's inc(): it moves p one position forward and reports 0
// within a line, 2 onto the line's end, 1 onto the next line's start and
// -1 (not moving) at the end of the buffer.
func (t *text) inc(p *int) int {
	if *p >= t.n() {
		return -1
	}
	if t.isNL(*p) {
		*p++
		return 1
	}
	*p++
	if *p >= t.n() || t.isNL(*p) {
		return 2
	}
	return 0
}

// dec is vim's dec(): one position back, 1 when that crossed onto the
// previous line's end, -1 (not moving) at the start of the buffer.
func (t *text) dec(p *int) int {
	if *p <= 0 {
		return -1
	}
	*p--
	if t.isNL(*p) {
		return 1
	}
	return 0
}

// clampNormal moves a position NORMAL mode cannot rest on (the line
// break of a non-empty line, or the end of a non-empty last line) onto
// the line's last character.
func (t *text) clampNormal(p int) int {
	p = max(0, min(p, t.n()))
	if ls := t.lineStart(p); p > ls && (p >= t.n() || t.isNL(p)) {
		p--
	}
	return p
}

// firstNonBlank is line l's first non-blank cell; on a line of blanks
// its last cell, and on an empty line its start.
func (t *text) firstNonBlank(l int) int {
	s, e := t.startOfLine(l), t.endOfLine(l)
	for i := s; i < e; i++ {
		if t.class(i, false) != 0 {
			return i
		}
	}
	if e > s {
		return e - 1
	}
	return s
}

// inIndent reports whether only blanks precede p on its line.
func (t *text) inIndent(p int) bool {
	for i := t.lineStart(p); i < p; i++ {
		if t.class(i, false) != 0 {
			return false
		}
	}
	return true
}

// colOf is p's display column within its line.
func (t *text) colOf(p int) int {
	c := 0
	for i := t.lineStart(p); i < p && i < t.n(); i++ {
		c += t.width[i]
	}
	return c
}

// endCol aims j/k at the end of the line ($).
const endCol = math.MaxInt

// cellAtCol is the cell of line l that covers display column col; past
// the end of the line it is the last character.
func (t *text) cellAtCol(l, col int) int {
	s, e := t.startOfLine(l), t.endOfLine(l)
	if s == e {
		return s
	}
	c := 0
	for i := s; i < e; i++ {
		if c+t.width[i] > col {
			return i
		}
		c += t.width[i]
	}
	return e - 1
}

// skipChars is vim's skip_chars(): it moves p past cells of class cls in
// direction dir and reports whether it hit an end of the buffer.
func (t *text) skipChars(p *int, cls int, big bool, dir int) bool {
	for t.class(*p, big) == cls {
		var r int
		if dir > 0 {
			r = t.inc(p)
		} else {
			r = t.dec(p)
		}
		if r == -1 {
			return true
		}
	}
	return false
}
