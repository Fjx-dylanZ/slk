// internal/ui/view_keyhints.go
//
// Drawing the key-hints menu (keyhints.go): bottom-right over the live
// screen, above the status bar, with no backdrop. It is spliced in
// after the compositor memo, so the memoized frame stays the one
// without it, and it is not an overlayActive modal: images outside it
// stay where they are (adaptive key hints design §7). View runs after
// every message, but the menu changes only with a key, a resize or a
// theme, so the drawn box and the last frame it went onto are kept
// (keyHintsDraw).
package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/keyhints"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
)

// keyHintStyles maps the theme onto the menu: the dialog chrome, keys
// in the primary colour, groups in the accent, the arrow muted. Built
// per draw, like messages.ChromeStyles.
func keyHintStyles() keyhints.Styles {
	c := messages.ChromeStyles()
	return keyhints.Styles{
		Chrome: c,
		Key:    c.Row.Foreground(styles.Primary).Bold(true),
		Arrow:  c.Footer,
		Desc:   c.Row,
		Group:  c.Row.Foreground(styles.Accent),
	}
}

// keyHintsArea is the room the menu may take: the screen less a column
// at each side and, at the bottom, the status bar; with one row free
// above it.
func (a *App) keyHintsArea() (w, h int) {
	return a.width - 2, a.height - 2
}

// keyHintsRect is where the menu went on screen; the zero value is no
// menu.
type keyHintsRect struct{ x, y, w, h int }

// keyHintsDraw is the menu as last drawn: the box and where it goes,
// valid while the model's version, the screen size and the theme are
// the ones it was drawn at; and the last frame the box went onto
// (base) with the result, reused while the frame beneath is the same
// (the compositor memo hands View the same string then).
type keyHintsDraw struct {
	drawn   bool
	version uint64
	w, h    int
	theme   int64
	box     string
	rect    keyHintsRect

	placed       bool
	base, screen string
}

// keyHintsBox is the open menu's box and where it goes: one column in
// from the right edge, its bottom border on the row above the status
// bar. "" when nothing is drawn: the menu is closed, Normal mode does
// not have the keyboard, a modal or the image preview is up, or the
// terminal is too small (the status bar's hint carries on then).
func (a *App) keyHintsBox() (string, keyHintsRect) {
	if !a.keyHints.IsVisible() || a.mode != ModeNormal || a.overlayActive() || a.preview.Active() {
		a.keyHintsDraw = keyHintsDraw{}
		return "", keyHintsRect{}
	}
	d := &a.keyHintsDraw
	version, theme := a.keyHints.Version(), styles.Version()
	if !d.drawn || d.version != version || d.w != a.width || d.h != a.height || d.theme != theme {
		w, h := a.keyHintsArea()
		box := a.keyHints.View(keyHintStyles(), w, h)
		*d = keyHintsDraw{drawn: true, version: version, w: a.width, h: a.height, theme: theme, box: box}
		if box != "" {
			bw, bh := lipgloss.Width(box), lipgloss.Height(box)
			d.rect = keyHintsRect{x: a.width - 1 - bw, y: a.height - 1 - bh, w: bw, h: bh}
		}
	}
	return d.box, d.rect
}

// placeKeyHints draws the open menu onto screen (keyHintsBox says where)
// and returns where it went. overlay.Place keeps every cell outside the
// box as it was, kitty images included.
func (a *App) placeKeyHints(screen string) (string, keyHintsRect) {
	box, r := a.keyHintsBox()
	if box == "" {
		return screen, keyHintsRect{}
	}
	if d := &a.keyHintsDraw; !d.placed || d.base != screen {
		d.placed, d.base, d.screen = true, screen, overlay.Place(screen, box, r.x, r.y)
	}
	return a.keyHintsDraw.screen, r
}

// outside drops the sixel placements r covers any cell of. Sixel is
// painted over the text, so an image under the menu would hide it; the
// painter erases a withheld placement and paints it again once the
// menu closes. Placements elsewhere stay.
func (r keyHintsRect) outside(ps []imgpkg.SixelPlacement) []imgpkg.SixelPlacement {
	out := ps[:0]
	for _, p := range ps {
		top, left := p.Row-1, p.Col-1 // placements are 1-based
		if top < r.y+r.h && r.y < top+p.Rows && left < r.x+r.w && r.x < left+p.Cols {
			continue
		}
		out = append(out, p)
	}
	return out
}
