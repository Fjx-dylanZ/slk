// internal/ui/reducer_search.go
//
// The focus-adaptive `/` search: its scopes, the one active search they
// share, the reducer for the channel FTS results, and the App-level
// helpers that drive match navigation (n/N with wrap) and the
// status-line `<scope> /query  i/N` segment. The prompt's keys are
// mode_search.go; the client-side scopes match through search_list.go.
// Design: docs/superpowers/specs/2026-10-13-adaptive-search-design.md.
//
// Message family:
//
//	ChannelSearchResultsMsg   - FTS match list for the active channel
//	WorkspaceSearchResultsMsg - server-side search.messages results
//	                            for the ctrl+f modal
//
// Stale results (the user switched channels while the query ran, or a
// newer search of any scope was installed) are dropped. An error clears
// search state and toasts; an empty result clears state and shows "no
// matches" in the status line. Otherwise the match list becomes the
// active search, highlights are pushed to the messages pane, and the
// selection jumps to the nearest match at or older than the cursor
// (FetchAround-ing a history window when the match is outside the
// loaded buffer).
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/text"
)

// searchScope is what `/` searches, resolved from focus when it is
// pressed (searchScopeForFocus). The zero value is the channel search.
type searchScope int

const (
	// searchChannel: the messages pane in ViewChannels. FTS over the
	// active channel's cached history, through the SearchService.
	searchChannel searchScope = iota
	// searchThread: the thread pane's loaded parent and replies,
	// client-side.
	searchThread
	// searchThreadsList: the cards of the Threads view, client-side.
	searchThreadsList
	// searchActivityList: the cards of the Activity view, client-side.
	searchActivityList
	// searchSidebar: a live filter over the sidebar's conversation
	// names. It filters rather than jumps, so it never becomes an
	// activeSearch; its state lives in sidebar.Model.
	searchSidebar
)

// prefix is the status-segment text in front of the query, naming the
// scope. The channel search keeps its bare "/".
func (s searchScope) prefix() string {
	switch s {
	case searchThread:
		return "thread /"
	case searchThreadsList:
		return "threads /"
	case searchActivityList:
		return "activity /"
	case searchSidebar:
		return "channels /"
	}
	return "/"
}

// newestAtTop reports whether the scope's pane lists its newest rows at
// the top: the Threads and Activity lists do, the thread pane (like the
// channel pane) puts them at the bottom. Matches are kept newest first
// either way, so n always steps back through history.
func (s searchScope) newestAtTop() bool {
	return s == searchThreadsList || s == searchActivityList
}

// activeSearch is the active `/` search of a message scope: the query,
// its folded terms (for highlighting), the match keys newest first (TSes
// for the panes, row keys for the lists), and the index of the
// currently-selected match. target names what a client-side search
// belongs to (searchTarget); a search whose target is gone is stale.
type activeSearch struct {
	scope   searchScope
	target  string
	query   string
	terms   []string
	matches []string
	idx     int
}

// indicator is the status segment of an installed search.
func (s *activeSearch) indicator() string {
	return fmt.Sprintf("%s%s  %d/%d", s.scope.prefix(), s.query, s.idx+1, len(s.matches))
}

// activeSearchIndicator is the status segment the active search shows
// once no prompt is open, "" without one.
func (a *App) activeSearchIndicator() string {
	if a.search == nil {
		return ""
	}
	return a.search.indicator()
}

// searchScopeForFocus resolves what `/` searches from the focused pane
// and the view the messages slot shows. ok is false where there is
// nothing to search: the workspace rail never takes keyboard focus, and
// a focused thread pane without a thread is empty.
func (a *App) searchScopeForFocus() (searchScope, bool) {
	switch a.focusedPanel {
	case PanelSidebar:
		return searchSidebar, true
	case PanelThread:
		if a.threadVisible && !a.threadPanel.IsEmpty() {
			return searchThread, true
		}
	case PanelMessages:
		switch a.view {
		case ViewThreads:
			return searchThreadsList, true
		case ViewActivity:
			return searchActivityList, true
		}
		return searchChannel, true
	}
	return 0, false
}

// openSearchPrompt opens the `/` prompt for the focused pane's scope.
// The sidebar's prompt starts with the filter already applied, so what
// the prompt shows is always exactly what is filtering, and remembers
// filter and cursor for Esc to restore.
func (a *App) openSearchPrompt() tea.Cmd {
	scope, ok := a.searchScopeForFocus()
	if !ok {
		return nil
	}
	a.searchScope = scope
	a.searchInput = ""
	if scope == searchSidebar {
		a.sidebarFilterBefore = a.sidebar.SnapshotFilter()
		a.searchInput = a.sidebar.Filter()
	}
	a.statusbar.SetSearch(scope.prefix() + a.searchInput)
	a.SetMode(ModeSearch)
	return nil
}

// searchPromptEdited redraws the prompt after its buffer changed and,
// for the sidebar, applies the buffer as the live filter.
func (a *App) searchPromptEdited() {
	a.statusbar.SetSearch(a.searchScope.prefix() + a.searchInput)
	if a.searchScope == searchSidebar {
		a.sidebar.SetFilter(strings.TrimSpace(a.searchInput))
	}
}

// cancelSearchPrompt is Esc in the prompt: the sidebar gets back the
// filter and cursor it had when `/` was pressed, and the segment goes
// back to the active search's indicator (or blank).
func (a *App) cancelSearchPrompt() {
	a.searchInput = ""
	if a.searchScope == searchSidebar {
		a.sidebar.RestoreFilter(a.sidebarFilterBefore)
	}
	a.statusbar.SetSearch(a.activeSearchIndicator())
	a.SetMode(ModeNormal)
}

// submitSearchPrompt is Enter in the prompt: it runs the query in the
// prompt's scope. An empty query clears: the active search, or the
// sidebar filter.
func (a *App) submitSearchPrompt() tea.Cmd {
	query := strings.TrimSpace(a.searchInput)
	a.searchInput = ""
	a.SetMode(ModeNormal)
	switch a.searchScope {
	case searchSidebar:
		a.sidebar.SetFilter(query)
		a.statusbar.SetSearch(a.activeSearchIndicator())
		return nil
	case searchChannel:
		if query == "" {
			a.clearActiveSearch()
			return nil
		}
		return a.dispatchChannelSearch(query)
	}
	if query == "" {
		a.clearActiveSearch()
		return nil
	}
	return a.runListSearch(a.searchScope, query)
}

// dispatchChannelSearch sends the FTS query for the active channel. A
// thread or list search is dropped first: one search at a time.
func (a *App) dispatchChannelSearch(query string) tea.Cmd {
	if a.search != nil && a.search.scope != searchChannel {
		a.dropSearchState()
	}
	a.statusbar.SetSearch(searchChannel.prefix() + query + "  …")
	// New dispatch supersedes any in-flight query; stamp the
	// result with this dispatch's generation so the reducer can
	// drop late results from older queries (or ones the user
	// cleared while pending). The Gen is attached here, UI-side,
	// so the SearchService stays unaware of UI bookkeeping.
	a.searchGen++
	gen := a.searchGen
	search := a.searchSvc
	chID := a.activeChannelID
	return func() tea.Msg {
		msg := search.SearchChannel(ids.ChannelID(chID), query)
		if r, ok := msg.(ChannelSearchResultsMsg); ok {
			r.Gen = gen
			return r
		}
		return msg
	}
}

// runListSearch runs query client-side over the thread pane or a list
// view, installs the matches as the active search (replacing any other)
// and selects the nearest at or older than the cursor.
func (a *App) runListSearch(scope searchScope, query string) tea.Cmd {
	l := a.searchableFor(scope)
	if l == nil {
		// The thread closed or the view changed while the prompt was
		// open: there is nothing left to search.
		a.statusbar.SetSearch(a.activeSearchIndicator())
		return nil
	}
	terms := foldedSearchTerms(query)
	// Bumps searchGen, so an FTS result still in flight cannot
	// replace this search when it lands.
	a.dropSearchState()
	matches, start := clientSearchMatches(l, terms, scope.newestAtTop())
	if len(matches) == 0 {
		a.statusbar.SetSearch(fmt.Sprintf("%s%s  no matches", scope.prefix(), query))
		return nil
	}
	a.search = &activeSearch{
		scope:   scope,
		target:  a.searchTarget(scope),
		query:   query,
		terms:   terms,
		matches: matches,
		idx:     start,
	}
	a.showSearchHighlights(scope, terms)
	return a.jumpToCurrentMatch()
}

// foldedSearchTerms splits a query into whitespace-separated terms,
// each folded (text.Fold) — the derivation the channel FTS path uses
// (cmd/slk SearchChannel), so the scopes highlight alike.
func foldedSearchTerms(query string) []string {
	fields := strings.Fields(query)
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		terms = append(terms, text.Fold(f))
	}
	return terms
}

// showSearchHighlights paints terms in the content pane scope searches
// and clears them from the other one: one search at a time. List scopes
// highlight nothing, so they clear both.
func (a *App) showSearchHighlights(scope searchScope, terms []string) {
	panes := []struct {
		pane  contentPane
		scope searchScope
	}{
		{a.messagepane, searchChannel},
		{a.threadPanel, searchThread},
	}
	for _, p := range panes {
		if p.scope == scope {
			p.pane.SetSearchTerms(terms)
		} else {
			p.pane.SetSearchTerms(nil)
		}
	}
}

// searchTarget names what a client-side search of scope belongs to
// right now: the open thread, or the list view on screen. "" when there
// is none. The channel search has no target; it keeps its own lifetime
// (cleared on channel switch and window focus swap).
func (a *App) searchTarget(scope searchScope) string {
	switch scope {
	case searchThread:
		if a.threadVisible && !a.threadPanel.IsEmpty() {
			return a.threadPanel.ChannelID() + "/" + a.threadPanel.ThreadTS()
		}
	case searchThreadsList:
		if a.view == ViewThreads {
			return "threads"
		}
	case searchActivityList:
		if a.view == ViewActivity {
			return "activity"
		}
	}
	return ""
}

// searchableFor returns the pane a client-side scope searches, nil when
// its target is gone.
func (a *App) searchableFor(scope searchScope) searchableList {
	if a.searchTarget(scope) == "" {
		return nil
	}
	switch scope {
	case searchThread:
		return a.threadPanel
	case searchThreadsList:
		return &a.threadsView
	case searchActivityList:
		return &a.activityView
	}
	return nil
}

// dropStaleSearch clears a thread or list search whose target is gone
// (the thread closed or changed, the view switched). It runs wherever
// those change: after threadPanel.SetThread, in CloseThread and in
// setView. An open prompt keeps what the user is typing.
func (a *App) dropStaleSearch() {
	s := a.search
	if s == nil || s.scope == searchChannel || s.target == a.searchTarget(s.scope) {
		return
	}
	a.dropSearchState()
	if a.mode != ModeSearch {
		a.statusbar.SetSearch("")
	}
}

// searchStepsHere reports whether n/N step the active search with the
// current focus: only where its matches are drawn. The channel search
// steps from any pane but the thread (as before) and never while a list
// view hides the channel pane.
func (a *App) searchStepsHere() bool {
	s := a.search
	if s == nil {
		return false
	}
	switch s.scope {
	case searchThread:
		return a.focusedPanel == PanelThread
	case searchThreadsList:
		return a.focusedPanel == PanelMessages && a.view == ViewThreads
	case searchActivityList:
		return a.focusedPanel == PanelMessages && a.view == ViewActivity
	}
	return a.view == ViewChannels && a.focusedPanel != PanelThread
}

var reduceSearch reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case ChannelSearchResultsMsg:
		return reduceChannelSearchResults(a, m), true
	case WorkspaceSearchResultsMsg:
		if !a.searchResults.IsVisible() || a.searchResults.Query() != m.Query {
			return nil, true // stale: modal closed or query changed
		}
		if m.Err != nil {
			a.searchResults.SetError("Search failed: " + m.Err.Error())
			return nil, true
		}
		a.searchResults.SetHighlightTerms(workspaceHighlightTerms(m.Query))
		a.searchResults.SetResults(m.Items, m.Total)
		return nil, true
	}
	return nil, false
}

// workspaceHighlightTerms derives the highlightable terms of a
// workspace search query for the ctrl+f results modal: whitespace-split
// tokens minus Slack search modifiers (any token containing ':', e.g.
// from:@user, in:#general, before:2026-01-01), each folded (text.Fold)
// for case- and accent-insensitive matching. Surrounding quote runes
// are stripped so a quoted phrase highlights its individual words
// (`"foo bar"` → foo, bar); tokens that strip to nothing are dropped.
// An empty result disables highlighting.
func workspaceHighlightTerms(query string) []string {
	var terms []string
	for _, tok := range strings.Fields(query) {
		if strings.Contains(tok, ":") {
			continue
		}
		tok = strings.Trim(tok, `"`)
		if tok == "" {
			continue
		}
		terms = append(terms, text.Fold(tok))
	}
	return terms
}

// reduceChannelSearchResults applies an in-channel `/` FTS result:
// stale/superseded drop, error toast, no-match status, or install the
// match list and jump to the nearest match.
func reduceChannelSearchResults(a *App, m ChannelSearchResultsMsg) tea.Cmd {
	if m.ChannelID != a.activeChannelID {
		return nil // stale: channel changed since query
	}
	if m.Gen != a.searchGen {
		// Superseded: the user cleared the search, dispatched a
		// newer query, or installed a thread/list search while this
		// one was in flight.
		return nil
	}
	if m.Err != nil {
		debuglog.General("ChannelSearchResultsMsg: channel=%s query=%q err=%v",
			m.ChannelID, m.Query, m.Err)
		a.clearActiveSearch()
		return func() tea.Msg { return ToastMsg{Text: "Search failed"} }
	}
	if len(m.TSes) == 0 {
		a.clearActiveSearch()
		a.statusbar.SetSearch(fmt.Sprintf("%s%s  no matches", searchChannel.prefix(), m.Query))
		return nil
	}

	a.search = &activeSearch{scope: searchChannel, query: m.Query, terms: m.Terms, matches: m.TSes}
	a.showSearchHighlights(searchChannel, m.Terms)

	// Nearest match at or older than the cursor (matches are newest
	// first; slack ts strings compare lexicographically).
	cursorTS := ""
	if sel, ok := a.messagepane.SelectedMessage(); ok {
		cursorTS = sel.TS
	}
	a.search.idx = 0
	for i, ts := range m.TSes {
		if ts <= cursorTS {
			a.search.idx = i
			break
		}
	}
	return a.jumpToCurrentMatch()
}

// jumpToCurrentMatch selects the match at a.search.idx and refreshes
// the status-line search segment. The channel search fetches a history
// window when the match is outside the loaded buffer; in the Threads
// view the selected card's thread opens on the right, as j/k open it.
func (a *App) jumpToCurrentMatch() tea.Cmd {
	s := a.search
	if s == nil || len(s.matches) == 0 {
		return nil
	}
	a.statusbar.SetSearch(s.indicator())
	key := s.matches[s.idx]
	if s.scope != searchChannel {
		// A key that left the pane since the search ran (a deleted
		// reply, a list refresh) selects nothing; the indicator still
		// moves, and the next n goes on from it.
		l := a.searchableFor(s.scope)
		if l != nil && l.SelectSearchKey(key) && s.scope == searchThreadsList {
			return a.openSelectedThreadCmd(true)
		}
		return nil
	}
	if a.messagepane.SelectByTS(key) {
		return nil
	}
	channels := a.channels
	chID := a.activeChannelID
	return func() tea.Msg {
		return channels.FetchAround(ids.ChannelID(chID), ids.MessageTS(key))
	}
}

// searchStep moves the match selection by delta with wrap: +1 is the
// next-older match (n, wrapping to newest), -1 the next-newer (N,
// wrapping to oldest). No-op without an active search; a stale thread
// or list search is cleared instead; toasts on wrap.
func (a *App) searchStep(delta int) tea.Cmd {
	s := a.search
	if s == nil || len(s.matches) == 0 {
		return nil
	}
	if s.scope != searchChannel && s.target != a.searchTarget(s.scope) {
		a.clearActiveSearch()
		return nil
	}
	var wrapped bool
	s.idx += delta
	if s.idx >= len(s.matches) {
		s.idx, wrapped = 0, true
	} else if s.idx < 0 {
		s.idx, wrapped = len(s.matches)-1, true
	}
	cmd := a.jumpToCurrentMatch()
	if wrapped {
		return tea.Batch(cmd, func() tea.Msg { return ToastMsg{Text: "Search wrapped"} })
	}
	return cmd
}
