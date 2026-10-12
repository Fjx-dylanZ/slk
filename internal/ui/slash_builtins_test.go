package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/core"
)

// presenceSlashApp is slashApp with the status setter recorded and the
// workspace's cached presence set to presence ("" = no entry).
func presenceSlashApp(t *testing.T, presence string) (*App, *[]runCall, *[]statusCall) {
	t.Helper()
	a, runs := slashApp(t, nil)
	if presence != "" {
		a.presence.Set("T1", presence, false, time.Time{})
	}
	statuses := &[]statusCall{}
	a.setStatusSetterForTest(func(action core.PresenceAction, mins int) {
		*statuses = append(*statuses, statusCall{action: action, mins: mins})
	})
	return a, runs, statuses
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
			a, runs, statuses := presenceSlashApp(t, tc.cached)
			a.compose.SetValue(tc.typed)
			_ = pressEnter(a)
			if len(*runs) != 0 {
				t.Errorf("chat.command ran %d times, want 0 for a built-in", len(*runs))
			}
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
	a, _, _ := presenceSlashApp(t, "active")
	a.compose.SetValue("/away")
	_ = pressEnter(a)
	if p, _, _, _ := a.presence.Status("T1"); p != "away" {
		t.Errorf("cached presence = %q, want away", p)
	}
}
