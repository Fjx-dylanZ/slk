// internal/ui/mode_search.go
//
// Search-mode key handler: the `/` prompt.
//
// The prompt is an input buffer rendered in the status line's search
// segment, prefixed with what it searches (searchScope.prefix: "/" for
// the channel, "thread /", "threads /", "activity /", "channels /").
// Enter runs the query in that scope and returns to Normal mode (the
// channel's FTS result lands as a ChannelSearchResultsMsg, the
// client-side scopes install synchronously; see reducer_search.go). Esc
// cancels. In the sidebar scope every edit filters the sidebar live.
package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func handleSearchMode(a *App, msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, a.keys.Escape):
		// If a search is still active (the user re-entered `/` on
		// top of existing results and bailed), its i/N indicator
		// comes back instead of a blank segment.
		a.cancelSearchPrompt()
		return nil
	case key.Matches(msg, a.keys.Enter):
		return a.submitSearchPrompt()
	}
	// Text editing: append printable runes, backspace deletes.
	switch s := normalizeFinderKey(msg); s {
	case "backspace":
		if a.searchInput != "" {
			r := []rune(a.searchInput)
			a.searchInput = string(r[:len(r)-1])
		}
	case "space":
		// Key.String() renders a literal space as "space"; queries
		// can be multi-term, so map it back.
		a.searchInput += " "
	default:
		// Single-rune filter: accepts ordinary printable input but
		// drops multi-rune graphemes (emoji ZWJ sequences, some IME
		// commits arrive as multi-rune strings). Known v1 limitation.
		if len([]rune(s)) == 1 {
			a.searchInput += s
		}
	}
	a.searchPromptEdited()
	return nil
}
