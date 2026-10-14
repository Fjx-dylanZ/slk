// internal/ui/adaptive_search_test.go
//
// The focus-adaptive `/` (docs/superpowers/specs/2026-10-13-adaptive-search-design.md):
// scope resolution, the sidebar's live filter, the thread pane's and
// the Threads/Activity lists' client-side searches, and `/` staying a
// literal character where something is typed. The channel FTS keeps its
// own tests (reducer_search_test.go, mode_search_test.go).
package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/gammons/slk/internal/ui/styles"
)

// typeIntoApp sends each rune of s through the real Update chain (the
// generic typeKeys in golden_dialogs_test.go feeds a widget directly).
func typeIntoApp(a *App, s string) {
	for _, r := range s {
		a.Update(keyPress(r))
	}
}

// recordChannelSearches makes the App's SearchService count FTS calls,
// so a test can prove the hidden channel was never searched.
func recordChannelSearches(a *App) *int {
	calls := 0
	a.SetSearchService(core.NewSearchService(core.SearchServiceFuncs{
		SearchChannel: func(ids.ChannelID, string) core.Msg {
			calls++
			return nil
		},
	}))
	return &calls
}

// ---------------------------------------------------------------------
// Scope resolution
// ---------------------------------------------------------------------

func TestSlashScopeFollowsFocus(t *testing.T) {
	prompt := func(scope searchScope, segment string) func(*testing.T, *App, tea.Cmd) {
		return func(t *testing.T, a *App, _ tea.Cmd) {
			if a.searchScope != scope {
				t.Errorf("searchScope = %v, want %v", a.searchScope, scope)
			}
			if got := a.statusbar.Search(); got != segment {
				t.Errorf("statusbar search = %q, want %q", got, segment)
			}
		}
	}
	noPrompt := func(t *testing.T, a *App, _ tea.Cmd) {
		if got := a.statusbar.Search(); got != "" {
			t.Errorf("statusbar search = %q, want empty", got)
		}
	}
	runKeyCases(t, ModeNormal, []keyCase{
		{
			name:     "sidebar focused filters conversations",
			opts:     normalOpts(),
			setup:    func(_ *testing.T, a *App) { a.focusedPanel = PanelSidebar },
			key:      keyPress('/'),
			wantMode: ModeSearch,
			assert:   prompt(searchSidebar, "channels /"),
		},
		{
			name:     "messages pane in the channel view searches the channel",
			opts:     normalOpts(),
			setup:    focusMessages,
			key:      keyPress('/'),
			wantMode: ModeSearch,
			assert:   prompt(searchChannel, "/"),
		},
		{
			name:     "messages slot in the Threads view searches the list",
			opts:     append(normalOpts(), withView(ViewThreads)),
			setup:    func(_ *testing.T, a *App) { a.focusedPanel = PanelMessages },
			key:      keyPress('/'),
			wantMode: ModeSearch,
			assert:   prompt(searchThreadsList, "threads /"),
		},
		{
			name:     "messages slot in the Activity view searches the list",
			opts:     append(normalOpts(), withView(ViewActivity)),
			setup:    func(_ *testing.T, a *App) { a.focusedPanel = PanelMessages },
			key:      keyPress('/'),
			wantMode: ModeSearch,
			assert:   prompt(searchActivityList, "activity /"),
		},
		{
			name:     "thread pane focused searches the thread",
			opts:     normalOpts(),
			setup:    focusThreadPanel,
			key:      keyPress('/'),
			wantMode: ModeSearch,
			assert:   prompt(searchThread, "thread /"),
		},
		{
			// The rail never takes keyboard focus; if it did, `/`
			// must not search a pane that does not have it.
			name:     "workspace rail is a no-op",
			opts:     normalOpts(),
			setup:    func(_ *testing.T, a *App) { a.focusedPanel = PanelWorkspace },
			key:      keyPress('/'),
			wantMode: ModeNormal,
			assert:   noPrompt,
		},
		{
			name:     "thread focus without a thread is a no-op",
			opts:     normalOpts(),
			setup:    func(_ *testing.T, a *App) { a.focusedPanel = PanelThread },
			key:      keyPress('/'),
			wantMode: ModeNormal,
			assert:   noPrompt,
		},
	})
}

// ---------------------------------------------------------------------
// Sidebar: live conversation-name filter
// ---------------------------------------------------------------------

// sidebarSearchApp has the sidebar focused over conversations in the
// default config-mode sections, where Channels starts collapsed.
func sidebarSearchApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t,
		withChannels(
			sidebar.ChannelItem{ID: "C1", Name: "general", Type: "channel"},
			sidebar.ChannelItem{ID: "C2", Name: "random", Type: "channel"},
			sidebar.ChannelItem{ID: "C3", Name: "eng-backend", Type: "channel"},
			sidebar.ChannelItem{ID: "C4", Name: "eng-frontend", Type: "channel"},
			sidebar.ChannelItem{ID: "D1", Name: "alice", Type: "dm"},
		),
		withMessages(testMessageItems(3)...),
		withActiveChannel("C1"),
		withRender(),
	)
	a.focusedPanel = PanelSidebar
	if !a.sidebar.IsCollapsed("Channels") {
		t.Fatal("precondition: the Channels section starts collapsed")
	}
	return a
}

func visibleIDs(a *App) []string {
	var out []string
	for _, it := range a.sidebar.VisibleItems() {
		out = append(out, it.ID)
	}
	return out
}

func TestSidebarSlashFiltersLiveAndEnterKeepsIt(t *testing.T) {
	a := sidebarSearchApp(t)

	a.Update(keyPress('/'))
	if a.mode != ModeSearch || a.statusbar.Search() != "channels /" {
		t.Fatalf("after /: mode=%v segment=%q", a.mode, a.statusbar.Search())
	}
	typeIntoApp(a, "eng")
	if got := a.sidebar.Filter(); got != "eng" {
		t.Fatalf("live filter = %q, want %q", got, "eng")
	}
	if got := strings.Join(visibleIDs(a), ","); got != "C3,C4" {
		t.Errorf("visible = %s, want C3,C4", got)
	}
	// The matches sit in the collapsed Channels section: shown and
	// selected without touching the saved collapse state.
	if got := a.sidebar.SelectedID(); got != "C3" {
		t.Errorf("cursor on %q, want the first match C3", got)
	}
	if !a.sidebar.IsCollapsed("Channels") {
		t.Error("filtering expanded the saved collapse state")
	}
	if got := a.statusbar.Search(); got != "channels /eng" {
		t.Errorf("segment = %q, want %q", got, "channels /eng")
	}
	screen := ansi.Strip(a.View().Content)
	for _, want := range []string{"/eng", "eng-backend", "eng-frontend"} {
		if !strings.Contains(screen, want) {
			t.Errorf("filtered frame lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "random") {
		t.Errorf("filtered frame still shows a non-match:\n%s", screen)
	}

	a.Update(keyCode(tea.KeyEnter))
	if a.mode != ModeNormal {
		t.Fatalf("mode after Enter = %v", a.mode)
	}
	if got := a.sidebar.Filter(); got != "eng" {
		t.Errorf("Enter dropped the filter: %q", got)
	}
	if got := a.statusbar.Search(); got != "" {
		t.Errorf("segment after Enter = %q, want empty (the sidebar shows the filter)", got)
	}

	// j/k walk the matches; Enter opens one.
	a.Update(keyPress('j'))
	if got := a.sidebar.SelectedID(); got != "C4" {
		t.Fatalf("j moved to %q, want C4", got)
	}
	_, cmd := a.Update(keyCode(tea.KeyEnter))
	var opened string
	for _, m := range drainCmd(cmd) {
		if cs, ok := m.(ChannelSelectedMsg); ok {
			opened = cs.ID
			a.Update(m)
		}
	}
	if opened != "C4" {
		t.Fatalf("Enter opened %q, want C4", opened)
	}
	if got := a.sidebar.Filter(); got != "eng" {
		t.Errorf("opening a match dropped the filter: %q", got)
	}
}

func TestSidebarSlashEscRestoresFilterAndCursor(t *testing.T) {
	a := sidebarSearchApp(t)
	typeIntoApp(a, "/eng")
	a.Update(keyCode(tea.KeyEnter))
	a.Update(keyPress('j')) // C4

	a.Update(keyPress('/'))
	if a.searchInput != "eng" || a.statusbar.Search() != "channels /eng" {
		t.Fatalf("prompt not prefilled with the filter: input=%q segment=%q", a.searchInput, a.statusbar.Search())
	}
	for range 3 {
		a.Update(keyCode(tea.KeyBackspace))
	}
	typeIntoApp(a, "ran")
	if got := a.sidebar.SelectedID(); got != "C2" {
		t.Fatalf("live edit selected %q, want C2", got)
	}

	a.Update(keyCode(tea.KeyEscape))
	if a.mode != ModeNormal {
		t.Fatalf("mode after Esc = %v", a.mode)
	}
	if got := a.sidebar.Filter(); got != "eng" {
		t.Errorf("Esc left filter %q, want the previous %q", got, "eng")
	}
	if got := a.sidebar.SelectedID(); got != "C4" {
		t.Errorf("Esc left the cursor on %q, want the previous C4", got)
	}
}

func TestSidebarEscInNormalClearsFilterFirst(t *testing.T) {
	a := sidebarSearchApp(t)
	typeIntoApp(a, "/front")
	a.Update(keyCode(tea.KeyEnter))
	// A channel search is active too: the focused sidebar's filter
	// goes first.
	a.search = &activeSearch{query: "x", matches: []string{"1.0"}}
	a.statusbar.SetSearch("/x  1/1")

	a.Update(keyCode(tea.KeyEscape))
	if got := a.sidebar.Filter(); got != "" {
		t.Fatalf("Esc left filter %q", got)
	}
	if got := a.sidebar.SelectedID(); got != "C4" {
		t.Errorf("cursor on %q after clearing, want the conversation it was on (C4)", got)
	}
	if a.search == nil {
		t.Error("the first Esc also cleared the channel search")
	}

	a.Update(keyCode(tea.KeyEscape))
	if a.search != nil {
		t.Error("the second Esc did not clear the channel search")
	}
}

func TestSidebarEmptyEnterClearsAndPasteFilters(t *testing.T) {
	a := sidebarSearchApp(t)
	a.Update(keyPress('/'))
	a.Update(tea.PasteMsg{Content: "back\n"})
	if got := a.sidebar.Filter(); got != "back" {
		t.Fatalf("paste filter = %q, want %q", got, "back")
	}
	if got := strings.Join(visibleIDs(a), ","); got != "C3" {
		t.Errorf("visible = %s, want C3", got)
	}
	a.Update(keyCode(tea.KeyEnter))

	a.Update(keyPress('/'))
	for range len("back ") {
		a.Update(keyCode(tea.KeyBackspace))
	}
	a.Update(keyCode(tea.KeyEnter))
	if got := a.sidebar.Filter(); got != "" {
		t.Errorf("empty Enter left filter %q", got)
	}
	if got := len(a.sidebar.VisibleItems()); got != 5 {
		t.Errorf("visible after clearing = %d, want 5", got)
	}
}

func TestSidebarFilterClearedByWorkspaceSwitch(t *testing.T) {
	a := sidebarSearchApp(t)
	typeIntoApp(a, "/eng")
	a.Update(keyCode(tea.KeyEnter))
	a.Update(WorkspaceSwitchedMsg{TeamID: "T2", TeamName: "Other"})
	if got := a.sidebar.Filter(); got != "" {
		t.Errorf("filter %q survived a workspace switch", got)
	}
}

// Regression: clicking a match while the sidebar prompt was open opened
// it but left ModeSearch on with an empty buffer, so the next key
// replaced the filter. A click commits the sidebar prompt first.
func TestSidebarClickCommitsThePrompt(t *testing.T) {
	a := sidebarSearchApp(t)
	typeIntoApp(a, "/eng")
	x := a.layout.RailWidth() + 1
	y := -1
	for i, line := range strings.Split(ansi.Strip(a.View().Content), "\n") {
		if strings.Contains(line, "eng-backend") {
			y = i
			break
		}
	}
	if y < 0 {
		t.Fatal("precondition: eng-backend is not drawn")
	}

	_, cmd := a.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if a.mode != ModeNormal {
		t.Fatalf("mode after the click = %v, want NORMAL", a.mode)
	}
	var opened string
	for _, m := range drainCmd(cmd) {
		if cs, ok := m.(ChannelSelectedMsg); ok {
			opened = cs.ID
			a.Update(m)
		}
	}
	if opened != "C3" {
		t.Fatalf("click opened %q, want C3", opened)
	}
	a.Update(keyPress('j'))
	if got := a.sidebar.Filter(); got != "eng" {
		t.Errorf("filter after the click and j = %q, want %q kept", got, "eng")
	}
	if got := a.statusbar.Search(); got != "" {
		t.Errorf("segment = %q, want empty", got)
	}
}

// With a filter active, a/A walk only the matches: an unread
// conversation outside the filter is not opened.
func TestNextUnreadWalksOnlyFilterMatches(t *testing.T) {
	a := sidebarSearchApp(t)
	a.setReadStateReaderForTest(func() map[string]core.ReadState {
		return map[string]core.ReadState{"C2": {HasUnread: true}}
	})
	typeIntoApp(a, "/eng")
	a.Update(keyCode(tea.KeyEnter))
	a.Update(keyPress('a'))
	if got := a.sidebar.SelectedID(); got == "C2" {
		t.Fatal("a selected C2, which the filter hides")
	}

	// Control: without the filter the same press opens C2.
	a.Update(keyCode(tea.KeyEscape)) // the focused sidebar's filter goes first
	_, cmd := a.Update(keyPress('a'))
	var opened string
	for _, m := range drainCmd(cmd) {
		if cs, ok := m.(ChannelSelectedMsg); ok {
			opened = cs.ID
		}
	}
	if opened != "C2" {
		t.Errorf("unfiltered a opened %q, want C2", opened)
	}
}

// ---------------------------------------------------------------------
// Thread pane: client-side search of the loaded thread
// ---------------------------------------------------------------------

// threadSearchApp opens a thread whose parent and two replies mention
// deploy, focused, with its newest reply selected.
func threadSearchApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t, normalOpts()...)
	a.threadPanel.SetThread(
		messages.MessageItem{TS: "10.0", UserName: "alice", Text: "deploy plan", ThreadTS: "10.0"},
		[]messages.MessageItem{
			{TS: "11.0", UserName: "bob", Text: "lunch?"},
			{TS: "12.0", UserName: "bob", Text: "deploy done"},
			{TS: "13.0", UserName: "carol", Text: "Deployment rolled back"},
			{TS: "14.0", UserName: "dave", Text: "redeploy tomorrow"},
		},
		"C1", "10.0")
	a.threadVisible = true
	a.focusedPanel = PanelThread
	return a
}

func selectedThreadTS(a *App) string {
	m, _ := a.threadPanel.SelectedMessage()
	return m.TS
}

func TestThreadSlashSearchesLoadedRepliesAndSteps(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	hlStart, _, ok := messages.SearchHighlightSGR()
	if !ok {
		t.Fatal("could not derive highlight SGR")
	}

	a := threadSearchApp(t)
	fts := recordChannelSearches(a)
	searchDispatch(t, a, "deploy")

	if *fts != 0 {
		t.Errorf("thread search ran the channel FTS %d times", *fts)
	}
	s := a.search
	if s == nil || s.scope != searchThread {
		t.Fatalf("active search = %+v, want a thread search", s)
	}
	// Word-start AND matching: "redeploy" does not match. Newest
	// first, so n walks up through the thread.
	if got := strings.Join(s.matches, ","); got != "13.0,12.0,10.0" {
		t.Fatalf("matches = %s, want 13.0,12.0,10.0", got)
	}
	// The cursor was on the newest reply (14.0): the nearest match at
	// or older than it is 13.0.
	if got := selectedThreadTS(a); got != "13.0" {
		t.Errorf("selected %s, want 13.0", got)
	}
	if got := a.statusbar.Search(); got != "thread /deploy  1/3" {
		t.Errorf("segment = %q", got)
	}
	if view := a.threadPanel.View(30, 80); !strings.Contains(view, hlStart+"deploy") || !strings.Contains(view, hlStart+"Deploy") {
		t.Errorf("thread does not highlight the matches:\n%q", view)
	}

	a.Update(keyPress('n'))
	if got := selectedThreadTS(a); got != "12.0" {
		t.Errorf("n selected %s, want 12.0", got)
	}
	a.Update(keyPress('n'))
	if got := selectedThreadTS(a); got != "10.0" {
		t.Errorf("n selected %s, want the parent 10.0", got)
	}
	_, cmd := a.Update(keyPress('n'))
	if got := selectedThreadTS(a); got != "13.0" {
		t.Errorf("n wrap selected %s, want 13.0", got)
	}
	wrapped := false
	for _, m := range drainCmd(cmd) {
		if tm, ok := m.(ToastMsg); ok && tm.Text == "Search wrapped" {
			wrapped = true
		}
	}
	if !wrapped {
		t.Error("no \"Search wrapped\" toast")
	}
	a.Update(keyPress('N'))
	if got := a.statusbar.Search(); got != "thread /deploy  3/3" {
		t.Errorf("N wrap segment = %q", got)
	}

	// Esc clears the search and its highlights; the thread stays.
	a.Update(keyCode(tea.KeyEscape))
	if a.search != nil || a.statusbar.Search() != "" {
		t.Fatalf("Esc left search=%+v segment=%q", a.search, a.statusbar.Search())
	}
	if !a.threadVisible {
		t.Error("Esc closed the thread instead of clearing the search")
	}
	if view := a.threadPanel.View(30, 80); strings.Contains(view, hlStart) {
		t.Error("highlights survived Esc")
	}
}

// A thread search that matches nothing still owns its "no matches"
// segment, which goes away with the thread (regression: it lingered
// after the thread closed, and the next Esc was spent clearing it).
func TestThreadSearchNoMatches(t *testing.T) {
	a := threadSearchApp(t)
	searchDispatch(t, a, "zzz")
	if a.search == nil || len(a.search.matches) != 0 {
		t.Fatalf("no-match search = %+v, want an installed search with no matches", a.search)
	}
	if got := a.statusbar.Search(); got != "thread /zzz  no matches" {
		t.Errorf("segment = %q", got)
	}
	a.Update(keyPress('n'))
	if got := a.statusbar.Search(); got != "thread /zzz  no matches" {
		t.Errorf("segment after n = %q", got)
	}
	a.CloseThread()
	if a.search != nil || a.statusbar.Search() != "" {
		t.Errorf("closing the thread left search=%+v segment=%q", a.search, a.statusbar.Search())
	}
}

func TestListSearchNoMatchesGoesWithTheView(t *testing.T) {
	a := threadsListApp(t)
	searchDispatch(t, a, "zzz")
	if got := a.statusbar.Search(); got != "threads /zzz  no matches" {
		t.Fatalf("segment = %q", got)
	}
	a.Update(ActivityViewActivatedMsg{})
	if a.search != nil || a.statusbar.Search() != "" {
		t.Errorf("view switch left search=%+v segment=%q", a.search, a.statusbar.Search())
	}
}

func TestThreadSearchDroppedWhenTheThreadGoes(t *testing.T) {
	t.Run("closed", func(t *testing.T) {
		a := threadSearchApp(t)
		searchDispatch(t, a, "deploy")
		a.CloseThread()
		if a.search != nil || a.statusbar.Search() != "" {
			t.Errorf("close left search=%+v segment=%q", a.search, a.statusbar.Search())
		}
	})
	t.Run("another thread opened", func(t *testing.T) {
		a := threadSearchApp(t)
		searchDispatch(t, a, "deploy")
		_ = a.openThreadPanel(messages.MessageItem{TS: "20.0", Text: "deploy again", ThreadTS: "20.0"}, "C1", "20.0")
		if a.search != nil || a.statusbar.Search() != "" {
			t.Errorf("switching threads left search=%+v segment=%q", a.search, a.statusbar.Search())
		}
	})
}

func TestThreadSearchStepsOnlyWithTheThreadFocused(t *testing.T) {
	a := threadSearchApp(t)
	searchDispatch(t, a, "deploy")
	a.focusedPanel = PanelMessages
	a.Update(keyPress('n'))
	if a.search == nil || a.search.idx != 0 {
		t.Fatalf("n stepped the thread search from the messages pane: %+v", a.search)
	}
}

func TestChannelSearchDispatchDropsThreadSearch(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	hlStart, _, _ := messages.SearchHighlightSGR()

	a := threadSearchApp(t)
	searchDispatch(t, a, "deploy")
	a.focusedPanel = PanelMessages
	_ = searchDispatch(t, a, "msg")
	if a.search != nil {
		t.Errorf("thread search survived a channel dispatch: %+v", a.search)
	}
	if view := a.threadPanel.View(30, 80); strings.Contains(view, hlStart) {
		t.Error("thread highlights survived a channel dispatch")
	}
	if got := a.statusbar.Search(); got != "/msg  …" {
		t.Errorf("segment = %q, want the pending channel search", got)
	}
}

// ---------------------------------------------------------------------
// Threads and Activity views: client-side search of the list
// ---------------------------------------------------------------------

func threadsListApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t,
		withThreadsView([]core.ThreadSummary{
			{ChannelID: "C1", ChannelName: "general", ThreadTS: "3.0", ParentText: "deploy plan", ParentUserID: "U1"},
			{ChannelID: "C2", ChannelName: "random", ThreadTS: "2.0", ParentText: "lunch?", ParentUserID: "U1"},
			{ChannelID: "C3", ChannelName: "eng", ThreadTS: "1.0", ParentText: "Deploy rollback", ParentUserID: "U2"},
		}),
		withActiveChannel("C9"),
		withView(ViewThreads),
		withRender(),
	)
	a.focusedPanel = PanelMessages
	return a
}

func TestThreadsListSlashSelectsMatchingCards(t *testing.T) {
	a := threadsListApp(t)
	fts := recordChannelSearches(a)
	searchDispatch(t, a, "deploy")
	if *fts != 0 {
		t.Errorf("the hidden channel was searched (%d FTS calls)", *fts)
	}
	if a.search == nil || a.search.scope != searchThreadsList {
		t.Fatalf("active search = %+v, want a Threads-list search", a.search)
	}
	if got := a.threadsView.SelectedIndex(); got != 0 {
		t.Errorf("selected card %d, want 0", got)
	}
	a.Update(keyPress('n'))
	if got := a.threadsView.SelectedIndex(); got != 2 {
		t.Errorf("n selected card %d, want 2 (the next older match)", got)
	}
	if got := a.statusbar.Search(); got != "threads /deploy  2/2" {
		t.Errorf("segment = %q", got)
	}
	// The thread pane follows the selection, as j/k make it.
	if got := a.threadPanel.ThreadTS(); got != "1.0" {
		t.Errorf("thread pane shows %q, want the selected card's thread 1.0", got)
	}

	// Leaving the view drops its search.
	a.Update(ActivityViewActivatedMsg{})
	if a.search != nil || a.statusbar.Search() != "" {
		t.Errorf("view switch left search=%+v segment=%q", a.search, a.statusbar.Search())
	}
}

func TestActivityListSlashSelectsMatchingCards(t *testing.T) {
	a := newTestApp(t, append(normalOpts(), withView(ViewActivity))...)
	a.activityView.SetItems([]core.ActivityItem{
		{Key: "K1", Type: "at_user", ChannelID: "C1", TS: "3.0", AuthorID: "U1"},
		{Key: "K2", Type: "at_user", ChannelID: "C1", TS: "2.0", AuthorID: "U1"},
		{Key: "K3", Type: "message_reaction", ChannelID: "C2", TS: "1.0", AuthorID: "U2", Reaction: "tada"},
	})
	a.activityView.SetBodies(map[string]core.ActivityMessage{
		core.ActivityMsgKey("C1", "3.0"): {Text: "standup in 5"},
		core.ActivityMsgKey("C1", "2.0"): {Text: "deploy is green"},
		core.ActivityMsgKey("C2", "1.0"): {Text: "deployed it"},
	})
	a.focusedPanel = PanelMessages
	fts := recordChannelSearches(a)

	searchDispatch(t, a, "deploy")
	if *fts != 0 {
		t.Errorf("the hidden channel was searched (%d FTS calls)", *fts)
	}
	if got := a.activityView.SelectedIndex(); got != 1 {
		t.Errorf("selected card %d, want 1", got)
	}
	a.Update(keyPress('n'))
	if got := a.activityView.SelectedIndex(); got != 2 {
		t.Errorf("n selected card %d, want 2", got)
	}
	if got := a.statusbar.Search(); got != "activity /deploy  2/2" {
		t.Errorf("segment = %q", got)
	}

	// ctrl+a back to the channel drops the Activity search.
	a.Update(keyMod('a', tea.ModCtrl))
	if a.view != ViewChannels {
		t.Fatalf("ctrl+a left view %v", a.view)
	}
	if a.search != nil || a.statusbar.Search() != "" {
		t.Errorf("leaving Activity left search=%+v segment=%q", a.search, a.statusbar.Search())
	}
}

// A channel search never steps while a list view hides the channel.
func TestChannelSearchDoesNotStepBehindAListView(t *testing.T) {
	a := newTestApp(t, append(normalOpts(), withView(ViewThreads))...)
	a.focusedPanel = PanelMessages
	seedActiveSearch(t, a)
	a.Update(keyPress('n'))
	if a.search.idx != 0 {
		t.Errorf("n stepped the hidden channel search to %d", a.search.idx)
	}
}

// ---------------------------------------------------------------------
// `/` stays literal where something is typed
// ---------------------------------------------------------------------

func TestSlashIsLiteralWhereTextIsTyped(t *testing.T) {
	t.Run("channel finder", func(t *testing.T) {
		a := newTestApp(t, append(channelFinderOpts(), withMode(ModeChannelFinder))...)
		a.Update(keyPress('/'))
		if a.mode != ModeChannelFinder || a.channelFinder.Query() != "/" {
			t.Errorf("mode=%v query=%q, want the finder to hold \"/\"", a.mode, a.channelFinder.Query())
		}
	})
	t.Run("workspace search", func(t *testing.T) {
		a := newTestApp(t, wsSearchOpts()...)
		a.Update(keyMod('f', tea.ModCtrl))
		typeIntoApp(a, "in:/x")
		a.Update(keyCode(tea.KeyEnter))
		if a.mode != ModeWorkspaceSearch || a.searchResults.Query() != "in:/x" {
			t.Errorf("mode=%v query=%q", a.mode, a.searchResults.Query())
		}
	})
	// The other type-to-filter dialogs: `/` lands in their filter line
	// (drawn by chrome.FilterInput as query + block cursor) and never
	// opens the search prompt.
	for _, tc := range []struct {
		name string
		open tea.KeyMsg
		mode Mode
	}{
		{"theme switcher", keyMod('y', tea.ModCtrl), ModeThemeSwitcher},
		{"presence menu", keyMod('s', tea.ModCtrl), ModePresenceMenu},
		{"reaction picker", keyPress('r'), ModeReactionPicker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, append(normalOpts(), withActiveTeam("T1"))...)
			focusMessages(t, a)
			a.Update(tc.open)
			if a.mode != tc.mode {
				t.Fatalf("precondition: mode = %v, want %v", a.mode, tc.mode)
			}
			a.Update(keyPress('/'))
			if a.mode != tc.mode || a.statusbar.Search() != "" {
				t.Fatalf("mode=%v segment=%q", a.mode, a.statusbar.Search())
			}
			if screen := a.View().Content; !strings.Contains(screen, "/█") {
				t.Errorf("filter line does not hold \"/\":\n%s", ansi.Strip(screen))
			}
		})
	}
}

// ---------------------------------------------------------------------
// The dialogs' `/` filter and the mouse and Delete keys
// ---------------------------------------------------------------------

// Regression: a click outside a dialog sends Esc, which a `/` filter
// spent on itself, so the dialog stayed open. (The link picker and the
// reactions list have no click target, so any click dismisses them.)
func TestClickOutsideClosesAFilteredDialog(t *testing.T) {
	clickAway := tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0}
	cases := []struct {
		name    string
		open    func(t *testing.T) *App
		visible func(a *App) bool
	}{
		{"marks, filter kept", func(t *testing.T) *App {
			a, _, _ := marksClickApp(t)
			typeIntoApp(a, "/b")
			a.Update(keyCode(tea.KeyEnter))
			return a
		}, func(a *App) bool { return a.marksOverlay.IsVisible() }},
		{"marks, filter typing", func(t *testing.T) *App {
			a, _, _ := marksClickApp(t)
			typeIntoApp(a, "/b")
			return a
		}, func(a *App) bool { return a.marksOverlay.IsVisible() }},
		{"help, filter kept", func(t *testing.T) *App {
			a := newTestApp(t, normalOpts()...)
			typeIntoApp(a, "?/thread")
			a.Update(keyCode(tea.KeyEnter))
			return a
		}, func(a *App) bool { return a.help.IsVisible() }},
		{"link picker, filter typing", func(t *testing.T) *App {
			a := newTestApp(t, normalOpts()...)
			openLinkPicker(t, a)
			typeIntoApp(a, "/b")
			return a
		}, func(a *App) bool { return a.linkPicker.IsVisible() }},
		{"reactions, filter typing", func(t *testing.T) *App {
			a := newTestApp(t, normalOpts()...)
			openReactionsViewFor(3)(t, a)
			typeIntoApp(a, "/z")
			return a
		}, func(a *App) bool { return a.reactionsView.IsVisible() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := c.open(t)
			if !a.mode.IsModalOverlay() || !c.visible(a) {
				t.Fatalf("precondition: mode=%v visible=%v", a.mode, c.visible(a))
			}
			a.Update(clickAway)
			if a.mode != ModeNormal || c.visible(a) {
				t.Errorf("after the click: mode=%v visible=%v, want closed", a.mode, c.visible(a))
			}
		})
	}
}

// Regression: a click on a marks row while the filter was typed ended
// the typing (the synthesised Enter went to the filter) instead of
// jumping.
func TestMarksRowClickWhileTypingJumps(t *testing.T) {
	a, _, _ := marksClickApp(t)
	typeIntoApp(a, "/b")
	// The filter line moved the list down and grew the box.
	w, h := a.marksOverlay.BoxSize(a.width, a.height)
	x, y := max((a.width-w)/2, 0), max((a.height-h)/2, 0)
	firstRow := chrome.BodyTop(messages.ChromeStyles(), true)
	_, cmd := a.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: y + firstRow})
	if cmd == nil {
		t.Fatal("clicking the filtered row did not jump")
	}
	if cs, ok := cmd().(ChannelSelectedMsg); !ok || cs.ID != "C3" {
		t.Fatalf("click produced %#v, want a jump to mark b (C3)", cmd())
	}
	if a.mode != ModeNormal {
		t.Errorf("mode = %v, want NORMAL after the jump", a.mode)
	}
}

// Regression: Delete while typing the marks filter fell through to the
// overlay and removed the highlighted mark from the store.
func TestMarksDeleteWhileTypingKeepsTheMark(t *testing.T) {
	a, _, _ := marksClickApp(t)
	typeIntoApp(a, "/b")
	a.Update(keyCode(tea.KeyDelete))
	if got := len(mustListMarks(t, a.marks, "T1")); got != 2 {
		t.Fatalf("marks after Delete while typing = %d, want 2", got)
	}
	if a.mode != ModeMarks || !a.marksOverlay.Typing() {
		t.Errorf("mode=%v typing=%v, want the overlay still filtering", a.mode, a.marksOverlay.Typing())
	}
}
