package linkpicker

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

// ViewOverlay renders the picker centered on a dimmed copy of
// background. Returns background unchanged when not visible.
func (m *Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.visible {
		return background
	}
	box := m.renderBox(termWidth)
	if box == "" {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}

func (m *Model) renderBox(termWidth int) string {
	s := messages.ChromeStyles()
	overlayWidth := termWidth * 6 / 10
	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayWidth > 80 {
		overlayWidth = 80
	}
	if overlayWidth > termWidth-2 {
		overlayWidth = termWidth - 2
	}
	innerWidth := overlayWidth - 4 // border + padding

	list := chrome.List{Start: 0, End: len(m.items), Total: len(m.items)}
	badgeStyle := s.Row.Foreground(styles.Accent)

	rows := make([]string, 0, len(m.items))
	for i, it := range m.items {
		var parts []string
		if it.Label != "" {
			parts = append(parts, it.Label)
		}
		if it.URL != "" && it.URL != it.Label {
			parts = append(parts, it.URL)
		}
		if it.Detail != "" {
			parts = append(parts, it.Detail)
		}
		text := strings.Join(parts, "  ")
		badge := ""
		if it.InApp {
			badge = " [slk]"
		}
		budget := list.ContentWidth(innerWidth) - lipgloss.Width(badge)
		if budget < 1 {
			budget = 1
		}
		if lipgloss.Width(text) > budget {
			text = truncate.StringWithTail(text, uint(budget), "\u2026")
		}
		style := s.Row
		if i == m.selected {
			style = s.RowSelected
		}
		rows = append(rows, style.Width(budget).Render(text)+badgeStyle.Render(badge))
	}

	return chrome.Frame{
		Title:  m.title,
		Body:   list.Rows(s, m.selected, rows),
		Footer: s.Footer.Render("j/k move   enter select   esc/q close"),
	}.Render(s, overlayWidth)
}
