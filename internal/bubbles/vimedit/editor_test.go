package vimedit

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

// The tests write a buffer as its text with "|" in front of the cursor's
// cell ("foo |bar": the cursor is on b), and keys as a string where each
// character is a key and <esc> <bs> <del> <left> <right> <up> <down>
// <home> <end> <c-r> name the others.

func buf(s string) Buffer {
	i := strings.Index(s, "|")
	if i < 0 {
		panic("buffer without cursor marker: " + s)
	}
	return Buffer{Text: s[:i] + s[i+1:], Cursor: i}
}

func show(b Buffer) string { return b.Text[:b.Cursor] + "|" + b.Text[b.Cursor:] }

var named = map[string]KeyName{
	"<esc>": KeyEsc, "<bs>": KeyBackspace, "<del>": KeyDelete, "<left>": KeyLeft,
	"<right>": KeyRight, "<up>": KeyUp, "<down>": KeyDown, "<home>": KeyHome,
	"<end>": KeyEnd, "<c-r>": KeyCtrlR,
}

func parseKeys(s string) []Key {
	var ks []Key
	for s != "" {
		if s[0] == '<' {
			if j := strings.IndexByte(s, '>'); j > 0 {
				if n, ok := named[s[:j+1]]; ok {
					ks = append(ks, Key{Name: n})
					s = s[j+1:]
					continue
				}
			}
		}
		t := newText(s)
		c := t.cell(0)
		ks = append(ks, Key{Text: c})
		s = s[len(c):]
	}
	return ks
}

// host feeds keys to e the way a text input would: a key the Editor
// does not take in INSERT is typed (or, for <bs> <left> <right>, edits
// and moves) by the host itself.
func host(e *Editor, b Buffer, keys string) Buffer {
	for _, k := range parseKeys(keys) {
		nb, ok := e.Handle(b, k)
		if ok {
			b = nb
			continue
		}
		if e.Mode() != Insert {
			continue // Esc in idle NORMAL: the host would leave
		}
		t := newText(b.Text)
		p := t.cellAt(b.Cursor)
		switch k.Name {
		case KeyBackspace:
			if p > 0 {
				b = Buffer{Text: b.Text[:t.byteOf(p-1)] + b.Text[b.Cursor:], Cursor: t.byteOf(p - 1)}
			}
		case KeyLeft:
			b.Cursor = t.byteOf(max(0, p-1))
		case KeyRight:
			b.Cursor = t.byteOf(min(t.n(), p+1))
		case KeyNone:
			b = Buffer{Text: b.Text[:b.Cursor] + k.Text + b.Text[b.Cursor:], Cursor: b.Cursor + len(k.Text)}
		}
	}
	return b
}

func normal(reg *Register) *Editor { return New(reg) }

type vimCase struct {
	name, start, keys, want string
}

func runCases(t *testing.T, cases []vimCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := normal(nil)
			got := show(host(e, buf(tc.start), tc.keys))
			if got != tc.want {
				t.Errorf("%q + %q = %q, want %q", tc.start, tc.keys, got, tc.want)
			}
		})
	}
}

func TestMotionsLeftRight(t *testing.T) {
	runCases(t, []vimCase{
		{"h", "ab|c", "h", "a|bc"},
		{"h at line start", "x\n|abc", "h", "x\n|abc"},
		{"count h", "abc|d", "2h", "a|bcd"},
		{"count h clamps", "ab|c", "5h", "|abc"},
		{"l", "|abc", "l", "a|bc"},
		{"l stops on last char", "ab|c", "l", "ab|c"},
		{"count l clamps", "|abc", "5l", "ab|c"},
		{"l does not cross lines", "a|b\ncd", "3l", "a|b\ncd"},
		{"space and backspace", "|abc", " <bs> ", "a|bc"},
		{"arrows", "|abc\ndef", "<right><down><left><up>", "|abc\ndef"},
		{"0", "  fo|o", "0", "|  foo"},
		{"home", "  fo|o", "<home>", "|  foo"},
		{"^", "|  foo", "^", "  |foo"},
		{"^ on blank line", "|   \nx", "^", "  | \nx"},
		{"$", "|  foo", "$", "  fo|o"},
		{"end", "|  foo", "<end>", "  fo|o"},
		{"count $", "|ab\ncd\nef", "2$", "ab\nc|d\nef"},
		{"0 after count digit is a digit", "|abcdefghijkl", "10l", "abcdefghij|kl"},
	})
}

func TestMotionsUpDown(t *testing.T) {
	runCases(t, []vimCase{
		{"j to shorter line", "ab|cd\nef", "j", "abcd\ne|f"},
		{"j then k keeps the column", "ab|cd\nef", "jk", "ab|cd\nef"},
		{"k", "abc\n|x", "k", "|abc\nx"},
		{"count j", "|a\nb\nc\nd", "2j", "a\nb\n|c\nd"},
		{"count j clamps", "|a\nb\nc\nd", "5j", "a\nb\nc\n|d"},
		{"j on last line", "a\n|b", "j", "a\n|b"},
		{"k on first line", "|a\nb", "k", "|a\nb"},
		{"through an empty line", "ab|c\n\nxyz", "j", "abc\n|\nxyz"},
		{"column survives an empty line", "ab|c\n\nxyz", "jj", "abc\n\nxy|z"},
		{"$ sticks to line ends", "a|b\nabcdef", "$j", "ab\nabcde|f"},
		{"CJK display column down", "日本|語\nabcdef", "j", "日本語\nabcd|ef"},
		{"CJK display column up", "abcd|ef\n日本語", "j", "abcdef\n日本|語"},
		{"gg", "a\nb\n|c", "gg", "|a\nb\nc"},
		{"gg to first non-blank", "  x\n|y", "gg", "  |x\ny"},
		{"G", "|a\nb\nc", "G", "a\nb\n|c"},
		{"count G", "|a\nb\nc", "2G", "a\n|b\nc"},
		{"count gg", "a\nb\n|c", "2gg", "a\n|b\nc"},
		{"count G clamps", "|a\nb", "9G", "a\n|b"},
	})
}

func TestMotionsWords(t *testing.T) {
	runCases(t, []vimCase{
		{"w", "|foo bar", "w", "foo |bar"},
		{"w stops at punctuation", "|foo.bar", "w", "foo|.bar"},
		{"w after punctuation", "foo|.bar", "w", "foo.|bar"},
		{"W skips punctuation", "|foo.bar baz", "W", "foo.bar |baz"},
		{"w crosses lines", "foo |bar\nbaz", "w", "foo bar\n|baz"},
		{"w stops on an empty line", "|foo\n\nbar", "w", "foo\n|\nbar"},
		{"w leaves an empty line", "|foo\n\nbar", "ww", "foo\n\n|bar"},
		{"w on the last word goes to its end", "foo |bar", "w", "foo ba|r"},
		{"count w", "|a b c d", "3w", "a b c |d"},
		{"w stops where CJK starts", "|foo日本語bar", "w", "foo|日本語bar"},
		{"w over a CJK run", "foo|日本語bar", "w", "foo日本語|bar"},
		{"w over emoji", "|hi😀😀 there", "w", "hi|😀😀 there"},
		{"w after emoji", "hi|😀😀 there", "w", "hi😀😀 |there"},
		{"b", "foo bar |baz", "b", "foo |bar baz"},
		{"b to the start", "foo |bar", "b", "|foo bar"},
		{"b crosses lines", "foo\n|bar", "b", "|foo\nbar"},
		{"b stops on an empty line", "foo\n\n|bar", "b", "foo\n|\nbar"},
		{"b over punctuation", "foo.ba|r", "b", "foo.|bar"},
		{"B", "foo.ba|r", "B", "|foo.bar"},
		{"b at buffer start", "|foo", "b", "|foo"},
		{"e", "|foo bar", "e", "fo|o bar"},
		{"e from a word end", "fo|o bar", "e", "foo ba|r"},
		{"e crosses lines", "fo|o\nbar", "e", "foo\nba|r"},
		{"e at the buffer end", "fo|o", "e", "fo|o"},
		{"E", "|a.b c", "E", "a.|b c"},
		{"ge", "foo ba|r", "ge", "fo|o bar"},
		{"ge crosses lines", "foo\n|bar", "ge", "fo|o\nbar"},
		{"gE", "a.b |c", "gE", "a.|b c"},
	})
}

func TestMotionsFind(t *testing.T) {
	runCases(t, []vimCase{
		{"f", "|a,b,c", "f,", "a|,b,c"},
		{"f then ;", "|a,b,c", "f,;", "a,b|,c"},
		{"f ; ,", "|a,b,c", "f,;,", "a|,b,c"},
		{"count f", "|a,b,c", "2f,", "a,b|,c"},
		{"t", "|ab,c", "t,", "a|b,c"},
		{"t next to the target", "|a,b,c", "t,", "|a,b,c"},
		{"; after t skips the adjacent match", "|a,b,c", "t,;", "a,|b,c"},
		{"counted ; after t does not skip the adjacent match", "|a,b,c,d", "t,2;", "a,|b,c,d"},
		{"counted ; after T does not skip the adjacent match", "a,b,c,|d", "T,2;", "a,b,|c,d"},
		{"1; after t skips like ;", "|a,b,c", "t,1;", "a,|b,c"},
		{"F", "a,b,|c", "F,", "a,b|,c"},
		{"T", "a,bb|c", "T,", "a,|bbc"},
		{"not found", "|abc", "fz", "|abc"},
		{"stays on its line", "|abc\nz", "fz", "|abc\nz"},
		{"emoji target", "|a😀b", "f😀", "a|😀b"},
		{"esc cancels", "|a,b", "f<esc>;", "|a,b"},
	})
}

func TestOperators(t *testing.T) {
	runCases(t, []vimCase{
		{"dw", "|foo bar", "dw", "|bar"},
		{"dw on the last word", "foo |bar", "dw", "foo| "},
		{"dw keeps the line break", "foo |bar\nbaz", "dw", "foo| \nbaz"},
		{"d2w", "|a b c", "d2w", "|c"},
		{"2dw", "|a b c", "2dw", "|c"},
		{"counts multiply", "|a b c d e f g", "2d3w", "|g"},
		{"dw on an empty line deletes it", "foo\n|\nbar", "dw", "foo\n|bar"},
		{"de", "|foo bar", "de", "| bar"},
		{"db", "foo |bar", "db", "|bar"},
		{"db at line start is linewise from the indent", "foo\n|bar", "db", "|bar"},
		{"D", "fo|o bar", "D", "f|o"},
		{"2D", "a|bc\ndef\nghi", "2D", "|a\nghi"},
		{"d0", "abc|def", "d0", "|def"},
		{"d^", "  ab|cd", "d^", "  |cd"},
		{"dd", "a\n|b\nc", "dd", "a\n|c"},
		{"dd last line", "a\n|b", "dd", "|a"},
		{"dd only line", "|abc", "dd", "|"},
		{"2dd", "|a\nb\nc", "2dd", "|c"},
		{"3dd clamps", "a\n|b\nc", "3dd", "|a"},
		{"2dd on the last line does nothing", "a\n|b", "2dd", "a\n|b"},
		{"dd to first non-blank", "a\n|b\n  c", "dd", "a\n  |c"},
		{"dj", "|a\nb\nc", "dj", "|c"},
		{"dk", "a\nb\n|c", "dk", "|a"},
		{"dj on the last line does nothing", "a\n|b", "dj", "a\n|b"},
		{"dG", "a\n|b\nc", "dG", "|a"},
		{"dgg", "a\n|b\nc", "dgg", "|c"},
		{"df", "|a,b,c", "df,", "|b,c"},
		{"dt", "|ab,c", "dt,", "|,c"},
		{"dF", "a,b|c", "dF,", "a|c"},
		{"failed motion cancels", "|abc", "dfz", "|abc"},
		{"esc cancels an operator", "|abc", "d<esc>x", "|bc"},
		{"other operator cancels", "|abc", "dyx", "|bc"},
		{"x", "a|bc", "x", "a|c"},
		{"x on the last char", "ab|c", "x", "a|b"},
		{"x on an empty buffer", "|", "x", "|"},
		{"count x", "|abcd", "3x", "|d"},
		{"count x stops at line end", "ab|cd\nef", "5x", "a|b\nef"},
		{"del", "a|bc", "<del>", "a|c"},
		{"X", "a|bc", "X", "|bc"},
		{"X at line start", "|abc", "X", "|abc"},
		{"cw", "|foo bar", "cwxyz<esc>", "xy|z bar"},
		{"cw on a word's last char", "fo|o bar", "cwX<esc>", "fo|X bar"},
		{"cw on blanks", "foo|  bar", "cwX<esc>", "foo|Xbar"},
		{"c$", "ab|cd", "c$X<esc>", "ab|X"},
		{"C", "ab|cd", "CX<esc>", "ab|X"},
		{"C on an empty line", "a\n|\nb", "CX<esc>", "a\n|X\nb"},
		{"cc", "a\n  |b\nc", "ccX<esc>", "a\n|X\nc"},
		{"S", "a\n|b\nc", "SX<esc>", "a\n|X\nc"},
		{"s", "a|bc", "sX<esc>", "a|Xc"},
		{"count s", "|abcd", "3sX<esc>", "|Xd"},
		{"s on an empty line", "|", "sX<esc>", "|X"},
		{"cj", "|a\nb\nc", "cjX<esc>", "|X\nc"},
		{"yw P", "|foo bar", "ywP", "foo| foo bar"},
		{"yw $p", "|foo bar", "yw$p", "foo barfoo| "},
		{"yb moves to the start", "foo |bar", "ybP", "foo| foo bar"},
		{"yy p", "|a\nb", "yyp", "a\n|a\nb"},
		{"yy P", "|a\nb", "yyP", "|a\na\nb"},
		{"Y is yy", "|a\nb", "YjP", "a\n|a\nb"},
		{"yk moves up", "a\n|b", "ykGp", "a\nb\n|a\nb"},
		{"count p", "|x", "yl3p", "xxx|x"},
		{"count linewise p", "|a", "yy2p", "a\n|a\na"},
		{"p on an empty line", "a\n|\nb", "kylj p", "a\n|a\nb"},
		{"yiw", "foo b|ar", "yiwP", "foo ba|rbar"},
	})
}

func TestEditCommands(t *testing.T) {
	runCases(t, []vimCase{
		{"r", "|abc", "rx", "|xbc"},
		{"count r", "|abc", "3rx", "xx|x"},
		{"count r past the line end", "|ab", "3rx", "|ab"},
		{"r emoji", "|abc", "r😀", "|😀bc"},
		{"r esc", "|abc", "r<esc>", "|abc"},
		{"~", "|aBc", "~", "A|Bc"},
		{"count ~", "|aBc", "3~", "Ab|C"},
		{"~ on an empty line", "|", "~", "|"},
		{"J", "|foo\nbar", "J", "foo| bar"},
		{"J after a trailing blank", "|foo \nbar", "J", "foo |bar"},
		{"J strips the indent", "|a\n   b", "J", "a| b"},
		{"J with an empty line", "|a\n\nb", "J", "|a\nb"},
		{"J before )", "|f(\n)", "J", "f(|)"},
		{"count J", "|a\nb\nc", "3J", "a b| c"},
		{"J on the last line", "a\n|b", "J", "a\n|b"},
		{"i", "a|bc", "iX<esc>", "a|Xbc"},
		{"a", "a|bc", "aX<esc>", "ab|Xc"},
		{"a on an empty line", "|", "aX<esc>", "|X"},
		{"I", "  a|bc", "IX<esc>", "  |Xabc"},
		{"A", "a|bc", "AX<esc>", "abc|X"},
		{"o", "a|bc\nd", "oX<esc>", "abc\n|X\nd"},
		{"O", "a|bc\nd", "OX<esc>", "|X\nabc\nd"},
		{"count i", "|", "3ix<esc>", "xx|x"},
		{"count o", "|a", "2oy<esc>", "a\ny\n|y"},
		{"esc at line start stays", "|abc", "i<esc>", "|abc"},
		{"typing in insert", "|ac", "lib<esc>", "a|bc"},
	})
}

func TestTextObjects(t *testing.T) {
	runCases(t, []vimCase{
		{"diw", "foo b|ar baz", "diw", "foo | baz"},
		{"daw", "foo b|ar baz", "daw", "foo |baz"},
		{"daw takes leading blanks at the end", "foo |bar", "daw", "fo|o"},
		{"diw on blanks", "foo |  bar", "diw", "foo|bar"},
		{"daw on blanks", "foo |  bar baz", "daw", "foo| baz"},
		{"ciw", "foo b|ar", "ciwX<esc>", "foo |X"},
		{"diW", "a foo.b|ar b", "diW", "a | b"},
		{"diw on emoji", "a 😀|😀 b", "diw", "a | b"},
		{`di"`, `say "he|llo" now`, `di"`, `say "|" now`},
		{`da"`, `say "he|llo" now`, `da"`, `say |now`},
		{`ci" before the string`, `|x "ab"`, `ci"Z<esc>`, `x "|Z"`},
		{`di" on an opening quote`, `"a" |"b"`, `di"`, `"a" "|"`},
		{`di" between strings`, `"a" |x "b"`, `di"`, `"a"|"b"`},
		{`di" escaped quote`, `"a\"|b"`, `di"`, `"|"`},
		{"di'", "x 'a|b'", "di'", "x '|'"},
		{"di`", "x `a|b`", "di`", "x `|`"},
		{"di(", "f(a, |b)", "di(", "f(|)"},
		{"da(", "f(a, |b)", "da(", "|f"},
		{"dib", "f(a|b)", "dib", "f(|)"},
		{"di)", "f(a|b)", "di)", "f(|)"},
		{"ci(", "f(a|)", "ci(X<esc>", "f(|X)"},
		{"nested", "(a (b|) c)", "di(", "(a (|) c)"},
		{"on the open bracket", "|(ab)", "di(", "(|)"},
		{"on the close bracket", "(ab|)", "di(", "(|)"},
		{"di[", "x[1, |2]", "di[", "x[|]"},
		{"da[", "x[1, |2]", "da[", "|x"},
		{"di{", "{a|b}", "di{", "{|}"},
		{"diB", "{a|b}", "diB", "{|}"},
		{"di<", "<a|b>", "di<", "<|>"},
		{"multi-line block", "(a\nb|c)", "di(", "(|)"},
		{"di{ on braces on their own lines", "{\n  fo|o\n}", "di{", "{\n|}"},
		{"ci{ on braces on their own lines", "{\n  fo|o\n}", "ci{X<esc>", "{\n|X\n}"},
		{"di{ with only the open brace ending its line", "{\n  f|oo }", "di{", "{\n|}"},
		{"ci{ with only the open brace ending its line", "{\n  f|oo }", "ci{X<esc>", "{\n|X}"},
		{"di( keeps the line break after (", "f(\n  a|, b)", "di(", "f(\n|)"},
		{"no object", "ab|c", "di(", "ab|c"},
		{"no quote", "ab|c", `di"`, "ab|c"},
		{"empty line", "a\n|\nb", "diw", "a\n|\nb"},
	})
}

func TestVisual(t *testing.T) {
	runCases(t, []vimCase{
		{"v d", "|foo bar", "vlld", "| bar"},
		{"v w d", "|foo bar", "vwd", "|ar"},
		{"v x", "|abc", "vlx", "|c"},
		{"V d", "a\n|b\nc", "Vjd", "|a"},
		{"V x", "a\n|b\nc", "Vx", "a\n|c"},
		{"v y moves to the start", "foo b|ar", "vbyP", "foo b|abar"},
		{"v U", "|abc", "v$U", "|ABC"},
		{"v u", "|ABC", "vlu", "|abC"},
		{"v ~", "|aBc", "v$~", "|AbC"},
		{"v o", "|abcd", "vllohd", "|d"},
		{"v iw", "fo|o bar", "viwd", "| bar"},
		{"v p", "|foo bar", "yiwwviwp", "foo fo|o"},
		{"V J", "|a\nb\nc", "VjJ", "a| b\nc"},
		{"v r", "|abc", "vlrx", "|xxc"},
		{"v c", "|abc", "vlcX<esc>", "|Xc"},
		{"V c", "a\n|b\nc", "VcX<esc>", "a\n|X\nc"},
		{"v D is linewise", "a\n|bc\nd", "vD", "a\n|d"},
		{"v esc", "|abc", "vl<esc>x", "a|c"},
		{"v v exits", "|abc", "vlvx", "a|c"},
		{"empty line's break is selected", "a\n|\nb", "vd", "a\n|b"},
		{"v $ d takes the line break", "a|bc\nd", "v$d", "a|d"},
		{"v $ j d", "a|bc\ndef\ng", "v$jd", "a|g"},
		{"v $ h forgets the line end", "a|bc\nd", "v$hd", "a|c\nd"},
		{"v $ on the last line", "a|bc", "v$d", "|a"},
		{"v j d", "a|bc\ndef", "vjd", "a|f"},
	})
}

func TestVisualModesAndSelection(t *testing.T) {
	e := normal(nil)
	b, _ := e.Handle(buf("a|bc"), Key{Text: "v"})
	if e.Mode() != Visual {
		t.Fatalf("v: mode %v, want VISUAL", e.Mode())
	}
	b, _ = e.Handle(b, Key{Text: "l"})
	if from, to, ok := e.Selection(b); !ok || from != 1 || to != 3 {
		t.Errorf("Selection = %d,%d,%v; want 1,3,true", from, to, ok)
	}
	b, _ = e.Handle(b, Key{Text: "V"})
	if e.Mode() != VisualLine {
		t.Fatalf("V from VISUAL: mode %v", e.Mode())
	}
	line := normal(nil)
	lb, _ := line.Handle(buf("ab\n|cd\nx"), Key{Text: "V"})
	if from, to, ok := line.Selection(lb); !ok || from != 3 || to != 6 {
		t.Errorf("linewise Selection = %d,%d,%v; want 3,6,true (with the line break)", from, to, ok)
	}
	last := normal(nil)
	lb, _ = last.Handle(buf("ab\n|cd"), Key{Text: "V"})
	if from, to, ok := last.Selection(lb); !ok || from != 3 || to != 6 {
		t.Errorf("linewise Selection of the last line = %d,%d,%v; want 3,6,true (one past the end)", from, to, ok)
	}
	if _, ok := e.Handle(b, Key{Name: KeyEsc}); !ok || e.Mode() != Normal {
		t.Errorf("Esc in V-LINE: consumed=%v mode=%v", ok, e.Mode())
	}
	if _, _, ok := e.Selection(b); ok {
		t.Error("Selection outside VISUAL must report false")
	}
}

// Selection takes in the line breaks a selection covers, so the host can
// paint the empty lines in it (an empty last line too).
func TestSelectionLineBreaks(t *testing.T) {
	for _, tc := range []struct {
		name, start, keys string
		from, to          int
	}{
		{"v$ runs through the line break", "a|bc\nd", "v$", 1, 4},
		{"V-LINE up to an empty line", "abc\n|\nxyz", "Vk", 0, 5},
		{"V-LINE on an empty last line", "ab\n|", "V", 3, 4},
		{"VISUAL from an empty last line", "ab\n|", "vk", 0, 4},
		{"VISUAL ending before an empty line", "a|bc\n\nd", "vl", 1, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := normal(nil)
			b := host(e, buf(tc.start), tc.keys)
			if from, to, ok := e.Selection(b); !ok || from != tc.from || to != tc.to {
				t.Errorf("Selection = %d,%d,%v; want %d,%d,true", from, to, ok, tc.from, tc.to)
			}
		})
	}

	reg := &Register{}
	host(New(reg), buf("a|bc\nd"), "v$y")
	if reg.Text != "bc\n" || reg.Linewise {
		t.Errorf("v$y register = %+v, want charwise \"bc\\n\"", *reg)
	}
}

func TestUndoRedo(t *testing.T) {
	runCases(t, []vimCase{
		{"u after dw", "|foo bar", "dwu", "|foo bar"},
		{"redo", "|foo bar", "dwu<c-r>", "|bar"},
		{"u after an insert session", "|abc", "iXY<esc>u", "|abc"},
		{"u after cw is one step", "|foo bar", "cwX<esc>u", "|foo bar"},
		{"two steps", "|abc", "xxuu", "|abc"},
		{"redo one of two", "|abc", "xxuu<c-r>", "|bc"},
		{"count u", "|abcd", "xxx2u", "|bcd"},
		{"nothing to undo", "|abc", "u", "|abc"},
		{"nothing to redo", "|abc", "<c-r>", "|abc"},
		{"a new change drops redo", "|abcd", "xux<c-r>", "|bcd"},
		{"cursor returns to the change", "abc |def", "dw0u", "abc |def"},
		{"motions are not undo steps", "|abc", "xllu", "|abc"},
		{"visual change", "|abc", "vldu", "|abc"},
		{"o", "|a", "ob<esc>u", "|a"},
		{"cursor on the deleted one of equal chars", "a|aa", "xu", "a|aa"},
		{"db undo goes to the start", "foo |bar", "dbu", "|foo bar"},
	})
}

func TestUndoLimit(t *testing.T) {
	e := normal(nil)
	b := host(e, buf("|"+strings.Repeat("x", 150)), strings.Repeat("x", 150))
	b = host(e, b, strings.Repeat("u", 150))
	if got := len(b.Text); got != undoLimit {
		t.Errorf("after 150 x and 150 u: %d chars back, want %d (the history limit)", got, undoLimit)
	}
}

func TestRepeat(t *testing.T) {
	runCases(t, []vimCase{
		{"dw .", "|a b c d", "dw.", "|c d"},
		{"count replaces", "|a b c d", "dw2.", "|d"},
		{"x . .", "|abc", "x..", "|"},
		{"2x 3.", "|abcdef", "2x3.", "|f"},
		{"insert", "|", "ifoo<esc>.", "fofo|oo"},
		{"ciw then w .", "|foo bar", "ciwX<esc>w.", "X |X"},
		{"A", "|a\nb", "A;<esc>j.", "a;\nb|;"},
		{"o", "|a", "ob<esc>.", "a\nb\n|b"},
		{"r", "|abcd", "rxl.", "x|xcd"},
		{"p", "|ab", "ylp.", "aa|ab"},
		{"J", "|a\nb\nc", "J.", "a b| c"},
		{"dd", "|a\nb\nc", "dd.", "|c"},
		{"visual change is not repeatable", "|abcdef", "xvlx.", "|def"},
		{"visual yank keeps the last change", "|abcdef", "xvly.", "|cdef"},
		{"backspacing past the start is not repeatable", "ab|c", "i<bs>X<esc>.", "a|Xc"},
		{"nothing to repeat", "|abc", ".", "|abc"},
		{"undo of a repeat", "|a b c", "dw.u", "|b c"},
	})
}

func TestInsertSessionsFromTheHost(t *testing.T) {
	// The host's input gained focus: typing then Esc is one undo step.
	e := normal(nil)
	e.EnterInsert(buf("|"))
	if e.Mode() != Insert {
		t.Fatalf("EnterInsert: mode %v", e.Mode())
	}
	if _, ok := e.Handle(buf("|"), Key{Text: "x"}); ok {
		t.Fatal("INSERT must leave typing to the host")
	}
	b := host(e, buf("|"), "hi<esc>")
	if show(b) != "h|i" || e.Mode() != Normal {
		t.Fatalf("typed then Esc: %q mode %v", show(b), e.Mode())
	}
	if b = host(e, b, "u"); show(b) != "|" {
		t.Errorf("u after a focus session = %q, want empty", show(b))
	}

	// A second EnterInsert while typing keeps the session.
	e = normal(nil)
	e.EnterInsert(buf("|"))
	e.EnterInsert(buf("ab|"))
	b = host(e, buf("ab|"), "<esc>u")
	if show(b) != "|" {
		t.Errorf("EnterInsert while in INSERT restarted the session: u = %q", show(b))
	}

	// Blur closes the session: still one undo step, no cursor step left.
	e = normal(nil)
	e.EnterInsert(buf("|"))
	e.Blur(buf("hi|"))
	if e.Mode() != Normal {
		t.Fatalf("Blur: mode %v", e.Mode())
	}
	if b = host(e, buf("h|i"), "u"); show(b) != "|" {
		t.Errorf("u after Blur = %q, want empty", show(b))
	}

	// Blur drops a selection and a half-typed command.
	e = normal(nil)
	host(e, buf("|abc"), "vl")
	e.Blur(buf("|abc"))
	if e.Mode() != Normal || e.Pending() {
		t.Errorf("Blur from VISUAL: mode %v pending %v", e.Mode(), e.Pending())
	}
}

func TestResetAndCheckpoint(t *testing.T) {
	e := normal(nil)
	b := host(e, buf("|foo bar"), "dw")
	e.Reset(b)
	if b = host(e, b, "u"); show(b) != "|bar" {
		t.Errorf("u after Reset = %q: history must be gone", show(b))
	}

	// Reset in INSERT restarts the session on the new text.
	e = normal(nil)
	e.EnterInsert(buf("|"))
	e.Reset(buf("ab|"))
	if b = host(e, buf("ab|"), "<esc>u"); show(b) != "a|b" {
		t.Errorf("Reset in INSERT: Esc u = %q, want the reset text kept", show(b))
	}

	// Reset ends VISUAL.
	e = normal(nil)
	host(e, buf("|abc"), "vl")
	e.Reset(buf("|abc"))
	if e.Mode() != Normal {
		t.Errorf("Reset from VISUAL: mode %v", e.Mode())
	}

	// Checkpoint makes an outside edit undoable.
	e = normal(nil)
	e.Checkpoint(buf("ab|c"))
	if b = host(e, buf("abXc|"), "u"); show(b) != "ab|c" {
		t.Errorf("u after Checkpoint = %q", show(b))
	}
}

func TestEscAndPending(t *testing.T) {
	e := normal(nil)
	if _, ok := e.Handle(buf("|abc"), Key{Name: KeyEsc}); ok {
		t.Error("Esc in idle NORMAL must not be consumed (the host leaves)")
	}
	for _, k := range []string{"2", "d", "f", "g", "r", "di"} {
		e := normal(nil)
		host(e, buf("|abc"), k)
		if !e.Pending() {
			t.Errorf("%q: Pending() = false", k)
		}
		if _, ok := e.Handle(buf("|abc"), Key{Name: KeyEsc}); !ok || e.Pending() {
			t.Errorf("%q then Esc: consumed=%v pending=%v", k, ok, e.Pending())
		}
	}
	e = normal(nil)
	for _, k := range []Key{{Text: "Q"}, {}, {Text: ":"}} {
		b, ok := e.Handle(buf("a|bc"), k)
		if !ok || show(b) != "a|bc" {
			t.Errorf("unbound %+v: consumed=%v buffer %q", k, ok, show(b))
		}
	}
}

func TestNormalize(t *testing.T) {
	e := normal(nil)
	if got := show(e.Normalize(buf("abc|"))); got != "ab|c" {
		t.Errorf("NORMAL Normalize at the end = %q", got)
	}
	if got := show(e.Normalize(buf("ab|\ncd"))); got != "a|b\ncd" {
		t.Errorf("NORMAL Normalize on a line break = %q", got)
	}
	flag := "🇯🇵" // two regional indicators: one cluster, two runes
	if got := e.Normalize(Buffer{Text: "a" + flag, Cursor: 1 + 4}); got.Cursor != 1 {
		t.Errorf("mid-cluster cursor snapped to %d, want 1", got.Cursor)
	}
	e.EnterInsert(buf("|"))
	if got := show(e.Normalize(buf("abc|"))); got != "abc|" {
		t.Errorf("INSERT Normalize moved the cursor: %q", got)
	}
}

func TestGraphemes(t *testing.T) {
	family := "👨\u200d👩\u200d👧"
	runCases(t, []vimCase{
		{"x deletes a whole ZWJ sequence", "|" + family + "x", "x", "|x"},
		{"l over a flag", "a|🇯🇵b", "l", "a🇯🇵|b"},
		{"h over a flag", "a🇯🇵|b", "h", "a|🇯🇵b"},
		{"combining mark stays with its base", "e\u0301|x", "h", "|e\u0301x"},
		{"$ lands on the last cluster", "|ab" + family, "$", "ab|" + family},
		{"r over an emoji", "|😀b", "rx", "|xb"},
		{"p after an emoji", "|😀b", "ylp", "😀|😀b"},
		{"f lands on cluster", "|a" + family + "b", "fb", "a" + family + "|b"},
	})
	// A cursor the host leaves inside a cluster snaps to its start.
	e := normal(nil)
	b, _ := e.Handle(Buffer{Text: "🇯🇵b", Cursor: 4}, Key{Text: "x"})
	if b.Text != "b" {
		t.Errorf("x from inside a flag = %q, want the flag deleted", b.Text)
	}
}

func TestSharedRegister(t *testing.T) {
	reg := &Register{}
	a, b := New(reg), New(reg)
	host(a, buf("|foo bar"), "yw")
	if got := show(host(b, buf("|x"), "p")); got != "xfoo| " {
		t.Errorf("put from the other editor = %q", got)
	}
	if reg.Text != "foo " || reg.Linewise {
		t.Errorf("register = %+v", *reg)
	}
	host(a, buf("|a\nb"), "dd")
	if !reg.Linewise || reg.Text != "a" {
		t.Errorf("dd register = %+v, want linewise a", *reg)
	}
	host(a, buf("|"), "")
	if got := show(host(New(&Register{Text: "x", Linewise: true}), buf("|"), "p")); got != "\n|x" {
		t.Errorf("linewise put into an empty buffer = %q", got)
	}
	if got := show(host(New(&Register{Text: "a\nb"}), buf("|xy"), "p")); got != "x|a\nby" {
		t.Errorf("multi-line charwise put = %q", got)
	}
}

func TestCountCap(t *testing.T) {
	e := normal(nil)
	b := host(e, buf("|abc"), "99999999x")
	if b.Text != "" {
		t.Errorf("huge count x = %q", b.Text)
	}
	b = host(e, buf("|a"), "yl99999999p")
	if n := len(b.Text); n > maxCount+1 {
		t.Errorf("huge count p made %d bytes, want at most %d", n, maxCount+1)
	}
}

func TestEmptyBufferNeverPanics(t *testing.T) {
	for _, k := range []string{
		"h", "l", "j", "k", "w", "b", "e", "ge", "W", "B", "E", "0", "^", "$", "gg", "G",
		"x", "X", "dd", "D", "C<esc>", "s<esc>", "S<esc>", "p", "P", "J", "~", "rx", "u",
		"<c-r>", ".", "diw", `di"`, "di(", "vd", "Vd", "vy", "vp", "vJ", "o<esc>", "A<esc>",
		"f,", ";", ",", "yy", "cw<esc>", "dw", "db",
	} {
		e := normal(nil)
		if got := host(e, buf("|"), k); len(got.Text) > 1 {
			t.Errorf("%q on an empty buffer gave %q", k, got.Text)
		}
	}
}

// TestRandomKeysKeepTheCursorValid drives random keys over text with
// emoji, CJK, combining marks and blank lines, checking after every key
// that the cursor is inside the buffer, on a cluster boundary, and (in
// NORMAL) not on the line break of a non-empty line.
func TestRandomKeysKeepTheCursorValid(t *testing.T) {
	texts := []string{
		"", "a", "foo bar\nbaz", "日本語 text\n\n  (x [y] {z})\n😀👨\u200d👩\u200d👧 e\u0301 \"q\" 'r'",
		"🇯🇵🇺🇸\n\n\n", "a\tb", "  \n  ",
	}
	alphabet := []string{
		"h", "j", "k", "l", "w", "b", "e", "W", "B", "E", "g", "0", "^", "$", "G", "f", "t", "F", "T",
		";", ",", "d", "c", "y", "p", "P", "x", "X", "s", "S", "D", "C", "Y", "r", "~", "J", "u",
		"<c-r>", ".", "i", "a", "I", "A", "o", "O", "v", "V", "iw", "a(", `i"`, "2", "3", "<esc>",
		"<bs>", "<del>", "<home>", "<end>", "<up>", "<down>", "😀", "日", " ", "\n", "q",
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for _, text := range texts {
		for range 40 {
			e := normal(nil)
			b := Buffer{Text: text, Cursor: rng.IntN(len(text) + 1)}
			b = e.Normalize(b)
			for range 60 {
				k := alphabet[rng.IntN(len(alphabet))]
				b = host(e, b, k)
				checkCursor(t, e, b, text, k)
			}
		}
	}
}

func checkCursor(t *testing.T, e *Editor, b Buffer, start, k string) {
	t.Helper()
	if b.Cursor < 0 || b.Cursor > len(b.Text) {
		t.Fatalf("from %q after %q: cursor %d outside %q", start, k, b.Cursor, b.Text)
	}
	if !utf8.ValidString(b.Text) {
		t.Fatalf("from %q after %q: invalid UTF-8 %q", start, k, b.Text)
	}
	tx := newText(b.Text)
	p := tx.cellAt(b.Cursor)
	if tx.byteOf(p) != b.Cursor {
		t.Fatalf("from %q after %q: cursor %d inside a cluster of %q", start, k, b.Cursor, b.Text)
	}
	if e.Mode() == Normal && p > tx.lineStart(p) && (p >= tx.n() || tx.isNL(p)) {
		t.Fatalf("from %q after %q: NORMAL cursor on a line end of %q at %d", start, k, b.Text, b.Cursor)
	}
}
