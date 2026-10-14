// internal/ui/search_list.go
//
// The client-side half of the `/` search (reducer_search.go): the
// thread pane, the Threads view and the Activity view are searched over
// what they have loaded, through one interface and one matcher, so the
// three cannot drift apart.
package ui

import (
	"github.com/gammons/slk/internal/ui/activityview"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/thread"
	"github.com/gammons/slk/internal/ui/threadsview"
)

// searchableList is a pane `/` searches client-side. Rows are indexed
// in display order, top to bottom.
type searchableList interface {
	// SearchRows is the number of rows.
	SearchRows() int
	// SearchText is the plain text a query matches for row i: what
	// the row shows, mrkdwn flattened.
	SearchText(i int) string
	// SearchKey is a stable identity for row i, which survives the
	// pane re-sorting or refreshing its rows.
	SearchKey(i int) string
	// SearchCursor is the display index of the selected row.
	SearchCursor() int
	// SelectSearchKey selects the row with that key, as j/k would,
	// and reports whether it is there.
	SelectSearchKey(key string) bool
}

var (
	_ searchableList = (*thread.Model)(nil)
	_ searchableList = (*threadsview.Model)(nil)
	_ searchableList = (*activityview.Model)(nil)
)

// clientSearchMatches returns the keys of the rows of l whose text
// matches every term (messages.MatchesSearchTerms), newest first — the
// order the channel FTS returns, so n steps back through history in
// every scope — and the index of the first match at or older than the
// cursor (0 when none is). newestAtTop says which end of l is newest.
func clientSearchMatches(l searchableList, terms []string, newestAtTop bool) (keys []string, start int) {
	n := l.SearchRows()
	cursor := l.SearchCursor()
	start = -1
	for k := range n {
		i := k
		if !newestAtTop {
			i = n - 1 - k
		}
		if !messages.MatchesSearchTerms(l.SearchText(i), terms) {
			continue
		}
		older := i >= cursor
		if !newestAtTop {
			older = i <= cursor
		}
		if start < 0 && older {
			start = len(keys)
		}
		keys = append(keys, l.SearchKey(i))
	}
	if start < 0 {
		start = 0
	}
	return keys, start
}
