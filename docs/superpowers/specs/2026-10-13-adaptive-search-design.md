# Focus-adaptive `/` search

**Date:** 2026-10-13
**Status:** Proposed
**Related:** `docs/superpowers/specs/2026-06-10-search-design.md` (the
in-channel FTS `/` this extends), `docs/superpowers/specs/2026-10-12-phase4-modal-chrome-design.md`
(the `bubbles/chrome` substrate the dialog filter draws through)

## Problem

`/` always means "search the active channel's cached history", whatever
has focus. The keybinding is vim's "search what I am looking at", but slk
only honours that in one pane:

- With the **sidebar** focused (the App's default focus), `/` opens the
  channel FTS prompt; the matches select messages in a pane that does not
  have focus. The sidebar has a working name filter
  (`sidebar.Model.SetFilter` → `rebuildFilter`, folded via `text.Fold`)
  that nothing outside tests calls.
- With the **thread pane** focused, `/` and `n`/`N` are explicit no-ops
  (`mode_normal.go`, "v1" per the 2026-06-10 spec's non-goals).
- In the **Threads view** and the **Activity view** the messages slot shows
  a list (`threadsview` / `activityview`), but `/` still runs the FTS over
  `a.activeChannelID`: it silently searches a channel that is hidden behind
  the list, and `n`/`N` move the selection in that hidden pane.
- **Dialogs**: the help overlay already filters on `/` (its own
  `searching`/`query` state in `help.Model`); the marks list, the link
  picker and the reactions list show navigable lists with no way to
  search them.

## Current behavior (code references)

| Context | What `/` does today |
|---|---|
| `PanelMessages`, `ViewChannels` | `ModeSearch` prompt (`mode_search.go`, status segment `/…`); Enter → `SearchService.SearchChannel(activeChannelID)` → `ChannelSearchResultsMsg` (`reducer_search.go`, `searchGen` drops stale results); `n`/`N` → `searchStep`; Esc in Normal → `clearActiveSearch` |
| `PanelSidebar` | same FTS prompt for the active channel |
| `PanelThread` | no-op; `n`/`N` no-op |
| `PanelMessages`, `ViewThreads` / `ViewActivity` | FTS over the hidden channel |
| `PanelWorkspace` | unreachable: `FocusNext`/`FocusPrev` never focus the rail; `PanelWorkspace` is only `panelAt`'s hit-test sentinel |
| `ModeHelp` | `help.Model.HandleKey("/")` enters its search; Enter keeps the filter, Esc in search clears it; Esc/q/? otherwise close |
| `ModeMarks`, `ModeLinkPicker`, `ModeReactionsView` | ignored |
| `ModeChannelFinder`, `ModeNewMessage`, `ModeReactionPicker`, `ModeWorkspaceFinder`, `ModeThemeSwitcher`, `ModePresenceMenu`, `ModeWorkspaceSearch` | typed into the dialog's own filter/query |
| `ModeInsert`, `ModeCommand` | typed into the composer / `:` prompt |
| `ModePresenceCustomSnooze` | dropped (digits only, `presence.AppendSnoozeDigit`) |
| `ModeConfirm`, `ModeUserProfile` | ignored |

## Design

`/` resolves a **scope** from focus (and the view the messages slot
shows), opens the same status-line prompt for it, and the scope decides
what the query does. One search state, generalized; no second
implementation.

### Scopes

```go
type searchScope int // internal/ui/reducer_search.go

const (
	searchChannel      searchScope = iota // messages pane, ViewChannels: FTS (unchanged)
	searchThread                          // thread pane: loaded parent + replies, client-side
	searchThreadsList                     // messages slot in ViewThreads: the thread cards
	searchActivityList                    // messages slot in ViewActivity: the activity cards
	searchSidebar                         // sidebar: live conversation-name filter
)
```

`(*App).searchScopeForFocus()`:

| `focusedPanel` | view | scope |
|---|---|---|
| `PanelSidebar` | any | `searchSidebar` |
| `PanelThread` | any, thread visible and loaded | `searchThread` (else no-op) |
| `PanelMessages` | `ViewChannels` | `searchChannel` |
| `PanelMessages` | `ViewThreads` | `searchThreadsList` |
| `PanelMessages` | `ViewActivity` | `searchActivityList` |
| anything else (`PanelWorkspace`) | | no-op |

The zero value is `searchChannel`, so every existing caller of the
`ModeSearch` handler keeps its meaning.

### Status segment

The prompt and the indicator are prefixed with the scope, so the user
always sees what they are searching:

| scope | prompt | after Enter |
|---|---|---|
| channel | `/deploy` (unchanged) | `/deploy  3/17`, `/deploy  …`, `/deploy  no matches` (unchanged) |
| thread | `thread /deploy` | `thread /deploy  1/2`, `thread /deploy  no matches` |
| Threads view | `threads /deploy` | `threads /deploy  2/5` |
| Activity view | `activity /deploy` | `activity /deploy  1/3` |
| sidebar | `channels /eng` | segment restored (see below); the sidebar shows the filter itself |

`statusbar.maxSearchWidth` (40) still elides a long segment.

### Message searches: channel, thread, lists

All four share `activeSearch` (now carrying `scope` and a `target`
identity), `searchStep` (n/N with wrap and the "Search wrapped" toast),
the status indicator, Esc-clears, and the "one search at a time" rule:
installing a search of any scope replaces the previous one and clears
its highlights. `searchGen` is bumped by every install, so an FTS result
still in flight cannot overwrite a newer thread or list search.

**Channel** — unchanged: FTS dispatch, `ChannelSearchResultsMsg`, nearest
match at or older than the cursor, `FetchAround` for off-buffer matches.
Dispatching an FTS while a thread/list search is active drops that
search first.

**Thread and lists** run client-side, synchronously on Enter, over a
`searchableList`:

```go
// internal/ui/search_list.go
type searchableList interface {
	SearchRows() int                 // rows in display order
	SearchText(i int) string         // plain text a query matches for row i
	SearchKey(i int) string          // stable identity of row i
	SearchCursor() int               // display index of the selected row
	SelectSearchKey(key string) bool // select the row with that key
}

var (
	_ searchableList = (*thread.Model)(nil)
	_ searchableList = (*threadsview.Model)(nil)
	_ searchableList = (*activityview.Model)(nil)
)
```

- **Terms**: whitespace-split, each `text.Fold`ed — the same derivation
  the FTS path uses (`cmd/slk/main.go` `SearchChannel`).
- **Matching**: `messages.MatchesSearchTerms(text, terms)` (new, next to
  `HighlightSearchTerms` in `messages/highlight.go`, sharing its
  word-start rule and `prefixMatchLen`): every term must occur at a word
  start — the semantics of the FTS query `"foo"* "bar"*`, and exactly
  what the highlighter paints.
- **Order**: matches are stored newest first and `n` steps to the next
  older one in every scope, as the channel search does ("backward through
  history"). The thread pane lists oldest at the top, so `n` moves up;
  the Threads and Activity lists put the newest at the top, so `n` moves
  down. The first match is the nearest at or older than the cursor.
- **Thread text**: each message's `Text` flattened with
  `messages.FlattenMrkdwnWithUserGroups` (mentions resolved, as drawn).
  Row 0 is the parent. Keys are message TSes.
- **Thread highlight**: `thread.Model.SetSearchTerms` (mirror of
  `messages.Model.SetSearchTerms`) feeds `RenderContext.SearchTerms`, so
  `messages.HighlightSearchTerms` paints parent and replies exactly as in
  the channel pane. `SetSearchTerms` joins the `contentPane` interface
  (both panes implement it), and clearing a search clears both panes.
  `thread.Model` drops its terms itself when `SetThread` changes the
  thread identity, and on `Clear`.
- **Threads list text**: channel name, parent author, flattened parent
  text, last replier. Keys are `channelID/threadTS`. Selecting a match
  opens it in the thread pane exactly as `j`/`k` do
  (`openSelectedThreadCmd(true)`).
- **Activity list text**: the card's author (or DM name), channel name,
  the activity detail and the hydrated body, flattened. Keys are
  `ActivityItem.Key`.
- Cards are not highlighted (they are one-line truncated previews); the
  selection marks the match.
- The match list is a snapshot taken at Enter, like the FTS result. A
  key that disappears (a deleted reply, a list refresh) is skipped by
  selection; re-run `/` to refresh.

**Lifetime** — `activeSearch.target` names what the search belongs to:
`channelID/threadTS` for the thread, the view for the lists.
`(*App).dropStaleSearch()` clears a thread/list search whose target is
gone; it runs wherever the thread or view changes: after the three
`threadPanel.SetThread` call sites and in `CloseThread`, and in a new
`(*App).setView(v)` that replaces every `a.view = …` assignment. The
channel search keeps its existing lifetime (cleared on channel switch and
window focus swap). The global clears (channel switch, Esc, empty Enter)
clear any scope.

**`n`/`N` gating** (`(*App).searchStepsHere()`): a channel search steps
only in `ViewChannels` with the thread pane unfocused (today's rule plus
"never in a hidden pane"); a thread search only with the thread pane
focused; a list search only with the messages slot focused in its view.

### Sidebar: live name filter

- `/` opens the prompt **prefilled with the current filter**, so the
  prompt text is always exactly what is filtering. Each edit applies
  `sidebar.Model.SetFilter(strings.TrimSpace(input))` live.
- **Enter** keeps the filter and returns to Normal; the cursor is on the
  first matching conversation (where the last edit put it); `j`/`k` walk
  the filtered rows; Enter on a row opens it. An empty Enter clears the
  filter. The status segment goes back to whatever message search is
  active (or blank): the filter is shown in the sidebar.
- **Esc in the prompt** restores the filter and cursor that were in place
  when `/` was pressed (`sidebar.Model.SnapshotFilter` /
  `RestoreFilter`, an opaque value holding the filter and the cursor's
  identity).
- **Esc in Normal with the sidebar focused and a filter active** clears
  the filter before anything else (focus first: the focused pane's search
  state goes first); the cursor stays on the conversation it was on, in
  its full list (its section expands, as `SelectByID` does, when it is
  collapsed). With other focus, Esc keeps its meaning there; the filter
  stays visible in the sidebar until cleared.
- The filter persists across opening a channel (focus moves to the
  messages pane, as always) so the user can come back and open the next
  match. It is cleared by a workspace switch: it was typed against the
  other workspace's list.

Sidebar model semantics while a filter is non-empty:

| Aspect | Behavior |
|---|---|
| Matching | folded substring of `ChannelItem.Name` (existing `rebuildFilter`) |
| Cursor on change | first matching conversation row; the Threads row when nothing matches |
| Cursor on clear | the same conversation/header/synthetic row, else Threads |
| Collapsed sections | drawn expanded so matches inside them are visible and navigable; the persisted collapse state (`IsCollapsed`, `collapseByID`/`collapsed`) is untouched and returns when the filter clears; the header glyph shows `▾` |
| Space / Enter on a header | no-op (`ToggleCollapseSelected` returns false): toggling an invisible state would surprise |
| Sections without matches | hidden, including Slack-mode `standard` sections that are otherwise always drawn |
| Staleness filter | bypassed: the filter searches every conversation the sidebar holds, including ones auto-hidden as stale — finding one you forgot is the point |
| Threads / Activity rows | stay pinned at the top: they are destinations, not conversations, and `rebuildNavWithCursor` falls back to row 0 |
| Indicator | the blank separator row under Activity reads ` /eng`, or ` /eng  no matches`, truncated to the width. Empty filter: the row is blank as today (goldens unchanged) |
| `a` / `A` | walk unread conversations among the matches (`NextUnread` already walks `m.filtered`; pinned by `TestNextUnread_RespectsActiveFilter`). With a filter active and nothing found the toast reads "No other unread channels match the filter" |
| `SelectByID` | finds matches (sections are expanded); a non-match is a no-op and never touches collapse state |

### Workspace rail

The rail never takes keyboard focus (`FocusNext`/`FocusPrev` skip it;
clicks switch workspaces directly), so there is no rail scope. If focus
were ever `PanelWorkspace`, `/` is a no-op rather than a search of a pane
that does not have focus. `:ws` remains the workspace finder.

### Dialogs: a shared `/` filter

New substrate `internal/bubbles/listfilter`: the `/` filter for a list
dialog that does not filter as you type. It holds the query and whether
the user is typing it, decides which keys it consumes, matches
(`text.Fold` substring over the fields a dialog passes) and draws the
dialog's `chrome.Frame.Input` line through `chrome.FilterInput`. A dialog
keeps its own rows, selection and keys and filters its rows through it
(RFC #236: extract the substrate, not the widget).

| State | Key | Effect |
|---|---|---|
| idle | `/` | start typing (consumed) |
| typing | printable rune (incl. `/`, `q`, `j`, `k`, mark letters), `space` | appended |
| typing | `backspace` | deletes the last rune |
| typing | `enter` | stop typing, keep the filter |
| typing | `esc` | stop typing, clear the filter |
| typing | `up`/`down`/`ctrl+n`/`ctrl+p` | not consumed: the dialog moves its selection through the filtered rows |
| filter kept | `esc` | clear the filter (consumed); the next `esc` is the dialog's own close |
| any | other keys | not consumed |

Input line: typing → `chrome.FilterInput(query, placeholder)`; filter
kept → `filter: <query>` in `Styles.Empty` (the help overlay's existing
look); idle → the dialog's idle hint, or no line. Geometry
(`BoxSize`, `ClickRow` via `chrome.BodyTop(s, hasInput)`) follows whether
the line is drawn.

Survey of every mode in `modeHandlers`:

| Mode | `/` after this change |
|---|---|
| `ModeHelp` | filter via `listfilter` (help migrates onto it; matches key label and description; idle hint "Press / to search" kept) |
| `ModeMarks` | filter via `listfilter` over letter, channel, author, excerpt. While typing, letters and `'`/`` ` `` are filter text instead of jumps/back-jump; after Enter they jump again. Enter selects and backspace deletes the highlighted *filtered* row; the filter survives a delete's refresh. No idle line (box unchanged until `/`) |
| `ModeLinkPicker` | filter via `listfilter` over label, URL, detail; `q` types while typing; Enter on a kept filter selects the highlighted filtered row (its original `Index` still maps to the source data). Footer gains `/ filter` |
| `ModeReactionsView` | filter via `listfilter`: a group whose emoji name matches keeps all its users, otherwise only matching users are kept, empty groups drop; scroll resets on change. Footer gains `/ filter` |
| `ModeChannelFinder`, `ModeNewMessage`, `ModeReactionPicker`, `ModeWorkspaceFinder`, `ModeThemeSwitcher`, `ModePresenceMenu`, `ModeWorkspaceSearch` | unchanged: `/` is a literal character in their type-to-filter input |
| `ModeInsert`, `ModeCommand` | unchanged: literal text |
| `ModePresenceCustomSnooze` | unchanged: dropped (digits only) |
| `ModeSearch` | unchanged: literal in the query |
| `ModeConfirm`, `ModeUserProfile` | unchanged: no-op (out of scope, below) |
| `ModeNormal` | the scope table above |

Deliberate change for existing users of the help overlay: after Enter has
kept a filter, the first Esc clears the filter instead of closing the
overlay (the uniform rule); a second Esc closes. Typing now accepts any
printable rune (it accepted ASCII only) and backspace deletes a rune, not
a byte.

### Key table

| Key | Context | Action |
|---|---|---|
| `/` | Normal, messages pane (channel) | Search the channel's cached history (FTS) — unchanged |
| `/` | Normal, thread pane | Search the open thread's loaded replies |
| `/` | Normal, Threads / Activity view list | Search the list's cards |
| `/` | Normal, sidebar | Filter conversations by name as you type |
| `Enter` / `Esc` | `/` prompt | Run (keep the filter) / cancel (restore the filter) |
| `n` / `N` | Normal, the searched pane focused | Next older / newer match (wraps) |
| `Esc` | Normal, sidebar focused, filter active | Clear the filter |
| `Esc` | Normal, a search active | Clear the search |
| `/` | Help, marks, link picker, reactions | Filter the list; `Enter` keeps it, `Esc` clears it |

### Help and docs

- `KeyMap.SearchMode` help text becomes "search focused pane"; the help
  overlay's existing `/` entry is the discoverability for the adaptive
  behavior. Dialog footers that exist mention `/ filter`.
- `wiki/Keybindings.md`: the `/`, `n`/`N` and `Esc` rows describe the
  scopes; new rows for the sidebar filter and the dialog filter.
- `AGENTS.md` shared-code tables: `messages.MatchesSearchTerms`,
  `searchableList`, `bubbles/listfilter`, and the sidebar filter API.

## Edge cases

- **Prompt open when the target changes** (a WS-driven thread switch):
  `dropStaleSearch` clears the stale search but leaves the prompt being
  typed alone.
- **Empty query**: Enter clears the search (any scope) or the sidebar
  filter.
- **Paste in the prompt**: flattened to one line and appended (existing
  `reducer_io` path); in the sidebar scope it live-filters too.
- **Thread with no replies**: the parent alone is searched.
- **Ephemeral replies**: searched like any shown reply.
- **Thread search, then channel `/` dispatched**: the thread search and
  its highlights are dropped at dispatch; the FTS result installs as
  today.
- **Channel search active, focus moves to the thread or to a list**: `n`/`N`
  do nothing there (unchanged for the thread; new for lists); Esc still
  clears it.
- **Sidebar filter and a message search at once**: independent. The status
  segment shows the message search; the sidebar row shows the filter.
  Esc clears the focused pane's state first.
- **Sidebar filter with no matches**: Threads/Activity rows, the indicator
  with "no matches", nothing else; Enter on the Threads row opens Threads.
- **Sidebar hidden** (`ctrl+b`): focus leaves the sidebar, so `/` cannot
  target it; an active filter stays and shows again with the sidebar.

## Out of scope

- **Incremental search for message scopes.** Channel FTS executes on
  Enter by design (2026-06-10 spec); the thread and list scopes mirror it
  so all message searches behave alike. Only the sidebar filters live,
  because it filters rather than jumps.
- **Highlighting inside Threads/Activity cards** — the cards are truncated
  one-line previews; the selection marks the match.
- **Searching beyond what is loaded** in the thread pane (no thread
  backfill exists) and in the lists (they show what the last fetch
  returned).
- **`ModeUserProfile`** — a read-only card of a handful of labelled
  fields, no list to filter.
- **`ModeConfirm`** — a prompt with at most a few buttons; its keys belong
  to `bubbles/confirmprompt`'s KeyMap.
- **App modals** (Slack "views", `2026-10-12-app-modals-research.md`) —
  not implemented in slk yet; they would adopt `listfilter` if they grow
  lists.
- **Search history / prompt recall** — unchanged non-goal.

## Testing

- `internal/ui`, real `Update` chain (`newTestApp`, `updateAndRender`,
  `runKeyCases` for the key tables):
  - scope resolution for every focus/view; `/` with `PanelWorkspace` is a
    no-op;
  - sidebar: typing filters live (cursor on first match), Enter keeps it
    and returns to Normal, `j`/`k`/Enter open a match, Esc in the prompt
    restores filter and cursor, Esc in Normal clears and keeps the
    cursor's conversation, collapsed sections show their matches without
    changing `IsCollapsed`, prompt prefilled on re-entry, paste filters,
    workspace switch clears, `a` toast;
  - thread: Enter finds matches in parent and replies, the newest match
    at or older than the cursor is selected, `n`/`N` step and wrap, the
    rendered thread contains the highlight SGR, Esc clears highlights,
    no-match segment, closing/switching the thread drops it, `n` with the
    thread unfocused is inert;
  - Threads/Activity lists: matches select cards (Threads opens the
    thread), `n`/`N`, view switch drops the search, the hidden channel is
    never searched (no `SearchChannel` call);
  - channel FTS: the existing `reducer_search_test.go` /
    `mode_search_test.go` rows stay green; their fixture focuses the
    messages pane explicitly, because the default focus (sidebar) now
    filters the sidebar;
  - every dialog given `/`; `/` still typed literally in the type-to-filter
    dialogs and in compose / `:`.
- `internal/ui/sidebar`: the model semantics table above.
- `internal/ui/messages`: `MatchesSearchTerms` (word start, AND, fold,
  accents, empty terms).
- `internal/bubbles/listfilter`: the key table, matching, input line.
- Dialog packages: filtering, selection/delete/choose through the filter,
  geometry with the input line.
- Goldens: new dialog scenarios for the filter states; the help golden
  changes only by its `/` description. Existing frames are otherwise
  unchanged (an empty sidebar filter draws the blank row as before).
