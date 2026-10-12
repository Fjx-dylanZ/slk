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
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/gammons/slk/internal/text"
	"github.com/gammons/slk/internal/ui/channelfinder"
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

// slashOpen switches to a channel, joining it first if needed, as the
// web client's /open did (navigation only, no chat.command). /join is
// its alias (client_app_commands.json: alias_of "/open").
func slashOpen(a *App, args commandArgs) tea.Cmd {
	if strings.TrimSpace(args.Rest) == "" {
		return toastWithClear(a, "Usage: /open #channel", 3*time.Second)
	}
	item, ok := a.findChannelForCommand(args.Rest)
	if !ok {
		return toastWithClear(a, "No channel named "+strings.TrimSpace(args.Fields[0]), 3*time.Second)
	}
	return a.openChannel(channelfinder.ChannelResult{ID: item.ID, Name: item.Name, Type: item.Type, Joined: item.Joined})
}

// findChannelForCommand resolves a command's channel argument against
// the finder's rows: by ID when the composer translated it to
// <#C…|name>, otherwise by name ("#design" or "design", case- and
// accent-insensitive) among channels, joined or not.
func (a *App) findChannelForCommand(arg string) (core.ChannelFinderItem, bool) {
	items := a.channelFinder.Items()
	if mentioned := slackfmt.MentionedChannelIDs(arg); len(mentioned) > 0 {
		for _, it := range items {
			if !it.Synthetic && it.ID == mentioned[0] {
				return it, true
			}
		}
		return core.ChannelFinderItem{}, false
	}
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return core.ChannelFinderItem{}, false
	}
	want := text.Fold(strings.TrimPrefix(fields[0], "#"))
	for _, it := range items {
		if it.Synthetic || (it.Type != "channel" && it.Type != "private") {
			continue
		}
		if text.Fold(it.Name) == want {
			return it, true
		}
	}
	return core.ChannelFinderItem{}, false
}
