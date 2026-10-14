// internal/ui/pane_focus.go
//
// Directional focus across every pane on screen (leader-keys design §5):
// ctrl+w h/j/k/l and <leader>w h/j/k/l move between the sidebar, the
// windows of the message region and the thread by geometry.
package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/wintree"
)

// paneTarget is one pane directional focus can land on.
type paneTarget struct {
	panel Panel
	// win is the window for a PanelMessages target in the channel
	// view; outside it the message region is one target carrying the
	// focused window.
	win  wintree.LeafID
	rect wintree.Rect
}

// paneTargets lists the focusable panes in screen order (sidebar, the
// message region's windows in tree order, thread) with their rects. The
// message region is the width the window tree fills (windowBounds), and
// the thread sits right of it, as drawn when both fit; in the stacked
// layout the thread keeps that place, so h/l move between the two the
// way Tab does. The workspace rail is never a target.
func (a *App) paneTargets() []paneTarget {
	var scratch panelLayout
	frame := scratch.Compute(a.width, a.height, a.workspaceRail.Width(), a.sidebar.Width(),
		a.sidebarVisible, a.threadVisible, false)
	h := frame.ContentHeight
	x := frame.RailWidth
	var out []paneTarget
	if a.sidebarVisible {
		w := frame.SidebarWidth + frame.SidebarBorder
		out = append(out, paneTarget{panel: PanelSidebar, rect: wintree.Rect{X: x, W: w, H: h}})
		x += w
	}
	region := wintree.Rect{X: x, W: frame.MsgWidth + frame.MsgBorder, H: h}
	if a.view == ViewChannels {
		rects := a.wins.ComputeRects(region)
		for _, id := range a.wins.Leaves() {
			out = append(out, paneTarget{panel: PanelMessages, win: id, rect: rects[id]})
		}
	} else {
		out = append(out, paneTarget{panel: PanelMessages, win: a.focusedWin, rect: region})
	}
	if a.threadVisible {
		// Side by side, the thread's drawn width; stacked, any width
		// will do: only its left edge and full height matter.
		w := max(frame.ThreadWidth+frame.ThreadBorder, 1)
		out = append(out, paneTarget{panel: PanelThread, rect: wintree.Rect{X: region.X + region.W, W: w, H: h}})
	}
	return out
}

// focusPaneDir moves focus to the pane next to the focused one in
// direction nd. Entering the message region from the sidebar or thread
// lands on the focused window when it borders them (the open thread is
// its thread); otherwise the neighbour sharing the longest edge, the
// first in screen order on ties. No neighbour is a silent no-op, like
// vim. One exception to the geometry: when the stacked thread is the
// only content pane on screen, l from the sidebar enters it, the pane
// the user sees there.
func (a *App) focusPaneDir(nd wintree.NavDir) tea.Cmd {
	targets := a.paneTargets()
	cur := -1
	for i, t := range targets {
		if t.panel == a.focusedPanel && (t.panel != PanelMessages || t.win == a.focusedWin) {
			cur = i
			break
		}
	}
	if cur < 0 {
		return nil
	}
	from := targets[cur].rect
	if from.W <= 0 || from.H <= 0 {
		return nil
	}
	if a.focusedPanel == PanelSidebar && nd == wintree.NavRight && a.threadDrawnAlone() {
		return a.focusPaneTarget(targets[len(targets)-1]) // the thread is listed last
	}
	if a.focusedPanel != PanelMessages {
		for _, t := range targets {
			if t.panel == PanelMessages && t.win == a.focusedWin {
				if _, ok := wintree.Adjacent(from, t.rect, nd); ok {
					return a.focusPaneTarget(t)
				}
			}
		}
	}
	rects := make([]wintree.Rect, len(targets))
	for i, t := range targets {
		rects[i] = t.rect
	}
	i, ok := wintree.Nearest(from, rects, nd)
	if !ok {
		return nil
	}
	return a.focusPaneTarget(targets[i])
}

// focusPaneTarget focuses t. Another window goes through focusWindow
// (which closes an open thread, window-management §7, and refuses during
// an upload); any other move is a plain panel change, as Tab makes.
func (a *App) focusPaneTarget(t paneTarget) tea.Cmd {
	if t.panel == PanelMessages && t.win != a.focusedWin {
		return a.focusWindow(t.win)
	}
	a.cancelEdit()
	a.clearSelections()
	a.focusedPanel = t.panel
	return nil
}
