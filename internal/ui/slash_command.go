// internal/ui/slash_command.go
//
// Slash commands typed in a composer (docs/superpowers/specs/
// 2026-10-12-slash-commands-design.md §3). Enter on text that starts
// with "/name" runs the command instead of posting the text.
package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/compose"
)

// parseSlashCommand splits composer text into a slash command's name and
// arguments. ok is false for anything that is not a command, which then
// sends as an ordinary message: text not starting with "/" (a leading
// space is the escape hatch), "/" alone, or a first token with anything
// but [A-Za-z0-9_-] after the slash ("/tmp/foo", "/giphy:"). name is
// lowercased; args keeps inner whitespace and newlines and is "" when
// only whitespace follows the name.
func parseSlashCommand(text string) (name, args string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	end := 1
	for end < len(text) && isCommandNameByte(text[end]) {
		end++
	}
	if end == 1 {
		return "", "", false
	}
	if end < len(text) && !isCommandSpace(text[end]) {
		return "", "", false
	}
	name = strings.ToLower(text[:end])
	args = strings.TrimLeft(text[end:], " \t\r\n")
	if strings.TrimSpace(args) == "" {
		args = ""
	}
	return name, args, true
}

func isCommandNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}

func isCommandSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// commandRanMsg is the result of running a slash command through
// CommandService. text is the composer text as typed, kept so a failure
// can put it back (Task 4); teamID/channelID/threadTS identify the
// conversation it was typed in.
type commandRanMsg struct {
	teamID, channelID, threadTS string
	name                        string
	text                        string
	err                         error
}

// trySlashCommand runs c's text as a slash command when it is one,
// called by both Enter branches of handleInsertMode before anything else
// is sent. It reports false for ordinary text and while panel is editing
// a message (an edit is saved as written), and the caller's send path
// runs unchanged. With files attached nothing is sent: a command cannot
// carry them.
func (a *App) trySlashCommand(panel Panel, c *compose.Model, channelID, threadTS string) (tea.Cmd, bool) {
	if a.editing.IsActive() && a.editing.Panel() == panel {
		return nil, false
	}
	raw := c.Value()
	name, _, ok := parseSlashCommand(raw)
	if !ok {
		return nil, false
	}
	if len(c.Attachments()) > 0 {
		return toastWithClear(a, "Files can't be sent with a command", 3*time.Second), true
	}
	// Translate @name/#name to wire form, as a send does. The name is
	// untouched by translation, so re-parsing yields the wire-form args.
	_, args, _ := parseSlashCommand(c.TranslateMentionsForSend(raw))
	c.Reset()
	a.exitInsertAfterSend()
	return tea.Batch(a.runRemoteCommand(channelID, threadTS, name, args, raw), a.tryOpenActionPrompt()), true
}

// runRemoteCommand runs name through CommandService off the Update
// goroutine. What the command produces arrives over the WebSocket; only
// the error comes back, in commandRanMsg.
func (a *App) runRemoteCommand(channelID, threadTS, name, args, text string) tea.Cmd {
	svc, teamID := a.commandSvc, a.activeTeamID
	return func() tea.Msg {
		err := svc.Run(ids.ChannelID(channelID), ids.ThreadTS(threadTS), name, args)
		return commandRanMsg{teamID: teamID, channelID: channelID, threadTS: threadTS, name: name, text: text, err: err}
	}
}
