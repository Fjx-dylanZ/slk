// internal/ui/slash_builtins.go
//
// Slash built-ins slk runs itself, because Slack's web client handles
// them locally instead of through chat.command. Each cites the capture
// in docs/superpowers/specs/2026-10-12-slash-commands-captures/ (or the
// spec §2 table) that shows the web client's behaviour.
package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

// slashAway toggles presence, as the web client's /away did twice in a
// row (presence_set_away.json, then presence.set active).
func slashAway(a *App, _ commandArgs) tea.Cmd {
	if p, _, _, _ := a.presence.Status(a.activeTeamID); p == "away" {
		a.applyPresenceAction(core.PresenceSetActive, 0)
		return nil
	}
	a.applyPresenceAction(core.PresenceSetAway, 0)
	return nil
}

// slashActive sets presence to active.
func slashActive(a *App, _ commandArgs) tea.Cmd {
	a.applyPresenceAction(core.PresenceSetActive, 0)
	return nil
}

// slashStatus handles "/status away" (captured: the web client called
// presence.set) and, by symmetry, "/status active". Bare /status and a
// custom status are not captured, so they are unsupported for now.
func slashStatus(a *App, args commandArgs) tea.Cmd {
	if len(args.Fields) == 1 {
		switch strings.ToLower(args.Fields[0]) {
		case "away":
			a.applyPresenceAction(core.PresenceSetAway, 0)
			return nil
		case "active":
			a.applyPresenceAction(core.PresenceSetActive, 0)
			return nil
		}
	}
	return slashUnsupported("/status")(a, args)
}

// slashUnsupported is a built-in the web client handles locally that slk
// does not do yet: it says so rather than sending it to chat.command.
func slashUnsupported(name string) commandFunc {
	return func(a *App, _ commandArgs) tea.Cmd {
		return toastWithClear(a, name+" isn't supported in slk yet", 3*time.Second)
	}
}
