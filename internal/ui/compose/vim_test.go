package compose

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/vimedit"
	"github.com/gammons/slk/internal/ui/mentionpicker"
)

// vimCompose is a focused composer at width with vim mode on, in INSERT.
func vimCompose(t *testing.T, width int) Model {
	t.Helper()
	m := New("general")
	m.SetWidth(width)
	m.SetVimMode(&vimedit.Register{})
	_ = m.Focus()
	if mode, ok := m.VimMode(); !ok || mode != vimedit.Insert {
		t.Fatalf("focused vim composer: mode %v ok %v, want INSERT", mode, ok)
	}
	return m
}

// vimPress sends keys through Update; <esc> goes through VimEscape, as
// the App sends it, and <up> <down> <left> <right> <bs> <c-r> are those
// keys.
func vimPress(m Model, keys string) Model {
	special := map[string]tea.KeyPressMsg{
		"<up>": {Code: tea.KeyUp}, "<down>": {Code: tea.KeyDown},
		"<left>": {Code: tea.KeyLeft}, "<right>": {Code: tea.KeyRight},
		"<bs>": {Code: tea.KeyBackspace}, "<c-r>": {Code: 'r', Mod: tea.ModCtrl},
		"<home>": {Code: tea.KeyHome}, "<end>": {Code: tea.KeyEnd}, "<del>": {Code: tea.KeyDelete},
	}
	for keys != "" {
		if strings.HasPrefix(keys, "<esc>") {
			m.VimEscape()
			keys = keys[len("<esc>"):]
			continue
		}
		matched := false
		for name, k := range special {
			if strings.HasPrefix(keys, name) {
				m, _ = m.Update(k)
				keys = keys[len(name):]
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		r := []rune(keys)[0]
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		keys = keys[len(string(r)):]
	}
	return m
}

// at is the composer's value with "|" before its cursor.
func at(m Model) string {
	v, c := m.Value(), m.cursorPosition()
	return v[:c] + "|" + v[c:]
}

func TestVimTypingThenNormalCommands(t *testing.T) {
	m := vimCompose(t, 60)
	m = vimPress(m, "hello world<esc>")
	if mode, _ := m.VimMode(); mode != vimedit.Normal || !m.VimNormal() {
		t.Fatalf("after Esc: mode %v", mode)
	}
	if got := at(m); got != "hello worl|d" {
		t.Fatalf("Esc steps left: %q", got)
	}
	m = vimPress(m, "bdw")
	if got := at(m); got != "hello| " {
		t.Errorf("b dw = %q", got)
	}
	m = vimPress(m, "u")
	if got := at(m); got != "hello |world" {
		t.Errorf("u = %q", got)
	}
	m = vimPress(m, "ciwthere<esc>")
	if got := at(m); got != "hello ther|e" {
		t.Errorf("ciw = %q", got)
	}
	m = vimPress(m, "0x<c-r>")
	if got := at(m); got != "|ello there" {
		t.Errorf("x then ctrl+r (nothing to redo) = %q", got)
	}
}

func TestVimCursorWriteBackMultiLine(t *testing.T) {
	m := vimCompose(t, 60)
	m = vimPress(m, "<esc>")
	m.SetValue("ab\ncd\nef")
	if got := at(m); got != "ab\ncd\ne|f" {
		t.Fatalf("SetValue in NORMAL leaves the cursor on the last char: %q", got)
	}
	for _, tc := range []struct{ keys, want string }{
		{"gg", "|ab\ncd\nef"},
		{"jl", "ab\nc|d\nef"},
		{"k", "a|b\ncd\nef"},
		{"G0", "ab\ncd\n|ef"},
		{"<up><up>", "|ab\ncd\nef"},
		{"<down>$", "ab\nc|d\nef"},
	} {
		m = vimPress(m, tc.keys)
		if got := at(m); got != tc.want {
			t.Errorf("%q: %q, want %q", tc.keys, got, tc.want)
		}
	}
}

func TestVimCursorWriteBackWrappedAndWide(t *testing.T) {
	m := vimCompose(t, 20) // textarea wraps at 15
	long := strings.Repeat("word ", 12) + "end"
	m.SetValue(long + "\n日本語😀x")
	m = vimPress(m, "<esc>gg$")
	if got, want := m.cursorPosition(), len(long)-1; got != want {
		t.Errorf("$ on a wrapped line: cursor %d, want %d", got, want)
	}
	m = vimPress(m, "0")
	if got := m.cursorPosition(); got != 0 {
		t.Errorf("0: cursor %d", got)
	}
	m = vimPress(m, "j")
	if got := at(m); !strings.HasSuffix(got, "\n|日本語😀x") {
		t.Errorf("j to the second logical line: %q", got)
	}
	m = vimPress(m, "lll")
	if got := at(m); !strings.HasSuffix(got, "\n日本語|😀x") {
		t.Errorf("l over CJK: %q", got)
	}
	m = vimPress(m, "x")
	if got := m.Value(); !strings.HasSuffix(got, "\n日本語x") {
		t.Errorf("x deletes the whole emoji: %q", got)
	}
}

func TestVimViewportFollowsTheCursor(t *testing.T) {
	m := vimCompose(t, 40)
	var lines []string
	for i := range 40 {
		lines = append(lines, fmt.Sprintf("line%02d", i))
	}
	m.SetValue(strings.Join(lines, "\n"))
	m = vimPress(m, "<esc>gg")
	if v := ansi.Strip(m.View(40, true)); !strings.Contains(v, "line00") || strings.Contains(v, "line39") {
		t.Errorf("gg: view does not show the top:\n%s", v)
	}
	m = vimPress(m, "G")
	if v := ansi.Strip(m.View(40, true)); !strings.Contains(v, "line39") || strings.Contains(v, "line00") {
		t.Errorf("G: view does not show the bottom:\n%s", v)
	}
}

// V-LINE paints an empty line as one cell, also when it is the last
// line of the selection.
func TestVimVisualLinePaintsAnEmptyLastLine(t *testing.T) {
	m := vimCompose(t, 40)
	m.SetValue("abc\n\nxyz")
	m = vimPress(m, "<esc>ggjVk")
	painted := strings.Split(m.View(40, true), "\n")
	m = vimPress(m, "<esc>") // same cursor, no selection
	unpainted := strings.Split(m.View(40, true), "\n")
	// Rows: top padding, abc, the empty line, xyz.
	if len(painted) < 4 || len(unpainted) != len(painted) {
		t.Fatalf("unexpected view shape:\n%s", strings.Join(painted, "\n"))
	}
	if painted[2] == unpainted[2] {
		t.Errorf("the empty line in the V-LINE selection is not painted:\n%q", painted[2])
	}
	if painted[3] != unpainted[3] {
		t.Errorf("the line after the selection is painted:\n%q", painted[3])
	}
}

func TestVimKeysNeverReachTheTextarea(t *testing.T) {
	m := vimCompose(t, 40)
	m = vimPress(m, "abc<esc>")
	before := m.Value()
	for _, k := range []tea.KeyPressMsg{
		{Code: 'a', Mod: tea.ModCtrl}, // textarea: line start
		{Code: 'k', Mod: tea.ModCtrl}, // textarea: delete to end
		{Code: 'z', Text: "z"},
		{Code: 'f', Mod: tea.ModAlt},
		{Code: tea.KeyTab},
	} {
		m, _ = m.Update(k)
	}
	if got := at(m); got != "ab|c" || m.Value() != before {
		t.Errorf("unbound keys changed the composer: %q", got)
	}
	m, _ = m.Update(tea.KeyReleaseMsg{Code: 'x', Text: "x"})
	if m.Value() != before {
		t.Error("a key release ran a command")
	}
}

func TestVimPasteInNormalIsOneUndoStep(t *testing.T) {
	m := vimCompose(t, 40)
	m = vimPress(m, "ab<esc>")
	m, _ = m.Update(tea.PasteMsg{Content: "XY"})
	if got := m.Value(); got != "aXYb" {
		t.Fatalf("paste in NORMAL = %q", got)
	}
	if mode, _ := m.VimMode(); mode != vimedit.Normal {
		t.Errorf("paste changed the sub-mode to %v", mode)
	}
	m = vimPress(m, "u")
	if got := m.Value(); got != "ab" {
		t.Errorf("u after paste = %q", got)
	}
}

func TestVimPickersStayInsertFeatures(t *testing.T) {
	m := vimCompose(t, 40)
	m.SetUsers([]mentionpicker.User{{ID: "U1", DisplayName: "Alice", Username: "alice"}})
	m = vimPress(m, "hi @")
	if !m.IsMentionActive() {
		t.Fatal("@ in INSERT should open the mention picker")
	}
	// The picker's Esc is the composer's own (the App asks VimEscape only
	// once no picker is open): it closes the picker, INSERT stays.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.IsMentionActive() {
		t.Fatal("Esc should close the picker")
	}
	if mode, _ := m.VimMode(); mode != vimedit.Insert {
		t.Errorf("closing the picker left INSERT: %v", mode)
	}

	// In NORMAL a trigger under the cursor opens nothing.
	m.SetValue("x :smile")
	m = vimPress(m, "<esc>h@#:")
	if m.IsEmojiActive() || m.IsMentionActive() || m.IsChannelActive() {
		t.Error("a picker opened in NORMAL")
	}
}

func TestVimEscapeLadder(t *testing.T) {
	m := vimCompose(t, 40)
	if !m.VimEscape() {
		t.Fatal("Esc in INSERT must be taken (to NORMAL)")
	}
	m = vimPress(m, "d")
	if !m.VimBusy() {
		t.Error("d should leave a command pending")
	}
	if !m.VimEscape() || m.VimBusy() {
		t.Error("Esc should drop the pending d")
	}
	m = vimPress(m, "v")
	if !m.VimBusy() {
		t.Error("VISUAL counts as busy")
	}
	if !m.VimEscape() {
		t.Error("Esc should end VISUAL")
	}
	if m.VimEscape() {
		t.Error("Esc in idle NORMAL must not be taken: it leaves the composer")
	}
}

func TestVimOffChangesNothing(t *testing.T) {
	m := New("general")
	m.SetWidth(40)
	_ = m.Focus()
	if _, ok := m.VimMode(); ok {
		t.Fatal("vim mode must default off")
	}
	if m.VimEscape() || m.VimNormal() || m.VimBusy() {
		t.Error("vim queries must be false with vim mode off")
	}
	m = typeChars(t, m, "hjkl")
	if m.Value() != "hjkl" {
		t.Errorf("typing with vim off = %q", m.Value())
	}
}

func TestVimFocusAndBlur(t *testing.T) {
	m := vimCompose(t, 40)
	m = vimPress(m, "abc<esc>")
	m.Blur()
	_ = m.Focus()
	if mode, _ := m.VimMode(); mode != vimedit.Insert {
		t.Errorf("re-focus lands in %v, want INSERT", mode)
	}
	// A focus while already composing keeps the session: undo still
	// covers the earlier typing.
	m = vimPress(m, "d")
	_ = m.Focus()
	m = vimPress(m, "<esc>u")
	if got := m.Value(); got != "abc" {
		t.Errorf("after refocus, u = %q (want the typing since the last focus undone)", got)
	}
}

func TestVimDraftSwitchForgetsHistory(t *testing.T) {
	m := vimCompose(t, 40)
	m.SetDraftContext("T1", "C1", "")
	m = vimPress(m, "one<esc>")
	m.SetDraftContext("T1", "C2", "")
	m = vimPress(m, "u")
	if got := m.Value(); got != "" {
		t.Errorf("u in another conversation brought back %q", got)
	}
	m.SetDraftContext("T1", "C1", "")
	m = vimPress(m, "u")
	if got := m.Value(); got != "one" {
		t.Errorf("u after switching back = %q, history must not cross drafts", got)
	}
}

func TestVimRegisterIsShared(t *testing.T) {
	reg := &vimedit.Register{}
	a, b := New("a"), New("b")
	a.SetWidth(40)
	b.SetWidth(40)
	a.SetVimMode(reg)
	b.SetVimMode(reg)
	_ = a.Focus()
	a = vimPress(a, "shared<esc>0yiw")
	_ = b.Focus()
	b = vimPress(b, "<esc>p")
	if got := b.Value(); got != "shared" {
		t.Errorf("put in the other composer = %q", got)
	}
}

func TestVimVisualSelectionIsPainted(t *testing.T) {
	m := vimCompose(t, 40)
	m = vimPress(m, "hello world<esc>0vll")
	plain := m.View(40, true)
	if !strings.Contains(ansi.Strip(plain), "hello world") {
		t.Fatalf("view lost the text:\n%s", plain)
	}
	m.VimEscape()
	unpainted := m.View(40, true)
	if plain == unpainted {
		t.Error("the visual selection is not drawn")
	}
	if ansi.Strip(plain) != ansi.Strip(unpainted) {
		t.Error("painting the selection changed the text")
	}
}

func TestPaintSelection(t *testing.T) {
	sel := lipgloss.NewStyle().Reverse(true)
	paint := func(s string) string { return sel.Render(s) }

	// One row: cells 1..3 painted, the cursor (on 0) untouched.
	got := paintSelection("hello", "hello", 0, 4, 0, 40, 0, sel)
	if want := "h" + paint("ell") + "o"; got != want {
		t.Errorf("single row:\n got %q\nwant %q", got, want)
	}

	// The cursor in the middle splits the range.
	got = paintSelection("hello", "hello", 0, 5, 2, 40, 0, sel)
	if want := paint("he") + "l" + paint("lo"); got != want {
		t.Errorf("cursor inside:\n got %q\nwant %q", got, want)
	}

	// Soft wrap: at width 5 "aaaa bbbb" wraps into "aaaa " / "bbbb ";
	// a selection of the second word paints only the second row.
	scratch := scratchTextarea(5)
	if starts := rowStarts(&scratch, "aaaa bbbb"); fmt.Sprint(starts) != "[0 5]" {
		t.Fatalf("rowStarts = %v, want [0 5]", starts)
	}
	// Row starts are byte offsets: each é is two bytes.
	if starts := rowStarts(&scratch, "éééé bbbb"); fmt.Sprint(starts) != "[0 9]" {
		t.Fatalf("rowStarts(é…) = %v, want [0 9]", starts)
	}
	got = paintSelection("aaaa \nbbbb ", "aaaa bbbb", 5, 9, 0, 5, 0, sel)
	if want := "aaaa \n" + paint("bbbb") + " "; got != want {
		t.Errorf("wrapped:\n got %q\nwant %q", got, want)
	}

	// Scrolled one row down, the first visible row is the second row.
	got = paintSelection("bbbb ", "aaaa bbbb", 5, 9, 0, 5, 1, sel)
	if want := paint("bbbb") + " "; got != want {
		t.Errorf("scrolled:\n got %q\nwant %q", got, want)
	}

	// Wide characters are measured in cells.
	got = paintSelection("日本語", "日本語", 3, 9, 0, 40, 0, sel)
	if want := "日" + paint("本語"); got != want {
		t.Errorf("CJK:\n got %q\nwant %q", got, want)
	}

	// An empty line inside the selection shows one painted cell.
	got = paintSelection("a\n \nb", "a\n\nb", 0, 3, 3, 40, 0, sel)
	if want := paint("a") + "\n" + paint(" ") + "\nb"; got != want {
		t.Errorf("empty line:\n got %q\nwant %q", got, want)
	}
}

// fitsOneRow is the shortcut paintSelection takes past the textarea's
// wrap. Whenever it says a line fits, the textarea must draw that line
// on a single row.
func TestFitsOneRowAgreesWithTheTextarea(t *testing.T) {
	pieces := []string{
		"a", "word", " ", "  ", "日本", "語", "😀", "👨\u200d👩\u200d👧", "🇯🇵", "e\u0301", " \u0301",
		"\u00a0", "\u3000", "\u2003", "x\u200b", "Ｗ", "-", ".",
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for _, width := range []int{5, 10, 15, 40, 80} {
		scratch := scratchTextarea(width)
		for range 400 {
			var b strings.Builder
			for n := rng.IntN(30); n > 0; n-- {
				b.WriteString(pieces[rng.IntN(len(pieces))])
			}
			line := b.String()
			if !fitsOneRow(line, width) {
				continue
			}
			if starts := rowStarts(&scratch, line); len(starts) != 1 {
				t.Errorf("width %d: fitsOneRow(%q) but the textarea wraps it at %v", width, line, starts)
			}
		}
	}
}

func TestVimKeyTranslation(t *testing.T) {
	for _, tc := range []struct {
		in   tea.KeyPressMsg
		want vimedit.Key
	}{
		{tea.KeyPressMsg{Code: tea.KeyEscape}, vimedit.Key{Name: vimedit.KeyEsc}},
		{tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, vimedit.Key{Name: vimedit.KeyCtrlR}},
		{tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl | tea.ModNumLock}, vimedit.Key{Name: vimedit.KeyCtrlR}},
		{tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}, vimedit.Key{}},
		{tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt, Text: "x"}, vimedit.Key{}},
		{tea.KeyPressMsg{Code: 'A', Mod: tea.ModShift, Text: "A"}, vimedit.Key{Text: "A"}},
		{tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, vimedit.Key{Text: " "}},
		{tea.KeyPressMsg{Code: tea.KeyBackspace}, vimedit.Key{Name: vimedit.KeyBackspace}},
		{tea.KeyPressMsg{Code: tea.KeyUp}, vimedit.Key{Name: vimedit.KeyUp}},
		{tea.KeyPressMsg{Code: tea.KeyEnd}, vimedit.Key{Name: vimedit.KeyEnd}},
		{tea.KeyPressMsg{Code: '😀', Text: "😀"}, vimedit.Key{Text: "😀"}},
	} {
		if got := vimKey(tc.in); got != tc.want {
			t.Errorf("vimKey(%v) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}
