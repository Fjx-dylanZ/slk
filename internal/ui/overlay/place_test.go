package overlay_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/overlay"
)

// placeholderCell is one kitty placeholder cell: the placeholder rune,
// then the row and column diacritics (kitty's table starts U+0305,
// U+030D, U+030E, …), so every cell names its own place in the image.
func placeholderCell(col int) string {
	cols := []rune{0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F}
	return string(image.PlaceholderRune) + "\u0305" + string(cols[col])
}

// Place splices the box into the rows it covers and keeps every byte
// outside it: the rows above and below, the cells left and right of the
// box, and the escapes that colour them. On a kitty placeholder row the
// cells beside the box keep the image ID colour, so they still draw
// their part of the image.
func TestPlaceKeepsEverythingOutsideTheBox(t *testing.T) {
	const idFG = "\x1b[38;2;0;0;42m"
	var kitty strings.Builder
	kitty.WriteString(idFG)
	for c := range 8 {
		kitty.WriteString(placeholderCell(c))
	}
	kitty.WriteString("\x1b[39m")
	bg := []string{"\x1b[1mabcdefgh\x1b[m", kitty.String(), "ABCDEFGH", "\x1b[2m12345678\x1b[m"}

	out := strings.Split(overlay.Place(strings.Join(bg, "\n"), "XYZ\nUVW", 3, 1), "\n")

	if out[0] != bg[0] || out[3] != bg[3] {
		t.Errorf("rows outside the box changed:\n%q\n%q", out[0], out[3])
	}
	if want := "ABC\x1b[mUVW\x1b[mGH"; out[2] != want {
		t.Errorf("plain row = %q, want %q", out[2], want)
	}
	left := idFG + placeholderCell(0) + placeholderCell(1) + placeholderCell(2)
	right := idFG + placeholderCell(6) + placeholderCell(7) + "\x1b[39m"
	if !strings.HasPrefix(out[1], left) || !strings.HasSuffix(out[1], right) || !strings.Contains(out[1], "XYZ") {
		t.Errorf("kitty row = %q\nwant %q … XYZ … %q", out[1], left, right)
	}
	for i, l := range out {
		if w := ansi.StringWidth(l); w != 8 {
			t.Errorf("row %d is %d wide, want 8", i, w)
		}
	}
}

// A wide character the box's edge cuts in half leaves a space in its
// other column; the row keeps its width, also when the cut character
// ends the line, and a line shorter than the box's left edge is padded.
func TestPlaceSplitsWideCharacters(t *testing.T) {
	for _, tc := range []struct {
		bg, box string
		x       int
		want    string
	}{
		{"a界bcd", "X", 0, "X界bcd"}, // 界 is columns 1-2
		{"a界bcd", "X", 1, "aX bcd"},
		{"a界bcd", "X", 2, "a Xbcd"},
		{"a界bcd", "X", 3, "a界Xcd"},
		{"ab界", "XY", 1, "aXY "}, // the right edge cuts the last character
		{"a界", "XY", 0, "XY "},
		{"ab", "X", 4, "ab  X"},
	} {
		got := ansi.Strip(overlay.Place(tc.bg, tc.box, tc.x, 0))
		if got != tc.want {
			t.Errorf("Place(%q, %q, %d): %q, want %q", tc.bg, tc.box, tc.x, got, tc.want)
		}
	}
}

// Place leaves the rows outside the box alone, also when the box
// starts above the first row or runs past the last one, and returns the
// background unchanged for a box below it.
func TestPlaceRows(t *testing.T) {
	const bg = "aaa\nbbb\nccc"
	for _, tc := range []struct {
		y    int
		want string
	}{
		{0, "Xaa\nYbb\nccc"},
		{2, "aaa\nbbb\nXcc"},
		{-1, "Yaa\nbbb\nccc"},
		{3, bg},
	} {
		if got := ansi.Strip(overlay.Place(bg, "X\nY", 0, tc.y)); got != tc.want {
			t.Errorf("y=%d: %q, want %q", tc.y, got, tc.want)
		}
	}
}
