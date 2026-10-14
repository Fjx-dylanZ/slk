// internal/ui/view_in_channel_test.go
//
// View in channel (gp in the thread pane, the view_in_channel leader
// action, a click on the thread header's channel name) through the real
// Update chain: from Activity, the Threads view and the channel view;
// the parent outside the buffer; a DM; the refusals; history; the key
// hints. Design: docs/superpowers/specs/2026-10-14-thread-view-in-channel-design.md.
package ui

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// The fixture's timestamps, in Slack's format so permalinks parse.
const (
	vicC1Old     = "1700000001.000000"
	vicC1Mid     = "1700000002.000000"
	vicC1New     = "1700000003.000000"
	vicC2Parent  = "1700000021.000000" // #random's thread, mid-buffer
	vicC2Reply   = "1700000021.000100"
	vicDMParent  = "1700000031.000000" // Priya Shah's DM thread
	vicFarParent = "1700000041.000000" // #ops's thread, outside its cache
	vicC9Parent  = "1700000091.000000" // a thread in a channel slk cannot open
)

// vicCalls records the service calls view in channel may make.
type vicCalls struct {
	threadFetches map[string]int // "channel/threadTS" → fetches
	around        []string       // "channel/ts" per FetchAround
}

// viewInChannelApp is an App in T1, 200 columns wide (the thread beside
// the channel), on #general (C1) with its middle message selected, so a
// departure from C1 names a message that is not the newest. Every
// channel but C9 resolves and has a fresh cache (a switch completes on
// its first render): #random (C2) holds a thread mid-buffer, the DM D1
// (Priya Shah) holds one, and #ops (C4) does not hold its thread's
// parent, which only FetchAround returns. Threads come from the thread
// cache and the fetch, both counted. The key-hints delay is short.
func viewInChannelApp(t *testing.T, opts ...testOpt) (*App, *vicCalls) {
	t.Helper()
	thread := func(parent string, replies ...string) []messages.MessageItem {
		out := []messages.MessageItem{{TS: parent, ThreadTS: parent, UserID: "U2", UserName: "bob", Text: "parent " + parent, ReplyCount: len(replies)}}
		for _, r := range replies {
			out = append(out, messages.MessageItem{TS: r, ThreadTS: parent, UserID: "U3", UserName: "carol", Text: "reply " + r})
		}
		return out
	}
	threads := map[string][]messages.MessageItem{
		vicC2Parent:  thread(vicC2Parent, vicC2Reply, "1700000021.000200"),
		vicDMParent:  thread(vicDMParent, "1700000031.000100"),
		vicFarParent: thread(vicFarParent, "1700000041.000100"),
		vicC9Parent:  thread(vicC9Parent, "1700000091.000100"),
	}
	caches := map[ids.ChannelID][]messages.MessageItem{
		"C1": {{TS: vicC1Old, Text: "c1 old"}, {TS: vicC1Mid, Text: "c1 mid"}, {TS: vicC1New, Text: "c1 new"}},
		"C2": {{TS: "1700000020.000000", Text: "before"}, threads[vicC2Parent][0], {TS: "1700000022.000000", Text: "after"}},
		"D1": {{TS: "1700000030.000000", Text: "hi"}, threads[vicDMParent][0]},
		"C4": {{TS: "1700000050.000000", Text: "much later"}},
	}
	names := map[ids.ChannelID][2]string{
		"C1": {"general", "channel"}, "C2": {"random", "channel"},
		"C4": {"ops", "channel"}, "D1": {"Priya Shah", "dm"},
	}
	calls := &vicCalls{threadFetches: map[string]int{}}
	a := newTestApp(t, append([]testOpt{
		withSize(200, 40),
		withActiveTeam("T1"),
		withChannels(
			sidebar.ChannelItem{ID: "C1", Name: "general", Type: "channel"},
			sidebar.ChannelItem{ID: "C2", Name: "random", Type: "channel"},
			sidebar.ChannelItem{ID: "C4", Name: "ops", Type: "channel"},
			sidebar.ChannelItem{ID: "D1", Name: "Priya Shah", Type: "dm"},
		),
		withChannelService(core.ChannelServiceFuncs{
			Lookup: func(id ids.ChannelID) (string, string, bool) {
				n, ok := names[id]
				return n[0], n[1], ok
			},
			ReadCache: func(id ids.ChannelID) []messages.MessageItem { return slices.Clone(caches[id]) },
			SyncedAt:  func(ids.ChannelID) int64 { return time.Now().Unix() },
			FetchAround: func(id ids.ChannelID, ts ids.MessageTS) core.Msg {
				calls.around = append(calls.around, string(id)+"/"+string(ts))
				return MessagesAroundLoadedMsg{ChannelID: string(id), TargetTS: string(ts), Messages: []messages.MessageItem{
					{TS: "1700000040.000000", Text: "before"}, threads[vicFarParent][0], {TS: "1700000042.000000", Text: "after"},
				}}
			},
		}),
	}, opts...)...)
	a.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{
		CacheRead: func(_ ids.ChannelID, ts ids.ThreadTS) []messages.MessageItem {
			return slices.Clone(threads[string(ts)])
		},
		Fetch: func(ch ids.ChannelID, ts ids.ThreadTS) core.Msg {
			calls.threadFetches[string(ch)+"/"+string(ts)]++
			return ThreadRepliesLoadedMsg{ThreadTS: string(ts), Replies: slices.Clone(threads[string(ts)][1:])}
		},
	}))
	a.SetKeyHintsDelay(testHintsDelay)
	a.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
	if !a.messagepane.SelectByTS(vicC1Mid) {
		t.Fatal("precondition: #general's middle message is not in the buffer")
	}
	return a, calls
}

// openFromActivity opens a thread the way Enter on an Activity item for
// a reply in it does, feeding its loads through Update.
func openFromActivity(t *testing.T, a *App, channelID, threadTS string) {
	t.Helper()
	a.Update(ActivityViewActivatedMsg{})
	_, cmd := a.Update(ActivitySelectedMsg{TeamID: "T1", Item: core.ActivityItem{
		Key: "k-" + threadTS, Type: "thread_v2", ChannelID: channelID, TS: threadTS, ThreadTS: threadTS,
	}})
	feed(t, a, cmd, 0)
	if a.view != ViewActivity || !a.threadVisible || a.focusedPanel != PanelThread || a.threadPanel.ThreadTS() != threadTS {
		t.Fatalf("precondition: view %v, thread visible %v (%s), focus %v; want %s's thread focused over Activity",
			a.view, a.threadVisible, a.threadPanel.ThreadTS(), a.focusedPanel, threadTS)
	}
}

// pressGP types g then p through Update and feeds what p started. g's
// own cmd is the key-hints delay tick, which is left alone.
func pressGP(t *testing.T, a *App) {
	t.Helper()
	a.Update(keyPress('g'))
	_, cmd := a.Update(keyPress('p'))
	feed(t, a, cmd, 0)
}

// wantInChannel asserts view in channel's end state: the channel view
// on channelID, parentTS selected in the channel pane and focused, that
// thread open beside it with its replies, nothing left pending.
func wantInChannel(t *testing.T, a *App, channelID, parentTS string) {
	t.Helper()
	if a.view != ViewChannels || a.activeChannelID != channelID {
		t.Fatalf("view %v on %q, want the channel view on %q", a.view, a.activeChannelID, channelID)
	}
	if sel, ok := a.messagepane.SelectedMessage(); !ok || sel.TS != parentTS {
		t.Errorf("channel pane selects %q (ok=%v), want the parent %s", sel.TS, ok, parentTS)
	}
	if a.focusedPanel != PanelMessages {
		t.Errorf("focus %v, want the channel pane", a.focusedPanel)
	}
	if !a.threadVisible || a.threadPanel.ChannelID() != channelID || a.threadPanel.ThreadTS() != parentTS {
		t.Errorf("thread visible %v: %s in %q, want %s in %q", a.threadVisible, a.threadPanel.ThreadTS(), a.threadPanel.ChannelID(), parentTS, channelID)
	}
	if len(a.threadPanel.Replies()) == 0 {
		t.Error("the thread beside the channel has no replies")
	}
	if a.pendingLinkNav != nil {
		t.Errorf("navigation left pending: %+v", *a.pendingLinkNav)
	}
}

func historyChannels(a *App) []string {
	if s := a.navHistory.Stack("T1"); s != nil {
		return navEntryChannelIDs(s.entries)
	}
	return nil
}

func TestViewInChannel_FromActivity(t *testing.T) {
	a, calls := viewInChannelApp(t)
	openFromActivity(t, a, "C2", vicC2Parent)
	a.Update(keyPress('k')) // read an older reply than the newest
	reading := a.threadPanel.SelectedReply()
	if reading == nil || reading.TS != vicC2Reply {
		t.Fatalf("precondition: reading %+v, want %s", reading, vicC2Reply)
	}
	fetches := maps.Clone(calls.threadFetches)

	pressGP(t, a)

	wantInChannel(t, a, "C2", vicC2Parent)
	// The thread pane is the one the user was reading, kept across the
	// switch: not closed and reloaded.
	if got := a.threadPanel.SelectedReply(); got == nil || got.TS != vicC2Reply {
		t.Errorf("the thread now selects %+v, want the reply being read, %s", got, vicC2Reply)
	}
	if !maps.Equal(calls.threadFetches, fetches) {
		t.Errorf("thread fetches %v, were %v: the thread was reloaded", calls.threadFetches, fetches)
	}
	if activityRowMarked(a) {
		t.Error("the sidebar still marks Activity over the channel view")
	}
	if got := historyChannels(a); !slices.Equal(got, []string{"C1", "C2"}) {
		t.Errorf("history %q, want [C1 C2]: a visit to the thread's channel", got)
	}
	frame := ansi.Strip(a.View().Content)
	if !strings.Contains(frame, "# random › Thread from bob") {
		t.Errorf("the frame shows no thread of #random beside the channel:\n%s", frame)
	}
}

func TestViewInChannel_FromThreadsView(t *testing.T) {
	a, _ := viewInChannelApp(t, withThreadsView([]cache.ThreadSummary{{
		ChannelID: "C2", ChannelName: "random", ChannelType: "channel",
		ThreadTS: vicC2Parent, ParentTS: vicC2Parent, ParentUserID: "U2", ParentText: "parent",
		ReplyCount: 2, LastReplyTS: vicC2Reply,
	}}))
	_, cmd := a.Update(ThreadsViewActivatedMsg{})
	feed(t, a, cmd, 0)
	_, cmd = a.Update(keyCode(tea.KeyEnter)) // into the thread, which Enter reloads
	feed(t, a, cmd, 0)
	if a.view != ViewThreads || a.focusedPanel != PanelThread || a.threadPanel.ThreadTS() != vicC2Parent {
		t.Fatalf("precondition: view %v, focus %v, thread %q", a.view, a.focusedPanel, a.threadPanel.ThreadTS())
	}

	pressGP(t, a)

	wantInChannel(t, a, "C2", vicC2Parent)
}

// A permalink into another channel's thread switches to that channel
// with the thread focused; gp from there completes in place: no reload,
// no reopened thread, no history entry.
func TestViewInChannel_AfterAPermalinkSelectsTheParentInPlace(t *testing.T) {
	a, calls := viewInChannelApp(t)
	a.workspaceDomains["T1"] = "acme"
	_, cmd := a.Update(OpenLinkMsg{URL: "https://acme.slack.com/archives/C2/p1700000021000100?thread_ts=" + vicC2Parent + "&cid=C2"})
	feed(t, a, cmd, 0)
	if a.activeChannelID != "C2" || a.focusedPanel != PanelThread || a.threadPanel.ThreadTS() != vicC2Parent {
		t.Fatalf("precondition: on %q, focus %v, thread %q; want C2's thread focused", a.activeChannelID, a.focusedPanel, a.threadPanel.ThreadTS())
	}
	fetches := maps.Clone(calls.threadFetches)
	history := historyChannels(a)
	buffer := a.messagepane.Messages()

	pressGP(t, a)

	wantInChannel(t, a, "C2", vicC2Parent)
	if !maps.Equal(calls.threadFetches, fetches) {
		t.Errorf("thread fetches %v, were %v: the open thread was reopened", calls.threadFetches, fetches)
	}
	if got := historyChannels(a); !slices.Equal(got, history) {
		t.Errorf("history %q, was %q", got, history)
	}
	if got := a.messagepane.Messages(); len(got) != len(buffer) {
		t.Errorf("the channel was reloaded: %d messages, were %d", len(got), len(buffer))
	}
}

// Ctrl+a back from Activity leaves the thread (another channel's) open
// over the channel view; gp from it switches to the thread's channel.
func TestViewInChannel_AnotherChannelsThreadOverTheChannelView(t *testing.T) {
	a, _ := viewInChannelApp(t)
	openFromActivity(t, a, "C2", vicC2Parent)
	a.Update(keyMod('a', tea.ModCtrl))
	a.Update(keyCode(tea.KeyTab))
	if a.view != ViewChannels || a.activeChannelID != "C1" || a.focusedPanel != PanelThread || a.threadPanel.ChannelID() != "C2" {
		t.Fatalf("precondition: view %v on %q, focus %v, thread of %q", a.view, a.activeChannelID, a.focusedPanel, a.threadPanel.ChannelID())
	}

	pressGP(t, a)

	wantInChannel(t, a, "C2", vicC2Parent)
}

func TestViewInChannel_ParentOutsideTheBufferIsFetchedAround(t *testing.T) {
	a, calls := viewInChannelApp(t)
	openFromActivity(t, a, "C4", vicFarParent)

	pressGP(t, a)

	if !slices.Equal(calls.around, []string{"C4/" + vicFarParent}) {
		t.Errorf("FetchAround calls %q, want one for the parent", calls.around)
	}
	wantInChannel(t, a, "C4", vicFarParent)
}

func TestViewInChannel_DMShowsThePersonsName(t *testing.T) {
	a, _ := viewInChannelApp(t)
	openFromActivity(t, a, "D1", vicDMParent)

	pressGP(t, a)

	wantInChannel(t, a, "D1", vicDMParent)
	frame := ansi.Strip(a.View().Content)
	if !strings.Contains(frame, "● Priya Shah › Thread") {
		t.Errorf("the thread header does not name the DM:\n%s", frame)
	}
	if s := statusbarText(a); !strings.Contains(s, "Priya Shah") {
		t.Errorf("status bar %q does not name the DM", s)
	}
}

// Every refusal leaves the thread, the view and the channel as they
// were, and says why.
func TestViewInChannel_Refusals(t *testing.T) {
	for _, tc := range []struct {
		name, channelID, threadTS, toast string
		setup                            func(t *testing.T, a *App)
	}{
		{name: "upload in flight", channelID: "C2", threadTS: vicC2Parent, toast: "Upload in progress",
			setup: func(t *testing.T, a *App) {
				a.setUploaderForTest(func(_, _, _ string, _ []core.PendingAttachment) tea.Cmd { return nil })
				a.compose.SetValue("caption")
				a.compose.AddAttachment(core.PendingAttachment{Filename: "shot.png", Bytes: []byte("x")})
				a.submitWithAttachments(&a.compose)
				if !a.compose.Uploading() {
					t.Fatal("precondition: no upload in flight")
				}
			}},
		{name: "channel slk cannot open", channelID: "C9", threadTS: vicC9Parent, toast: "Channel for this thread is unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := viewInChannelApp(t)
			openFromActivity(t, a, tc.channelID, tc.threadTS)
			if tc.setup != nil {
				tc.setup(t, a)
			}

			a.Update(keyPress('g'))
			a.Update(keyPress('p')) // a toast's cmd is its clear tick

			if s := statusbarText(a); !strings.Contains(s, tc.toast) {
				t.Errorf("status bar %q, want %q", s, tc.toast)
			}
			if a.view != ViewActivity || a.activeChannelID != "C1" || a.focusedPanel != PanelThread ||
				!a.threadVisible || a.threadPanel.ThreadTS() != tc.threadTS || a.pendingLinkNav != nil {
				t.Errorf("state moved: view %v on %q, focus %v, thread %v %q, pending %v",
					a.view, a.activeChannelID, a.focusedPanel, a.threadVisible, a.threadPanel.ThreadTS(), a.pendingLinkNav)
			}
		})
	}
}

// The leader action needs an open thread, not the thread pane's focus;
// gp is the thread pane's, and elsewhere p after g stays what it was.
func TestViewInChannel_WhereItActs(t *testing.T) {
	t.Run("leader action with no thread open", func(t *testing.T) {
		a, _ := viewInChannelApp(t)
		withLeaderMap(t, a, map[string][]string{"view_in_channel": {"x"}})
		a.Update(leaderKey)
		a.Update(keyPress('x'))
		if s := statusbarText(a); !strings.Contains(s, "No thread open") {
			t.Errorf("status bar %q, want the no-thread toast", s)
		}
		if a.pendingLinkNav != nil || a.activeChannelID != "C1" {
			t.Errorf("navigated with no thread: on %q, pending %v", a.activeChannelID, a.pendingLinkNav)
		}
	})
	t.Run("gp in the channel pane", func(t *testing.T) {
		a, _ := viewInChannelApp(t)
		openFromActivity(t, a, "C2", vicC2Parent)
		a.Update(keyMod('a', tea.ModCtrl)) // back to #general, focus on its pane
		pressGP(t, a)
		if a.activeChannelID != "C1" || a.pendingLinkNav != nil || a.threadPanel.ChannelID() != "C2" {
			t.Errorf("gp outside the thread pane navigated: on %q, thread of %q, pending %v", a.activeChannelID, a.threadPanel.ChannelID(), a.pendingLinkNav)
		}
	})
	t.Run("leader action from the channel pane", func(t *testing.T) {
		a, _ := viewInChannelApp(t)
		withLeaderMap(t, a, map[string][]string{"view_in_channel": {"x"}})
		openFromActivity(t, a, "C2", vicC2Parent)
		a.Update(keyMod('a', tea.ModCtrl))
		a.Update(leaderKey)
		_, cmd := a.Update(keyPress('x'))
		feed(t, a, cmd, 0)
		wantInChannel(t, a, "C2", vicC2Parent)
	})
}

// Back returns to the channel the thread was opened over, at the
// message the user had selected there; forward returns to the parent.
// The Activity view is not a history entry (spec §4).
func TestViewInChannel_HistoryBackAndForward(t *testing.T) {
	a, _ := viewInChannelApp(t)
	openFromActivity(t, a, "C2", vicC2Parent)
	pressGP(t, a)

	_, cmd := a.Update(keyMod('h', tea.ModCtrl))
	walkAndUpdate(t, a, cmd)
	if a.view != ViewChannels || a.activeChannelID != "C1" {
		t.Fatalf("back: view %v on %q, want #general", a.view, a.activeChannelID)
	}
	if sel, ok := a.messagepane.SelectedMessage(); !ok || sel.TS != vicC1Mid {
		t.Errorf("back selects %q, want %s, where the user was before Activity", sel.TS, vicC1Mid)
	}

	_, cmd = a.Update(keyMod('l', tea.ModCtrl))
	walkAndUpdate(t, a, cmd)
	if a.activeChannelID != "C2" {
		t.Fatalf("forward: on %q, want #random", a.activeChannelID)
	}
	if sel, ok := a.messagepane.SelectedMessage(); !ok || sel.TS != vicC2Parent {
		t.Errorf("forward selects %q, want the parent %s", sel.TS, vicC2Parent)
	}
}

// After g the menu lists gp in the thread pane only; the context menu
// there groups it under g, never as a key of its own; and the keys
// taken from the menus run it.
func TestViewInChannel_KeyHints(t *testing.T) {
	t.Run("the g menu", func(t *testing.T) {
		a, _ := viewInChannelApp(t)
		openFromActivity(t, a, "C2", vicC2Parent)
		a.Update(deliveredMsgs[keyHintsTickMsg](t, sendKey(a, keyPress('g')))[0])
		if got := rowKeys(a); !slices.Equal(got, []string{"g → top", "p → view thread in channel"}) {
			t.Fatalf("g menu in the thread pane: %q", got)
		}
		feed(t, a, sendKey(a, keyPress('p')), 0)
		wantInChannel(t, a, "C2", vicC2Parent)
		if a.keyHints.IsVisible() {
			t.Error("the menu stayed up")
		}
		a.Update(deliveredMsgs[keyHintsTickMsg](t, sendKey(a, keyPress('g')))[0])
		if got := rowKeys(a); !slices.Equal(got, []string{"g → top"}) {
			t.Errorf("g menu in the channel pane: %q", got)
		}
	})
	t.Run("the context menu", func(t *testing.T) {
		a, _ := viewInChannelApp(t)
		openFromActivity(t, a, "C2", vicC2Parent)
		openContext(t, a)
		wantRows(t, a, "g → +go to")
		unwantedRows(t, a, "p")
		sendKey(a, keyPress('g'))
		wantRows(t, a, "p → view thread in channel")
		feed(t, a, sendKey(a, keyPress('p')), 0)
		wantInChannel(t, a, "C2", vicC2Parent)
	})
}

// A click on the channel name in the thread header views the thread in
// that channel; a click elsewhere on the header does not.
func TestViewInChannel_ClickOnTheHeaderChannelName(t *testing.T) {
	a, _ := viewInChannelApp(t)
	openFromActivity(t, a, "C2", vicC2Parent)
	_ = a.View()
	if a.layout.threadEnd <= a.layout.msgEnd {
		t.Fatalf("precondition: thread pane not drawn beside the list (bands %+v)", *a.layout)
	}
	// The header row is under the pane's top border, after its left one.
	x, y := a.layout.msgEnd+1, 1
	click := func(x int) tea.Cmd {
		_, cmd := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		a.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
		return cmd
	}

	feed(t, a, click(x+12), 0) // "# random › Thread": the h of Thread
	if a.view != ViewActivity || a.pendingLinkNav != nil {
		t.Fatalf("a click past the channel name navigated: view %v, pending %v", a.view, a.pendingLinkNav)
	}

	feed(t, a, click(x), 0)
	wantInChannel(t, a, "C2", vicC2Parent)
}
