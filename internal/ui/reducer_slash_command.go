// internal/ui/reducer_slash_command.go
//
// Result of a slash command run through CommandService, and the
// server's answer to a /open lookup.
package ui

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

var reduceSlashCommand reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	if lookup, ok := msg.(channelLookupMsg); ok {
		// An answer for a workspace the user has since left is dropped.
		if lookup.teamID != a.activeTeamID {
			return nil, true
		}
		if !lookup.found {
			return toastWithClear(a, "No channel named #"+lookup.name, 3*time.Second), true
		}
		return a.openChannel(channelResultOf(lookup.item)), true
	}
	m, ok := msg.(commandRanMsg)
	if !ok {
		return nil, false
	}
	if m.err == nil {
		return nil, true
	}
	// Put the command back where it was typed, unless that conversation
	// has a draft again by now. A thread command goes back to the
	// thread composer, keyed by its thread; anything else to the
	// channel composer.
	c := &a.compose
	if m.threadTS != "" {
		c = &a.threadCompose
	}
	c.OfferDraft(m.teamID, m.channelID, m.threadTS, m.text)
	return toastWithClear(a, commandFailureText(m.name, m.err), 4*time.Second), true
}

// commandFailureText is the toast for a command that did not run.
func commandFailureText(name string, err error) string {
	var ce *core.CommandError
	switch {
	case errors.As(err, &ce) && ce.Code == "unknown_command":
		return name + " is not a valid command"
	case errors.As(err, &ce):
		return name + " failed: " + ce.Code
	case errors.Is(err, errors.ErrUnsupported):
		return name + " failed: not available"
	default:
		return name + " failed: " + err.Error()
	}
}
