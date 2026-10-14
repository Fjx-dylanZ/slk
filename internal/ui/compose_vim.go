// internal/ui/compose_vim.go
//
// App side of [compose] vim_mode. The vim sub-modes (INSERT, NORMAL,
// VISUAL, V-LINE) are the composer's state inside app-level ModeInsert:
// the composer owns the keyboard until Esc leaves it, so no app
// Normal-mode key can fire from compose NORMAL. handleInsertMode routes
// Esc and Enter (see the design doc,
// docs/superpowers/specs/2026-10-13-compose-vim-mode-design.md).
package ui

import (
	"fmt"

	"github.com/gammons/slk/internal/bubbles/vimedit"
	"github.com/gammons/slk/internal/ui/compose"
)

// SetComposeVimMode turns vim editing on or off in both composers. They
// share one unnamed register, as vim's registers are shared between its
// windows: yank in a thread, put in the channel.
func (a *App) SetComposeVimMode(on bool) {
	var reg *vimedit.Register
	if on {
		reg = &vimedit.Register{}
	}
	a.compose.SetVimMode(reg)
	a.threadCompose.SetVimMode(reg)
}

// composeModeLabel is a status-bar mode text that is not a ui.Mode.
type composeModeLabel string

func (l composeModeLabel) String() string { return string(l) }

// composeMode is what the status bar's mode segment says for composer
// c while composing: INSERT as without vim mode, C-NORMAL (not NORMAL,
// so it reads differently from app Normal mode, which Esc returns to),
// VISUAL or V-LINE.
func composeMode(c *compose.Model) fmt.Stringer {
	mode, ok := c.VimMode()
	if !ok {
		return ModeInsert
	}
	switch mode {
	case vimedit.Normal:
		return composeModeLabel("C-NORMAL")
	case vimedit.Visual, vimedit.VisualLine:
		return composeModeLabel(mode.String())
	}
	return ModeInsert
}

// syncComposeModeLabel shows the focused composer's vim sub-mode in the
// status bar while in insert mode.
func (a *App) syncComposeModeLabel() {
	if a.mode != ModeInsert {
		return
	}
	target := &a.compose
	if a.focusedPanel == PanelThread && a.threadVisible {
		target = &a.threadCompose
	}
	a.statusbar.SetMode(composeMode(target))
}
