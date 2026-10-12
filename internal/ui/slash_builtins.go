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
//
// A mention resolves by ID against the finder's rows. A name resolves
// there first; a name the user is not in is looked up on the server,
// since the finder holds only the user's own conversations until a
// search runs. The answer comes back as channelLookupMsg.
func slashOpen(a *App, args commandArgs) tea.Cmd {
	if strings.TrimSpace(args.Rest) == "" {
		return toastWithClear(a, "Usage: /open #channel", 3*time.Second)
	}
	if mentioned := slackfmt.MentionedChannelIDs(args.Rest); len(mentioned) > 0 {
		item, ok := a.findChannelByID(mentioned[0])
		if !ok {
			return toastWithClear(a, "No channel named "+strings.TrimSpace(args.Fields[0]), 3*time.Second)
		}
		return a.openChannel(channelResultOf(item))
	}
	name := strings.TrimPrefix(args.Fields[0], "#")
	if item, ok := matchChannelByName(a.channelFinder.Items(), name); ok {
		return a.openChannel(channelResultOf(item))
	}
	channels, teamID := a.channels, a.activeTeamID
	return func() tea.Msg {
		item, found := matchChannelByName(channels.SearchRemote(name), name)
		return channelLookupMsg{teamID: teamID, name: name, item: item, found: found}
	}
}

// channelLookupMsg is the server's answer to a /open name the finder did
// not hold. name is as the user typed it, without the "#"; teamID is the
// workspace the lookup ran in, so an answer that arrives after a switch
// is dropped.
type channelLookupMsg struct {
	teamID string
	name   string
	item   core.ChannelFinderItem
	found  bool
}

// findChannelByID returns the finder's row for a channel ID. Synthetic
// destinations never match.
func (a *App) findChannelByID(id string) (core.ChannelFinderItem, bool) {
	for _, it := range a.channelFinder.Items() {
		if !it.Synthetic && it.ID == id {
			return it, true
		}
	}
	return core.ChannelFinderItem{}, false
}

// matchChannelByName finds the channel named name among items, joined or
// not. A leading "#" is ignored and the match is case- and
// accent-insensitive. Only channels and private channels match: a DM
// named the same is not a channel, and synthetic destinations are never
// one.
func matchChannelByName(items []core.ChannelFinderItem, name string) (core.ChannelFinderItem, bool) {
	want := text.Fold(strings.TrimPrefix(name, "#"))
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

// channelResultOf is the finder result openChannel takes for a row.
func channelResultOf(it core.ChannelFinderItem) channelfinder.ChannelResult {
	return channelfinder.ChannelResult{ID: it.ID, Name: it.Name, Type: it.Type, Joined: it.Joined}
}

// slashMsg opens the DM with one user. The web client's /msg resolved
// the user and posted with chat.postMessage (spec §2); slk pre-fills the
// message in the DM's composer instead of sending it (spec §3.3). The
// text is wire form, so a mention inside it shows as <@U…> in the
// composer; it still sends correctly. /dm is its alias. The toasts are
// built only on the error paths: building one sets it.
func slashMsg(name string) commandFunc {
	return func(a *App, args commandArgs) tea.Cmd {
		usage := func() tea.Cmd {
			return toastWithClear(a, "Usage: "+name+" @someone [message]", 3*time.Second)
		}
		if len(args.Fields) == 0 {
			return usage()
		}
		first := args.Fields[0]
		users := slackfmt.MentionedUserIDs(first)
		if len(users) != 1 || first != "<@"+users[0]+">" {
			if strings.HasPrefix(first, "@") {
				return toastWithClear(a, "No one called "+first, 3*time.Second)
			}
			return usage()
		}
		text := strings.TrimLeft(strings.TrimPrefix(args.Rest, first), " \t\r\n")
		return a.openConversation([]string{users[0]}, text)
	}
}
