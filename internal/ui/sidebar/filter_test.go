package sidebar

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/core"
)

// filterModel is a config-mode sidebar with a human DM (Direct Messages,
// expanded by default) and two channels in the Channels section, which
// starts collapsed. Nav without a filter: Threads, Activity, the Direct
// Messages header, D1, the Channels header.
func filterModel() Model {
	return New([]ChannelItem{
		{ID: "D1", Name: "bob", Type: "dm"},
		{ID: "C1", Name: "eng-ops", Type: "channel"},
		{ID: "C2", Name: "general", Type: "channel"},
	})
}

// viewLines renders the sidebar and returns its ANSI-stripped lines.
func viewLines(m *Model, height, width int) []string {
	return strings.Split(ansi.Strip(m.View(height, width)), "\n")
}

func TestFilter_MatchesFoldedName(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "Café-Ops", Type: "channel"},
		{ID: "C2", Name: "general", Type: "channel"},
	})
	m.SetFilter("CAFE")
	if got := m.Filter(); got != "CAFE" {
		t.Fatalf("Filter() = %q, want CAFE", got)
	}
	visible := m.VisibleItems()
	if len(visible) != 1 || visible[0].ID != "C1" {
		t.Fatalf("visible = %+v, want only C1 (case- and accent-folded match)", visible)
	}
}

func TestFilter_CursorOnFirstMatchOrThreads(t *testing.T) {
	m := filterModel()
	m.SetFilter("eng")
	if got := m.SelectedID(); got != "C1" {
		t.Fatalf("filter eng: selected %q, want the first match C1", got)
	}
	// Each change re-targets the first match, even from a moved cursor.
	m.SelectThreadsRow()
	m.SetFilter("e")
	if got := m.SelectedID(); got != "C1" {
		t.Fatalf("filter e: selected %q, want the first match C1 in nav order", got)
	}
	m.SetFilter("o")
	if got := m.SelectedID(); got != "D1" {
		t.Fatalf("filter o: selected %q, want D1 (Direct Messages precede Channels)", got)
	}
	m.SetFilter("zzz")
	if !m.IsThreadsSelected() {
		t.Fatalf("no match: want the Threads row selected, got %q", m.SelectedID())
	}
	if n := len(m.VisibleItems()); n != 0 {
		t.Fatalf("no match: %d visible items, want 0", n)
	}
}

// Setting the filter it already has must not move a cursor the user
// moved after the last edit.
func TestFilter_SameValueKeepsCursor(t *testing.T) {
	m := filterModel()
	m.SetFilter("eng")
	m.MoveUp() // the Channels header
	v := m.Version()
	m.SetFilter("eng")
	if name, ok := m.IsSectionHeaderSelected(); !ok || name != defaultChannelsSection {
		t.Fatalf("same filter moved the cursor: header=%q ok=%v selected=%q", name, ok, m.SelectedID())
	}
	if m.Version() != v {
		t.Errorf("same filter bumped the version %d -> %d", v, m.Version())
	}
}

func TestFilter_ClearKeepsSelectedRow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter string
		move   func(m *Model)
		check  func(t *testing.T, m *Model)
	}{
		{
			name:   "conversation in an expanded section",
			filter: "bob",
			check: func(t *testing.T, m *Model) {
				if got := m.SelectedID(); got != "D1" {
					t.Errorf("selected %q, want D1", got)
				}
				if !m.IsCollapsed(defaultChannelsSection) {
					t.Errorf("an unrelated collapsed section was expanded")
				}
			},
		},
		{
			name:   "conversation in a collapsed section expands it",
			filter: "eng",
			check: func(t *testing.T, m *Model) {
				if got := m.SelectedID(); got != "C1" {
					t.Errorf("selected %q, want C1", got)
				}
				if m.IsCollapsed(defaultChannelsSection) {
					t.Errorf("Channels still collapsed; want it expanded as SelectByID does")
				}
			},
		},
		{
			name:   "section header",
			filter: "eng",
			move:   func(m *Model) { m.MoveUp() },
			check: func(t *testing.T, m *Model) {
				if name, ok := m.IsSectionHeaderSelected(); !ok || name != defaultChannelsSection {
					t.Errorf("header=%q ok=%v, want the Channels header", name, ok)
				}
				if !m.IsCollapsed(defaultChannelsSection) {
					t.Errorf("Channels expanded; only a selected conversation expands its section")
				}
			},
		},
		{
			name:   "Activity row",
			filter: "eng",
			move:   func(m *Model) { m.SelectActivityRow() },
			check: func(t *testing.T, m *Model) {
				if !m.IsActivitySelected() {
					t.Errorf("want Activity selected, got %q", m.SelectedID())
				}
			},
		},
		{
			name:   "Threads row (no matches)",
			filter: "zzz",
			check: func(t *testing.T, m *Model) {
				if !m.IsThreadsSelected() {
					t.Errorf("want Threads selected, got %q", m.SelectedID())
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := filterModel()
			m.SetFilter(tc.filter)
			if tc.move != nil {
				tc.move(&m)
			}
			m.SetFilter("")
			if m.Filter() != "" || len(m.VisibleItems()) != 3 {
				t.Fatalf("clear: filter %q, %d visible, want empty and 3", m.Filter(), len(m.VisibleItems()))
			}
			tc.check(t, &m)
		})
	}
}

func TestFilter_SnapshotRestore(t *testing.T) {
	m := filterModel()
	m.SelectByID("D1")
	before := m.SnapshotFilter()
	m.SetFilter("e")
	m.SetFilter("en")
	m.RestoreFilter(before)
	if m.Filter() != "" || m.SelectedID() != "D1" {
		t.Fatalf("restore unfiltered: filter %q selected %q, want \"\" and D1", m.Filter(), m.SelectedID())
	}
	if !m.IsCollapsed(defaultChannelsSection) {
		t.Errorf("restore expanded Channels; it was collapsed when snapshotted")
	}

	m.SetFilter("eng")
	m.MoveUp() // the Channels header
	filtered := m.SnapshotFilter()
	m.SetFilter("bob")
	m.RestoreFilter(filtered)
	if m.Filter() != "eng" {
		t.Fatalf("restore filtered: filter %q, want eng", m.Filter())
	}
	if name, ok := m.IsSectionHeaderSelected(); !ok || name != defaultChannelsSection {
		t.Fatalf("restore filtered: header=%q ok=%v selected=%q, want the Channels header", name, ok, m.SelectedID())
	}
	if ids := visibleIDs(&m); !reflect.DeepEqual(ids, []string{"C1"}) {
		t.Fatalf("restore filtered: visible %v, want [C1]", ids)
	}
}

func visibleIDs(m *Model) []string {
	var out []string
	for _, it := range m.VisibleItems() {
		out = append(out, it.ID)
	}
	return out
}

// A collapsed section is drawn expanded while filtering (rows, ▾ glyph,
// no collapsed aggregates) without touching its persisted state, which
// returns when the filter clears.
func TestFilter_CollapsedSectionDrawnExpanded(t *testing.T) {
	m := filterModel()
	m.SetReadStateReader(func() map[string]core.ReadState {
		return map[string]core.ReadState{"C1": {HasUnread: true}}
	})
	m.SetFilter("eng")
	view := m.View(15, 30)
	if got := headerFor(t, view, defaultChannelsSection); got != "▾ Channels" {
		t.Errorf("filtered header = %q, want %q (expanded, no aggregate)", got, "▾ Channels")
	}
	if !strings.Contains(view, "eng-ops") {
		t.Errorf("match inside the collapsed section not drawn:\n%s", ansi.Strip(view))
	}
	if !m.IsCollapsed(defaultChannelsSection) {
		t.Errorf("IsCollapsed(Channels) = false while filtering; persisted state must be untouched")
	}

	m.SelectThreadsRow() // clear without a conversation selected
	m.SetFilter("")
	view = m.View(15, 30)
	if got := headerFor(t, view, defaultChannelsSection); got != "▸ Channels •1" {
		t.Errorf("cleared header = %q, want %q", got, "▸ Channels •1")
	}
	if strings.Contains(view, "eng-ops") {
		t.Errorf("collapsed section rows drawn after the filter cleared:\n%s", ansi.Strip(view))
	}
}

func TestFilter_ToggleCollapseSelectedIsNoop(t *testing.T) {
	m := filterModel()
	m.SetFilter("eng")
	m.MoveUp() // the Channels header
	nav := append([]navItem(nil), m.nav...)
	cursor := m.cursor
	if m.ToggleCollapseSelected() {
		t.Fatal("ToggleCollapseSelected reported a toggle while filtering")
	}
	if !m.IsCollapsed(defaultChannelsSection) {
		t.Error("collapse state changed while filtering")
	}
	if !reflect.DeepEqual(m.nav, nav) || m.cursor != cursor {
		t.Error("nav or cursor changed by a no-op toggle")
	}
}

func TestFilter_SelectByIDNonMatchIsNoop(t *testing.T) {
	m := filterModel()
	m.SetFilter("bob")
	m.SelectByID("C2") // collapsed section, not a match
	if got := m.SelectedID(); got != "D1" {
		t.Errorf("selected %q, want D1 unchanged", got)
	}
	if !m.IsCollapsed(defaultChannelsSection) {
		t.Error("SelectByID of a non-match expanded its section")
	}
	m.SetFilter("eng")
	m.SelectThreadsRow()
	m.SelectByID("C1") // a match: found without touching collapse state
	if got := m.SelectedID(); got != "C1" {
		t.Errorf("selected %q, want the match C1", got)
	}
	if !m.IsCollapsed(defaultChannelsSection) {
		t.Error("SelectByID of a match changed persisted collapse state")
	}
}

// In Slack mode standard sections are drawn even when empty, except
// while filtering: then only sections holding a match are drawn.
func TestFilter_SlackModeHidesSectionsWithoutMatches(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "eng", Type: "channel", Section: "S1"},
		{ID: "C2", Name: "general", Type: "channel"},
	})
	m.SetSectionsProvider(&fakeProvider{ready: true, sections: []SectionMeta{
		{ID: "S1", Name: "Pinned", Type: "standard"},
		{ID: "S2", Name: "Empty", Type: "standard"},
		{ID: "SC", Name: "Channels", Type: "channels"},
	}})
	m.ToggleCollapse("S1")
	if got := slackModeNavHeaders(&m); !reflect.DeepEqual(got, []string{"S1", "S2", "SC"}) {
		t.Fatalf("precondition: headers %v, want [S1 S2 SC]", got)
	}

	m.SetFilter("eng")
	if got := slackModeNavHeaders(&m); !reflect.DeepEqual(got, []string{"S1"}) {
		t.Errorf("filtered headers %v, want [S1]", got)
	}
	if got := m.SelectedID(); got != "C1" {
		t.Errorf("selected %q, want C1 inside the collapsed S1", got)
	}
	if !m.IsCollapsed("S1") || !m.collapseByID["S1"] {
		t.Error("Slack-mode collapse state changed while filtering")
	}

	m.SetFilter("zzz")
	if got := slackModeNavHeaders(&m); len(got) != 0 {
		t.Errorf("no-match headers %v, want none", got)
	}
	m.SetFilter("")
	if got := slackModeNavHeaders(&m); !reflect.DeepEqual(got, []string{"S1", "S2", "SC"}) {
		t.Errorf("cleared headers %v, want [S1 S2 SC]", got)
	}
}

// The filter searches conversations hidden as stale; clearing hides them
// again, and a cursor left on one falls back to the Threads row.
func TestFilter_BypassesStaleness(t *testing.T) {
	now := time.Unix(2000000000, 0)
	m := New([]ChannelItem{
		{ID: "D1", Name: "old", Type: "dm"},
		{ID: "D2", Name: "new", Type: "dm"},
	})
	m.SetReadStateReader(func() map[string]core.ReadState {
		return map[string]core.ReadState{
			"D1": {LastReadTS: fmt.Sprintf("%d.000000", now.Add(-48*time.Hour).Unix())},
			"D2": {LastReadTS: fmt.Sprintf("%d.000000", now.Unix())},
		}
	})
	m.SetNowFunc(func() time.Time { return now })
	m.SetStaleThreshold(24 * time.Hour)
	if ids := visibleIDs(&m); !reflect.DeepEqual(ids, []string{"D2"}) {
		t.Fatalf("precondition: visible %v, want [D2] (D1 stale)", ids)
	}

	m.SetFilter("old")
	if got := m.SelectedID(); got != "D1" {
		t.Fatalf("filter old: selected %q, want the stale match D1", got)
	}
	m.SetFilter("")
	if ids := visibleIDs(&m); !reflect.DeepEqual(ids, []string{"D2"}) {
		t.Errorf("cleared: visible %v, want [D2]", ids)
	}
	if !m.IsThreadsSelected() {
		t.Errorf("cleared on a hidden conversation: selected %q, want Threads", m.SelectedID())
	}
}

func TestFilter_IndicatorRow(t *testing.T) {
	m := filterModel()
	unfiltered := m.View(10, 30)
	if row := strings.Split(unfiltered, "\n")[2]; row != m.cacheFiller {
		t.Fatalf("no filter: row under Activity = %q, want the blank filler", row)
	}

	m.SetFilter("eng")
	lines := viewLines(&m, 10, 30)
	if !strings.Contains(lines[0], "Threads") || !strings.Contains(lines[1], "Activity") {
		t.Errorf("Threads/Activity rows not pinned at the top: %q", lines[:2])
	}
	if got := strings.TrimSpace(lines[2]); got != "/eng" {
		t.Errorf("match indicator = %q, want %q", got, "/eng")
	}

	m.SetFilter("zzz")
	lines = viewLines(&m, 10, 30)
	if !strings.Contains(lines[0], "Threads") || !strings.Contains(lines[1], "Activity") {
		t.Errorf("no match: Threads/Activity rows missing: %q", lines[:2])
	}
	if got := strings.TrimSpace(lines[2]); got != "/zzz  no matches" {
		t.Errorf("no-match indicator = %q, want %q", got, "/zzz  no matches")
	}
	if got := strings.TrimSpace(lines[3]); got != "" {
		t.Errorf("no match: row below the indicator = %q, want nothing else", got)
	}

	// Clearing (cursor back on Threads, as before) draws the very frame
	// the sidebar drew before any filter.
	m.SelectThreadsRow()
	m.SetFilter("")
	if got := m.View(10, 30); got != unfiltered {
		t.Errorf("cleared view differs from the unfiltered one:\n%s\nwant:\n%s", ansi.Strip(got), ansi.Strip(unfiltered))
	}
}

func TestFilter_IndicatorTruncatesToOneLine(t *testing.T) {
	m := filterModel()
	m.SetFilter("engineering-platform-team")
	const height, width = 8, 16
	view := m.View(height, width)
	rows := strings.Split(view, "\n")
	if len(rows) != height {
		t.Fatalf("view has %d lines, want %d (indicator wrapped?)", len(rows), height)
	}
	plain := ansi.Strip(rows[2])
	if !strings.Contains(plain, "/engin") || !strings.Contains(plain, "…") {
		t.Errorf("indicator = %q, want a truncated /engineering… with an ellipsis", plain)
	}
	if w := ansi.StringWidth(rows[2]); w != width-2 {
		t.Errorf("indicator width = %d, want %d like the other rows", w, width-2)
	}
}

func TestFilter_IndicatorNotClickable(t *testing.T) {
	m := filterModel()
	m.SetFilter("eng")
	m.View(10, 30)
	cursor := m.cursor
	if item, ok := m.ClickAt(2); ok || item.ID != "" {
		t.Fatalf("ClickAt(indicator) = %+v, %v; want zero, false", item, ok)
	}
	if m.cursor != cursor {
		t.Errorf("click on the indicator moved the cursor %d -> %d", cursor, m.cursor)
	}
}
