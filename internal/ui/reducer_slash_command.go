// internal/ui/reducer_slash_command.go
//
// Result of a slash command run through CommandService.
package ui

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

var reduceSlashCommand reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(commandRanMsg)
	if !ok {
		return nil, false
	}
	if m.err == nil {
		return nil, true
	}
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
