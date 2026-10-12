package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
)

// presenceSlashApp is slashApp with the status setter recorded and the
// workspace's cached presence set to presence ("" = no entry).
func presenceSlashApp(t *testing.T, presence string) (*App, *[]statusCall) {
	t.Helper()
	a, _ := slashApp(t, nil)
	if presence != "" {
		a.presence.Set("T1", presence, false, time.Time{})
	}
	statuses := &[]statusCall{}
	a.setStatusSetterForTest(func(action core.PresenceAction, mins int) {
		*statuses = append(*statuses, statusCall{action: action, mins: mins})
	})
	return a, statuses
}

func TestSlashPresence(t *testing.T) {
	cases := []struct {
		name, cached, typed string
		want                []statusCall
		toast               string
	}{
		{"/away when active goes away", "active", "/away", []statusCall{{action: core.PresenceSetAway}}, ""},
		{"/away when away comes back (toggle)", "away", "/away", []statusCall{{action: core.PresenceSetActive}}, ""},
		{"/away with nothing cached goes away", "", "/away", []statusCall{{action: core.PresenceSetAway}}, ""},
		{"/active", "away", "/active", []statusCall{{action: core.PresenceSetActive}}, ""},
		{"/status away", "active", "/status away", []statusCall{{action: core.PresenceSetAway}}, ""},
		{"/status Active is case-insensitive", "away", "/status Active", []statusCall{{action: core.PresenceSetActive}}, ""},
		{"bare /status is unsupported", "active", "/status", nil, "/status isn't supported in slk yet"},
		{"/status with a custom status is unsupported", "active", "/status :palm_tree: on holiday", nil, "/status isn't supported in slk yet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, statuses := presenceSlashApp(t, tc.cached)
			a.compose.SetValue(tc.typed)
			_ = pressEnter(a)
			if len(*statuses) != len(tc.want) {
				t.Fatalf("SetStatus calls = %+v, want %+v", *statuses, tc.want)
			}
			for i := range tc.want {
				if (*statuses)[i] != tc.want[i] {
					t.Errorf("SetStatus[%d] = %+v, want %+v", i, (*statuses)[i], tc.want[i])
				}
			}
			if tc.toast != "" && !strings.Contains(statusbarText(a), tc.toast) {
				t.Errorf("status bar = %q, want %q", statusbarText(a), tc.toast)
			}
		})
	}
}

func TestSlashAwayUpdatesTheCachedPresence(t *testing.T) {
	a, _ := presenceSlashApp(t, "active")
	a.compose.SetValue("/away")
	_ = pressEnter(a)
	if p, _, _, _ := a.presence.Status("T1"); p != "away" {
		t.Errorf("cached presence = %q, want away", p)
	}
}

// Built-ins run locally because their names are keys of slashCommands:
// dispatchCommand calls a table entry and never the remote fallback
// (TestDispatchCommand_*). This is the guard against a built-in being
// routed to chat.command; extend the list with each new built-in.
func TestSlashBuiltinsAreLocal(t *testing.T) {
	for _, name := range []string{"/away", "/active", "/status", "/open", "/join"} {
		if _, ok := slashCommands[name]; !ok {
			t.Errorf("%s is not in slashCommands, so it would run through chat.command", name)
		}
	}
}

// openSlashApp is slashApp with three channels in the finder (design
// not joined) and joins recorded.
func openSlashApp(t *testing.T) (*App, *[]runCall, *[]string) {
	t.Helper()
	a, runs := slashApp(t, nil)
	a.channelFinder.SetItems([]core.ChannelFinderItem{
		{ID: "C1", Name: "general", Type: "channel", Joined: true},
		{ID: "C2", Name: "random", Type: "channel", Joined: true},
		{ID: "C9", Name: "design", Type: "channel", Joined: false},
	})
	joins := &[]string{}
	a.SetChannelService(core.NewChannelService(core.ChannelServiceFuncs{
		Join: func(id ids.ChannelID, name string) core.Msg {
			*joins = append(*joins, string(id))
			return ChannelJoinedMsg{ID: string(id), Name: name}
		},
	}))
	return a, runs, joins
}

func TestSlashOpen_SelectsAJoinedChannelByName(t *testing.T) {
	a, runs, _ := openSlashApp(t)
	a.compose.SetValue("/open #random")
	sel := deliveredMsgs[ChannelSelectedMsg](t, pressEnter(a))
	if len(sel) != 1 || sel[0].ID != "C2" {
		t.Fatalf("ChannelSelectedMsg = %+v, want C2", sel)
	}
	if len(*runs) != 0 {
		t.Errorf("chat.command ran %d times, want 0", len(*runs))
	}
}

func TestSlashJoin_SelectsByMentionID(t *testing.T) {
	a, _, _ := openSlashApp(t)
	a.compose.SetValue("/join <#C2|random>")
	sel := deliveredMsgs[ChannelSelectedMsg](t, pressEnter(a))
	if len(sel) != 1 || sel[0].ID != "C2" {
		t.Fatalf("ChannelSelectedMsg = %+v, want C2", sel)
	}
}

func TestSlashOpen_JoinsAChannelTheUserIsNotIn(t *testing.T) {
	a, _, joins := openSlashApp(t)
	a.compose.SetValue("/open #Design")
	_ = deliveredMsgs[ChannelJoinedMsg](t, pressEnter(a))
	if len(*joins) != 1 || (*joins)[0] != "C9" {
		t.Fatalf("joins = %v, want [C9]", *joins)
	}
}

func TestSlashOpen_ErrorsToast(t *testing.T) {
	cases := map[string]string{
		"/open #nope": "No channel named #nope",
		"/open":       "Usage: /open #channel",
	}
	for typed, want := range cases {
		t.Run(typed, func(t *testing.T) {
			a, _, _ := openSlashApp(t)
			a.compose.SetValue(typed)
			_ = pressEnter(a)
			if !strings.Contains(statusbarText(a), want) {
				t.Errorf("status bar = %q, want %q", statusbarText(a), want)
			}
		})
	}
}
