package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/text"
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

// openSlashApp is slashApp with the finder holding joined general (C1)
// and random (C2), a server channel search that answers with unjoined
// design (C9), and joins and searches recorded.
func openSlashApp(t *testing.T) (*App, *[]runCall, *[]string, *[]string) {
	t.Helper()
	a, runs := slashApp(t, nil)
	a.channelFinder.SetItems([]core.ChannelFinderItem{
		{ID: "C1", Name: "general", Type: "channel", Joined: true},
		{ID: "C2", Name: "random", Type: "channel", Joined: true},
	})
	joins := &[]string{}
	searches := &[]string{}
	a.SetChannelService(core.NewChannelService(core.ChannelServiceFuncs{
		Join: func(id ids.ChannelID, name string) core.Msg {
			*joins = append(*joins, string(id))
			return ChannelJoinedMsg{ID: string(id), Name: name}
		},
		SearchRemote: func(query string) []core.ChannelFinderItem {
			*searches = append(*searches, query)
			if text.Fold(query) == "design" {
				return []core.ChannelFinderItem{{ID: "C9", Name: "design", Type: "channel", Joined: false}}
			}
			return nil
		},
	}))
	return a, runs, joins, searches
}

func TestSlashOpen_SelectsAJoinedChannelByName(t *testing.T) {
	a, runs, _, searches := openSlashApp(t)
	a.compose.SetValue("/open #random")
	sel := deliveredMsgs[ChannelSelectedMsg](t, pressEnter(a))
	if len(sel) != 1 || sel[0].ID != "C2" {
		t.Fatalf("ChannelSelectedMsg = %+v, want C2", sel)
	}
	if len(*runs) != 0 {
		t.Errorf("chat.command ran %d times, want 0", len(*runs))
	}
	if len(*searches) != 0 {
		t.Errorf("server search ran for %v, want none for a local hit", *searches)
	}
}

func TestSlashJoin_SelectsByMentionID(t *testing.T) {
	a, _, _, _ := openSlashApp(t)
	a.compose.SetValue("/join <#C2|random>")
	sel := deliveredMsgs[ChannelSelectedMsg](t, pressEnter(a))
	if len(sel) != 1 || sel[0].ID != "C2" {
		t.Fatalf("ChannelSelectedMsg = %+v, want C2", sel)
	}
}

func TestSlashOpen_JoinsAChannelTheUserIsNotIn(t *testing.T) {
	a, _, joins, searches := openSlashApp(t)
	a.compose.SetValue("/open #Design")
	lookups := deliveredMsgs[channelLookupMsg](t, pressEnter(a))
	if len(*searches) != 1 || (*searches)[0] != "Design" {
		t.Fatalf("searches = %v, want [Design]", *searches)
	}
	_, cmd := a.Update(lookups[0])
	_ = deliveredMsgs[ChannelJoinedMsg](t, cmd)
	if len(*joins) != 1 || (*joins)[0] != "C9" {
		t.Fatalf("joins = %v, want [C9]", *joins)
	}
}

func TestSlashOpen_LookupForAnotherWorkspaceIsDropped(t *testing.T) {
	a, _, joins, _ := openSlashApp(t)
	a.compose.SetValue("/open #Design")
	lookups := deliveredMsgs[channelLookupMsg](t, pressEnter(a))
	a.activeTeamID = "T2"
	cmd, handled := reduceSlashCommand(a, lookups[0])
	if !handled || cmd != nil {
		t.Fatalf("reduceSlashCommand = (%v, %v), want a dropped answer (nil, true)", cmd, handled)
	}
	if len(*joins) != 0 {
		t.Errorf("joins = %v, want none", *joins)
	}
	if strings.Contains(statusbarText(a), "No channel named") {
		t.Errorf("status bar = %q, want no toast", statusbarText(a))
	}
}

func TestSlashOpen_ErrorsToast(t *testing.T) {
	cases := []struct {
		typed, want string
		lookup      bool
	}{
		{"/open #nope", "No channel named #nope", true},
		{"/open", "Usage: /open #channel", false},
	}
	for _, tc := range cases {
		t.Run(tc.typed, func(t *testing.T) {
			a, _, _, _ := openSlashApp(t)
			a.compose.SetValue(tc.typed)
			cmd := pressEnter(a)
			if tc.lookup {
				lookups := deliveredMsgs[channelLookupMsg](t, cmd)
				_, cmd = a.Update(lookups[0])
			}
			_ = cmd
			if !strings.Contains(statusbarText(a), tc.want) {
				t.Errorf("status bar = %q, want %q", statusbarText(a), tc.want)
			}
		})
	}
}

func TestMatchChannelByName(t *testing.T) {
	items := []core.ChannelFinderItem{
		{ID: "D7", Name: "design", Type: "dm"},
		{ID: "T0", Name: "threads", Type: "channel", Synthetic: true},
		{ID: "D2", Name: "alice", Type: "dm"},
		{ID: "G1", Name: "secret", Type: "private"},
		{ID: "C9", Name: "design", Type: "channel"},
		{ID: "C1", Name: "general", Type: "channel", Joined: true},
	}
	cases := []struct {
		name, query, wantID string
	}{
		{"exact", "general", "C1"},
		{"case-insensitive", "Design", "C9"},
		{"accent-insensitive", "Désign", "C9"},
		{"leading # ignored", "#design", "C9"},
		{"a DM with the same name is skipped", "design", "C9"},
		{"private channel matches", "secret", "G1"},
		{"synthetic destination is skipped", "threads", ""},
		{"DM-only name is a miss", "alice", ""},
		{"miss", "nope", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := matchChannelByName(items, tc.query)
			if tc.wantID == "" {
				if ok {
					t.Errorf("matchChannelByName(%q) = %+v, want no match", tc.query, got)
				}
				return
			}
			if !ok || got.ID != tc.wantID {
				t.Errorf("matchChannelByName(%q) = (%+v, %v), want %s", tc.query, got, ok, tc.wantID)
			}
		})
	}
}
