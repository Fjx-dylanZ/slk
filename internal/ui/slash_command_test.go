package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

func TestParseSlashCommand(t *testing.T) {
	cases := []struct {
		in         string
		name, args string
		ok         bool
	}{
		{"/giphy cats", "/giphy", "cats", true},
		{"/giphy", "/giphy", "", true},
		{"/GIPHY cats", "/giphy", "cats", true},                             // Review Focus 3
		{"/giphy   ", "/giphy", "", true},                                   // Review Focus 4
		{"/giphy \t\n ", "/giphy", "", true},                                // whitespace-only args
		{"/shrug line one\nline two", "/shrug", "line one\nline two", true}, // Review Focus 2
		{"/giphy\ncats", "/giphy", "cats", true},
		{"/my-cmd_2 x", "/my-cmd_2", "x", true},
		{"/remind me  to  stretch", "/remind", "me  to  stretch", true}, // inner spacing kept
		{" /giphy cats", "", "", false},                                 // Review Focus 1: leading space escapes
		{"/tmp/foo is full", "", "", false},
		{"/ hello", "", "", false},
		{"/", "", "", false},
		{"/giphy: cats", "", "", false},
		{"hello /giphy", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		name, args, ok := parseSlashCommand(tc.in)
		if name != tc.name || args != tc.args || ok != tc.ok {
			t.Errorf("parseSlashCommand(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, name, args, ok, tc.name, tc.args, tc.ok)
		}
	}
}

// runCall records one CommandService.Run.
type runCall struct {
	channelID, threadTS, name, args string
}

// slashApp is an App in insert mode on C1 in team T1, its channel
// composer bound to (T1, C1), whose command runs are recorded and
// answered with err.
func slashApp(t *testing.T, err error) (*App, *[]runCall) {
	t.Helper()
	a := newTestApp(t, append(insertOpts(), withActiveTeam("T1"), withMode(ModeInsert))...)
	a.focusedPanel = PanelMessages
	a.compose.SetDraftContext("T1", "C1", "")
	calls := &[]runCall{}
	a.setCommandRunnerForTest(func(ch ids.ChannelID, th ids.ThreadTS, name, args string) error {
		*calls = append(*calls, runCall{string(ch), string(th), name, args})
		return err
	})
	return a, calls
}

// pressEnter sends Enter through the real Update chain.
func pressEnter(a *App) tea.Cmd {
	_, cmd := a.Update(keyCode(tea.KeyEnter))
	return cmd
}

func TestSlashCommand_ChannelComposerRunsRemote(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/giphy cats")

	cmd := pressEnter(a)
	ran := deliveredMsgs[commandRanMsg](t, cmd)
	if len(ran) != 1 {
		t.Fatalf("commandRanMsg delivered %d times, want 1", len(ran))
	}
	want := runCall{channelID: "C1", threadTS: "", name: "/giphy", args: "cats"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("Run calls = %+v, want [%+v]", *calls, want)
	}
	if got := a.compose.Value(); got != "" {
		t.Errorf("compose = %q, want cleared", got)
	}
	if a.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (left insert like a send)", a.mode)
	}
}

func TestSlashCommand_ThreadComposerRunsInThread(t *testing.T) {
	a, calls := slashApp(t, nil)
	showThread(t, a)
	a.threadCompose.SetValue("/giphy cats")

	cmd := pressEnter(a)
	_ = deliveredMsgs[commandRanMsg](t, cmd)
	want := runCall{channelID: "C1", threadTS: "10.0", name: "/giphy", args: "cats"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("Run calls = %+v, want [%+v]", *calls, want)
	}
	if got := a.threadCompose.Value(); got != "" {
		t.Errorf("thread compose = %q, want cleared", got)
	}
}

func TestSlashCommand_UpperCaseNameRunsLowercased(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/GIPHY cats")
	_ = deliveredMsgs[commandRanMsg](t, pressEnter(a))
	if len(*calls) != 1 || (*calls)[0].name != "/giphy" {
		t.Fatalf("Run calls = %+v, want one /giphy", *calls)
	}
}

func TestSlashCommand_MultiLineArgsKeepNewline(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/shrug line one\nline two")
	_ = deliveredMsgs[commandRanMsg](t, pressEnter(a))
	if len(*calls) != 1 || (*calls)[0].args != "line one\nline two" {
		t.Fatalf("Run calls = %+v, want args with the newline", *calls)
	}
}

func TestSlashCommand_TrailingSpacesMeanNoArgs(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/giphy   ")
	_ = deliveredMsgs[commandRanMsg](t, pressEnter(a))
	if len(*calls) != 1 || (*calls)[0].args != "" {
		t.Fatalf("Run calls = %+v, want args \"\"", *calls)
	}
}

func TestSlashCommand_LeadingSpaceSendsAsMessage(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue(" /giphy cats")
	sent := deliveredMsgs[SendMessageMsg](t, pressEnter(a))
	if len(sent) != 1 || sent[0].Text != " /giphy cats" {
		t.Fatalf("SendMessageMsg = %+v, want the literal text", sent)
	}
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
}

func TestSlashCommand_PathLikeTextSendsAsMessage(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.SetValue("/tmp/foo is full")
	sent := deliveredMsgs[SendMessageMsg](t, pressEnter(a))
	if len(sent) != 1 {
		t.Fatalf("SendMessageMsg delivered %d times, want 1", len(sent))
	}
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
}

func TestSlashCommand_EditModeSavesLiterally(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.editing.Begin("C1", "1.0", PanelMessages, "")
	a.compose.SetValue("/giphy cats")
	edits := deliveredMsgs[EditMessageMsg](t, pressEnter(a))
	if len(edits) != 1 || edits[0].NewText != "/giphy cats" {
		t.Fatalf("EditMessageMsg = %+v, want the literal edit", edits)
	}
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
}

func TestSlashCommand_AttachmentsRefused(t *testing.T) {
	a, calls := slashApp(t, nil)
	a.compose.AddAttachment(core.PendingAttachment{Filename: "cat.png", Bytes: []byte{1}})
	a.compose.SetValue("/giphy cats")
	_ = pressEnter(a)
	if len(*calls) != 0 {
		t.Errorf("Run called %d times, want 0", len(*calls))
	}
	if got := a.compose.Value(); got != "/giphy cats" {
		t.Errorf("compose = %q, want the text kept", got)
	}
	if n := len(a.compose.Attachments()); n != 1 {
		t.Errorf("attachments = %d, want 1 kept", n)
	}
	if out := statusbarText(a); !strings.Contains(out, "Files can't be sent with a command") {
		t.Errorf("status bar = %q, want the refusal toast", out)
	}
}

func TestSlashCommand_FailureToasts(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&core.CommandError{Code: "unknown_command"}, "/giphy is not a valid command"},
		{&core.CommandError{Code: "restricted_action"}, "/giphy failed: restricted_action"},
		{errors.ErrUnsupported, "/giphy failed: not available"},
		{errors.New("calling chat.command: timeout"), "/giphy failed: calling chat.command: timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			a, _ := slashApp(t, tc.err)
			a.compose.SetValue("/giphy cats")
			ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
			a.Update(ran[0])
			if out := statusbarText(a); !strings.Contains(out, tc.want) {
				t.Errorf("status bar = %q, want %q", out, tc.want)
			}
		})
	}
}

// The GIF a command posts arrives with user = you and bot_id = the app
// (ws_posted_gif.json). A command never marks the channel in flight, so
// reduceNewMessage's self-send guards must not swallow it.
func TestSlashCommand_PostedResultIsNotSwallowedAsSelfSend(t *testing.T) {
	a, _ := slashApp(t, nil)
	a.currentUserID = "U0EXAMPLE01"
	a.compose.SetValue("/giphy cats")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.Update(ran[0])
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "99.000100", UserID: "U0EXAMPLE01", BotID: "B0EXAMPLE01", Text: "cats",
	}})
	msgs := a.messagepane.Messages()
	if len(msgs) == 0 || msgs[len(msgs)-1].TS != "99.000100" {
		t.Fatalf("last message = %+v, want the posted GIF 99.000100", msgs[len(msgs)-1:])
	}
}

func TestSlashCommand_SuccessIsSilent(t *testing.T) {
	a, _ := slashApp(t, nil)
	a.compose.SetValue("/giphy cats")
	ran := deliveredMsgs[commandRanMsg](t, pressEnter(a))
	a.Update(ran[0])
	if out := statusbarText(a); strings.Contains(out, "/giphy") {
		t.Errorf("status bar = %q, want no toast on success", out)
	}
}
