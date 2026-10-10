package confirmprompt

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/truncate"

	"github.com/gammons/slk/internal/bubbles/ansi"
	"github.com/gammons/slk/internal/bubbles/buttonrow"
)

// Box width as a share of the terminal, and the bounds it is clamped to.
const (
	widthPercent = 35
	minWidth     = 40
	maxWidth     = 60
	// chrome is the border and horizontal padding the content sits inside.
	chrome = 4
	// buttonGap is the number of spaces between buttons.
	buttonGap = 3
)

// View renders the prompt box, or "" when hidden.
func (m Model) View() string {
	if !m.visible {
		return ""
	}

	width := boxWidth(m.width)
	content := strings.Join([]string{
		m.styles.Title.Render(m.choice.Title),
		m.styles.Body.Render("> " + preview(m.choice.Body, width-chrome)),
		m.renderButtons(width - chrome),
	}, "\n\n")

	return m.styles.Box.Width(width).Render(ansi.ReapplyAfterResets(content, m.styles.BaseANSI))
}

func boxWidth(termWidth int) int {
	return min(max(termWidth*widthPercent/100, minWidth), maxWidth)
}

// preview flattens body to one line and truncates it to limit columns.
func preview(body string, limit int) string {
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\t", " ")
	if lipgloss.Width(body) > limit {
		return truncate.StringWithTail(body, uint(limit), "…")
	}
	return body
}

// renderButtons draws the buttons as wrapping rows, the highlighted one in
// ButtonActive.
func (m Model) renderButtons(width int) string {
	labels := make([]string, len(m.choice.Buttons))
	for i := range labels {
		labels[i] = m.label(i)
	}
	placed := buttonrow.Layout(labels, width, buttonGap)
	gap := m.styles.Footer.Render(strings.Repeat(" ", buttonGap))
	rows := buttonrow.RowsFunc(placed, gap, func(p buttonrow.Placed) string {
		if p.Index == m.highlight {
			return m.styles.ButtonActive.Render(p.Text)
		}
		return m.styles.Button.Render(p.Text)
	})
	return strings.Join(rows, "\n")
}
