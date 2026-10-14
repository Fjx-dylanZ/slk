package overlay

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/image"
)

// kittyPlaceholderPrefix is the leading byte sequence of any cell whose
// content begins with image.PlaceholderRune (U+10EEEE encoded as UTF-8:
// 0xF4 0x8E 0xBB 0xAE). We match against the Cell.Content string rather
// than the rune to handle the diacritic-suffixed graphemes kitty's
// renderer emits without taking a dependency on cluster segmentation.
var kittyPlaceholderPrefix = string(image.PlaceholderRune)

// DimmedOverlay composites a modal box on top of a dimmed background.
// The background string is rendered to a Canvas, all cell colors are
// darkened by dimPercent (0.0-1.0), then the modal box is placed centered
// on top by copying its cells.
//
// Cells whose content carries the kitty unicode-placeholder rune are
// blanked: their FG is a 24-bit encoding of an image ID (not a visual
// color), so darkening it would mutate the ID and surface a different
// image — see internal/image/kitty.go. Replacing the placeholder rune
// with a space drops the kitty placement for the duration of the
// overlay; the image data stays uploaded and the next non-overlay
// frame re-emits the placeholder cells, re-creating the placement
// without any image-state plumbing. Issue #18.
func DimmedOverlay(width, height int, background string, box string, dimPercent float64) string {
	// Step 1: Render background to canvas and dim all cells.
	//
	// Wide characters (emoji, CJK) need two pieces of care:
	//
	//   1. Continuation cells (Width==0 at column X+1 after a wide
	//      char at column X) are layout placeholders, not real
	//      content; skip them so we don't redundantly process them.
	//
	//   2. lipgloss's canvas has a quirk where calling SetCell(x, y,
	//      cell) on a wide-char position that ALREADY contains that
	//      wide char destroys the glyph (the canvas seems to drop the
	//      preserved continuation column). CellAt returns a live
	//      pointer into the canvas's internal cell storage, so
	//      mutating cell.Style.{Bg,Fg} in-place propagates without
	//      any SetCell call — and avoids the wide-char drop. Use that
	//      path here; do not call SetCell.
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewLayer(background))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			if strings.HasPrefix(cell.Content, kittyPlaceholderPrefix) {
				// Drop the kitty placement at this cell. Clear the FG
				// (was image-ID-as-RGB, not a real color) and let the
				// normal dim path tint any BG so the region blends
				// into the rest of the dimmed background.
				cell.Content = " "
				cell.Width = 1
				cell.Style.Fg = nil
			}
			if cell.Style.Bg != nil {
				cell.Style.Bg = lipgloss.Darken(cell.Style.Bg, dimPercent)
			}
			if cell.Style.Fg != nil {
				cell.Style.Fg = lipgloss.Darken(cell.Style.Fg, dimPercent)
			}
			// Intentionally NO canvas.SetCell here — see comment above.
		}
	}

	// Step 2: Render dimmed canvas, create output canvas
	dimmedStr := canvas.Render()
	outCanvas := lipgloss.NewCanvas(width, height)
	outCanvas.Compose(lipgloss.NewLayer(dimmedStr))

	// Step 3: Render modal to its own canvas, compute centered position
	modalW := lipgloss.Width(box)
	modalH := lipgloss.Height(box)
	startX := (width - modalW) / 2
	startY := (height - modalH) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	modalCanvas := lipgloss.NewCanvas(modalW, modalH)
	modalCanvas.Compose(lipgloss.NewLayer(box))

	// Step 4: Copy modal cells onto output canvas.
	//
	// Wide characters (emoji, CJK) occupy two grid columns. lipgloss's
	// canvas reports the wide cell at column X with Width=2 and a
	// CONTINUATION cell at column X+1 with Content="" Width=0. The
	// continuation cell is a layout placeholder, not real content;
	// calling SetCell on it would overwrite the second column of the
	// wide glyph with empty content, erasing the emoji visually.
	// Skip Width==0 cells so the wide character that already landed
	// at column X stays intact across both of its columns.
	for my := 0; my < modalH; my++ {
		for mx := 0; mx < modalW; mx++ {
			cell := modalCanvas.CellAt(mx, my)
			if cell == nil || cell.Width == 0 {
				continue
			}
			outCanvas.SetCell(startX+mx, startY+my, cell)
		}
	}

	output := outCanvas.Render()

	// Step 5: Patch kitty-placement rows back to their original bytes.
	//
	// The canvas's cell-by-cell store/emit path bundles per-cell style
	// (e.g. Bold) into the SGR it writes before each rune, mutating the
	// pristine "\x1b[38;2;0;0;Bm" foreground that buildPlaceholderLines
	// emits into something like "\x1b[38;2;0;0;B;1m". The RGB triple
	// (which carries the kitty image ID) survives, but in practice
	// terminals will not detect the placeholder as a kitty image when
	// the SGR carries extra attributes — the cell renders blank.
	//
	// Fix: for any modal row that contains a kitty placeholder rune,
	// splice the ORIGINAL box row (exact byte-for-byte SGR + rune +
	// diacritic sequence) into the corresponding output row at column
	// startX, with splice (the one implementation Place uses too), so
	// the splice is width-correct and doesn't break escape sequences in
	// the surrounding (dimmed background) content.
	hasPlacement := false
	boxLines := strings.Split(box, "\n")
	for _, ln := range boxLines {
		if strings.Contains(ln, kittyPlaceholderPrefix) {
			hasPlacement = true
			break
		}
	}
	if !hasPlacement {
		return output
	}

	outputLines := strings.Split(output, "\n")
	for my, boxLine := range boxLines {
		if !strings.Contains(boxLine, kittyPlaceholderPrefix) {
			continue
		}
		outRow := startY + my
		if outRow < 0 || outRow >= len(outputLines) {
			continue
		}
		outputLines[outRow] = splice(outputLines[outRow], boxLine, startX)
	}
	return strings.Join(outputLines, "\n")
}

// Place composites box onto background with the box's top-left cell at
// column x of row y, without dimming or centering: a floating panel over
// the live screen (the which-key menu). Each row the box covers becomes
// the background up to x, the box row, then the background after it;
// rows and columns outside the box keep the background's bytes, escape
// sequences included. That is what keeps kitty images whole around the
// box: each placeholder cell carries its own row and column diacritics
// (image.buildPlaceholderLines) and the row's ID colour survives in the
// kept escapes, so the cells left and right of the box still draw their
// part of the image while the cells under it show the box. A wide
// character the box's edge cuts in half becomes a space.
func Place(background, box string, x, y int) string {
	rows := strings.Split(box, "\n")
	// Copy the rows above the box and below it as they are: only the
	// rows it covers are parsed.
	start := 0
	for range y {
		i := strings.IndexByte(background[start:], '\n')
		if i < 0 {
			return background // the box is below the last row
		}
		start += i + 1
	}
	var b strings.Builder
	b.Grow(len(background) + len(box) + 8*len(rows))
	b.WriteString(background[:start])
	rest := background[start:]
	for i, row := range rows {
		if y+i < 0 {
			continue // above the first row
		}
		line, after, more := strings.Cut(rest, "\n")
		b.WriteString(splice(line, row, x))
		if !more {
			rest = ""
			break
		}
		b.WriteByte('\n')
		rest = after
	}
	b.WriteString(rest)
	return b.String()
}

// splice replaces the cells of line from column x on with row: line's
// cells before x, a reset, row, a reset, then line's cells after the
// row's width. Every escape sequence of line is kept on both sides, so
// the cells after the box get back the style (or kitty image colour, or
// open hyperlink) they had, and whatever line closes after x is closed
// before the box. A wide character the box's edge cuts in half becomes
// a space in its style, so the line keeps its width (a line shorter
// than x is padded to it). One pass over line.
func splice(line, row string, x int) string {
	end := x + ansi.StringWidth(row)
	var pre, suf strings.Builder
	pre.Grow(len(line))
	suf.Grow(len(line))
	col := 0
	var state byte
	for s := line; s != ""; {
		seq, w, n, next := ansi.DecodeSequence(s, state, nil)
		if n == 0 {
			break
		}
		state, s = next, s[n:]
		switch {
		case w == 0: // an escape sequence or control: both sides keep it
			pre.WriteString(seq)
			suf.WriteString(seq)
		case col+w <= x:
			pre.WriteString(seq)
		case col >= end:
			suf.WriteString(seq)
		default: // under the box, or cut by one of its edges
			if col < x {
				pre.WriteString(strings.Repeat(" ", x-col))
			}
			if col+w > end {
				suf.WriteString(strings.Repeat(" ", col+w-end))
			}
		}
		col += w
	}
	if col < x {
		pre.WriteString(strings.Repeat(" ", x-col))
	}
	return pre.String() + "\x1b[m" + row + "\x1b[m" + suf.String()
}
