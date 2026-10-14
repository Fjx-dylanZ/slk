# Adaptive key hints — design

**Date:** 2026-10-14
**Status:** Proposed
**Builds on:** `2026-10-13-leader-keys-design.md` (keyseq, the which-key
status-bar hint, the leader layer, `[keys.leader_map]`, the configurable
sidebar keys), `2026-10-12-phase4-modal-chrome-design.md` (`bubbles/chrome`).

## Problem

1. **The which-key hint is one status-bar line.** While a sequence is
   pending, `statusbar.SetKeyHint` shows the keys typed and as many
   continuations as fit, then `…`. The default leader layer already
   overflows an 80-column bar, a nested `[keys.leader_map]` group only
   reads `f …`, and the hint has no room for what a key does beyond one
   word.
2. **Only the keyseq tables hint.** `gg`, `m` and `'` (with
   `show_jump_overlay = false`) set a fixed `g …` / `m…` / `'…` and say
   nothing about what may follow; `'` in particular does not show which
   marks exist.
3. **No way to ask "what can I press here?"** The help overlay (`?`)
   lists every binding, sorted by description, whatever pane has focus.
   Many of them do nothing where the user is (`S` outside a thread, `u`
   outside Activity, message ops in the sidebar).

## Current behavior (references)

- `bubbles/keyseq`: `Sequence` holds the armed map and the keys typed;
  `Press` consumes every key while pending (`esc` and unbound keys
  cancel); `Hint()` feeds `statusbar.SetKeyHint` from `pressKeySeq`
  (`internal/ui/keyseqs.go`). A user group's binding has the hint `…`.
- `gg`, `m`, `'`: the `pendingTop`, `pendingMark`, `pendingJumpMark`
  flags at the top of `handleNormalMode`; `SetMode` disarms all of them.
  With `show_jump_overlay` on (the default) `'` opens the marks list
  (`ModeMarks`) instead of arming.
- Reaction navigation (`R`) is a sub-state of the focused pane, not a
  prefix: it swallows keys until Esc and shows its own cursor.
- Overlays: every modal composites through `overlay.DimmedOverlay`
  (centered, dimmed backdrop, kitty placeholder cells blanked) and is
  listed in `overlayActive`, which withholds every sixel placement and
  disables the compositor memo.

## Design

### 1. keyseq additions

- `Binding.Group`: a binding with no `Next` whose action starts another
  pending state the host tracks itself (the `gg` chord). Menus show it
  as a group.
- `Map.Items()`: the map as menu rows, `Item{Keys []string, Desc, Group}`.
  Like `Hints`, a run of consecutive bindings with the same text is one
  item (`h/j/k/l → focus pane in direction`). An action's text is its
  `Desc` (falling back to `Hint`); a group's is its `Hint`, the label.
- `Hints()` writes a group as `key +label` (`w +windows`, `f +…`).
- `Sequence` keeps a stack of steps (map, key, label) instead of one map:
  `Back()` pops a step (the first one cancels), `Arm(m, title)` starts a
  sequence at a map the host opens itself (the context menu, §5),
  `Title()` is the title and keys typed, `Label()` the label of the
  binding that armed the current map, `Next()` the armed map.
- `Press` while pending: `backspace` goes back a step, like `esc`
  cancels. Both are the menu's own keys, so a leader-map sequence may
  not contain `backspace` (§8).

### 2. `internal/bubbles/keyhints`: the menu

A self-contained component (no `internal/ui` import, no I/O):

```go
type Row struct { Keys []string; Desc string; Group bool }
type Styles struct { Chrome chrome.Styles; Key, Arrow, Desc, Group lipgloss.Style }
func (m *Model) Open(title string, rows []Row) // sorts; keeps the page for the same title
func (m *Model) Close()
func (m Model) IsVisible() bool
func (m *Model) Page(delta, maxW, maxH int) bool // ctrl+d / ctrl+u
func (m Model) View(s Styles, maxW, maxH int) string // "" when it cannot fit
```

- **Rows** read `key → description`, keys drawn with glyphs for special
  keys (`␣` space, `⇥` tab, `⇤` shift+tab, `⏎` enter, `⌫` backspace,
  `⎋` esc, `⌦` delete; all narrow, no Nerd Font). Arrow keys stay words
  (`←` would read as the row's own arrow). A row's keys are joined by `/`.
- **Order** (which-key's): plain actions first, then groups (`+label`, in
  the accent style). Within each: single letters and digits, compared
  case-insensitively with lowercase first (`a A b B`); then other single
  characters (`/ ? ] '`); then named keys (`␣ ⏎ ⇥`, arrows, pgup);
  then chords with a modifier (`ctrl+…`, `alt+…`), by their base key.
- **Layout**: a column of up to `maxH - 2` rows; more rows than that
  become several balanced columns (filled top to bottom, so the sorted
  order reads down each column), as many as `maxW` allows. Descriptions
  are cut at 40 columns (`…`). When even the columns cannot hold every
  row, the menu pages: `ctrl+d` / `ctrl+u` turn the page, and the footer
  says so (`ctrl+d/u 1/3`). Columns first because a page hides rows; the
  page keys only belong to the menu while it pages (§6).
- **Frame**: `chrome.Panel` (§3) with the title in the top border and
  the footer `esc close  ⌫ back` in the bottom one.

### 3. `chrome.Panel`

`Panel(s, title, footer, content, width)`: `s.Box`'s border and colors
with one column of side padding and none above or below, the title set
into the top border (`╭─ \ w +windows ───╮`) in `s.Title`, the footer
into the bottom border in `s.Footer`, both cut to fit. The menu is the
first user; any floating panel can be.

### 4. Hint sources: one per prefix

The menu is a view of the pending state, not a mode. Each pending state
implements one interface:

```go
type keyHintSource interface {
	keyHints(a *App) (title string, rows []keyhints.Row)
}
```

`(*App).pendingHintSource()` returns the one armed, in
`handleNormalMode`'s order:

| Prefix | Source | Rows from |
|---|---|---|
| leader, `ctrl+w`, nested groups, the context menu (§5) | `seqHints` | `pendingSeq.Next().Items()`; title `Sequence.Title()` plus ` +label` |
| `g` | `topHints` | `KeyMap.Top` (`g → top`) |
| `m` | `markHints{}` | the workspace's marks (`marksRows`, the marks list's own rows: `a → #general  alice: lunch?`) |
| `'` / `` ` `` (`show_jump_overlay = false`) | `markHints{jump}` | the marks, plus `'/\`` → back-jump when one is recorded |

Every row comes from where its keys are defined, so the menu cannot
drift from dispatch. `gg`, `m`, `'` stay pending flags (leader-keys §Out
of scope still applies: `m`/`'` take any letter, `gg` re-handles an
unmatched key); `markHints` and `topHints` are their declarations of
what may follow. `m` with no marks and `'` with no marks and no
back-jump have no rows, so no menu: the status hint (`m…`) remains.

Reaction navigation is not a prefix and is out of scope (see below).

### 5. The context menu: `key_hints`

A new leader action, `key_hints`, opens the menu immediately with the
single keys that act in the current context. It arms the sequence with
a map built for the context (`Sequence.Arm`), so it navigates like any
other level: a listed key closes the menu and runs exactly as if typed
in Normal mode (`handleNormalMode`), so `\` from there starts the
leader and `g` arms `gg`; Esc and Backspace close; any other key closes
and does nothing else (the leader rule: a slip after a menu does not run
`D`).

**Default `\ ?`.** `?` is "what can I press" everywhere, and
`<leader>?` is LazyVim's "Buffer Keymaps (which-key)", the same menu.
It breaks the layer's "leader stands in for ctrl" mnemonic on purpose:
there is no ctrl chord to mirror. Explicit beats default: the wiki's
neovim example maps `help = ["?"]`, so there `key_hints` loses `?`; the
example maps it to `s k` (LazyVim's "search keymaps").

**Context** = the focused pane and the view: the sidebar (any view),
the channel pane, the Threads list, the Activity list, the thread pane.
Its title names it (`keys: messages`).

**The scope table** (`internal/ui/keyscopes.go`) is the only place that
says where a `KeyMap` binding acts: one row per Normal-mode binding,
`{binding, where, when, kind}`.

- `where` is a set of contexts; `when`, optional, the state the binding
  also needs (`n`/`N`: a search to step through; `q`/`ctrl+]`: a thread
  open; `u`: the Activity view, whatever has focus; `/`: the channel
  view; `i`: a composer on screen; `m`: something to name, so not the
  Activity view outside the thread; `[`/`]`: the sidebar shown).
- `kind`: `prefix` for the root sequence keys (leader, `ctrl+w`), which
  dispatch before panel-scoped keys; `chord` for `g`; `plain` otherwise.
  Prefixes and chords show as groups (`\ → +leader`, `ctrl+w →
  +windows`, `g → +top`).
- Esc has a row with no contexts: in the menu Esc closes it.
- The configurable sidebar actions (`keyActions`) keep their own scope
  (their panel); the menu takes them from there, with the keys
  `[keys] sidebar_open` / `sidebar_toggle_section` gave them.

Building the menu applies dispatch precedence: prefixes, then the
focused panel's `keyActions`, then the table; a key an earlier binding
took is dropped from later ones (`sidebar_open = ["l"]` leaves the
sidebar's `→ right` row with `right` only). The KeyMap is read through
`helpKeyMap`, so a key the leader displaced is gone; Esc and Backspace
are the menu's. A binding's visible keys are its help label's keys
(`k/up`, `ctrl+l`, not the hidden alias `ctrl+k`); the hidden ones still
dispatch.

**Lockstep.** `TestKeyScopesCoverTheKeyMap` fails when a `KeyMap` field
with keys (other than the Insert-only `ctrl+o` / `ctrl+e`) has neither a
scope row nor a `keyActions` row, or when a scope row names a binding
twice.

Scopes follow what each handler does today. Message operations act on
the focused content pane's selection; `beginForwardOfSelected` and
`currentLocation` already refuse the Threads/Activity lists (the channel
pane is hidden behind them), so the table lists message operations for
the channel and thread panes only.

### 6. Delay and navigation

- After every key in Normal mode, `syncKeyHints` follows the pending
  state: no source → the menu closes; the menu already open → it shows
  the new level at once (which-key: once open it stays open across
  levels and on Backspace); otherwise a `keyHintsTickMsg{seq}` is
  scheduled `key_hints_delay` later. Every key that leaves the menu
  closed bumps `seq`, so only the tick of the latest key can open it,
  and only if a source is still pending: a fast `\ w l` never shows it.
  `0` opens the menu with the key.
- `key_hints` opens at once and also with `key_hints = false` (an
  explicit request); once open, the menu follows the sequence.
- `SetMode` closes the menu with the pending flags it already resets.
- **Paging**: while the open menu pages, `ctrl+d` / `ctrl+u` turn its
  page before anything else sees them. Otherwise they are ordinary keys
  (in the context menu they still scroll the pane).
- Mouse: `chrome.Clickable` is a centered, modal-mode contract
  (`reduceModalClick` centers the box like `DimmedOverlay` and only runs
  in a modal mode). The menu is neither; clicking a row is out of scope.

**Default delay 300 ms**: LazyVim's `timeoutlen` (it lowered vim's
1000 ms "to quickly trigger which-key"), inside which-key's own
200–300 ms range. Long enough that a practised `\ w l` (keys ~100 ms
apart) never flashes a menu, short enough that hesitating brings it up.

**Status bar.** Unchanged: `SetKeyHint` still shows the pending keys and
continuations from the first key. It is what shows while the delay runs,
with `key_hints = false`, and when the terminal is too small for the
menu.

### 7. Rendering and images

`View` draws the menu after the compositor memo, so the memoized base
frame stays valid; the menu is a few spliced rows per frame. It is not
an `overlayActive` modal: no dimmed backdrop, no full-screen re-wrap,
and images elsewhere stay.

- **Placement**: bottom-right, one column from the right edge, its bottom
  border on the row above the status bar (never over it). It may use
  the whole width less two columns and the height less the status bar
  and one row.
- **Compositing**: `overlay.Place(background, box, x, y)` splices the
  box's rows into the background rows; everything outside the box is
  the background's bytes, escape sequences included
  (`ansi.Truncate` / `ansi.TruncateLeft`, the splice `DimmedOverlay` uses
  for kitty rows). Kitty images under the box lose those cells only:
  each placeholder cell carries its own row and column diacritics
  (`buildPlaceholderLines`), so the cells left of and right of the box
  still draw their part of the image.
- **Sixel** is painted over the text, so a placement under the box would
  cover the menu. `collectSixelPlacements` withholds the placements that
  overlap the menu's rectangle (the painter erases them) and keeps the
  others; they come back when the menu closes. A placement on the
  menu's rows whose cells it does not cover is repainted, because its
  frame guard (the whole rows) changed.

### 8. Configuration

```toml
[keys]
key_hints = true        # default true
key_hints_delay = 300   # milliseconds; 0 shows the menu at once
[keys.leader_groups]
f = "find"
s = "search"
```

- **`key_hints`** (default on): the point is discovery, and the delay
  keeps the menu away from anyone who types a sequence fluently. The
  behavior change for existing users: pausing 300 ms after `ctrl+w` (or
  the leader, `g`, `m`) now brings up the menu; `key_hints = false`
  restores the status-bar-only hint.
- **`key_hints_delay`**: an integer, milliseconds, `0`–`10000`; anything
  else is an error naming `[keys] key_hints_delay`.
- **`[keys.leader_groups]`**: a label for each group of the leader
  layer, keyed by its sequence after the leader (spelled as in
  `[keys.leader_map]`). The label shows as `+label` in the menu, the
  status hint and the menu title. A group is a sequence some longer
  layer sequence begins with (`f` of `f f`), or the sequence of an action
  that opens a table of its own (`window_commands`, whose label defaults
  to `windows`: `w = "windows"` in the default layer, wherever
  `window_commands` is mapped). Errors name `[keys.leader_groups]` and
  the sequence: not a group (naming the action when the sequence runs
  one), an empty label, a key `normalizeKey` refuses.
  Resolved by `core.ResolveLeaderGroups` against the effective layer
  (`core.LeaderAction.Group` marks `window_commands`).
- `[keys.leader_map]` sequences may not contain `backspace` (§1).

Delivery follows the other `[keys]` settings: `config.Keys` →
`uiSettings.KeyHints` / `KeyHintsDelay` / `LeaderGroups` →
`App.SetKeyHintsEnabled` / `SetKeyHintsDelay` / `SetLeaderGroups`. nil
keeps the App's defaults; `--demo` passes nothing.

### 9. Help overlay and docs

The leader's help row reads "leader key (shows what can follow)"; the
layer's rows add `\ ?  show keys for this pane`. The wiki documents the
menu (Keybindings), the three settings and the groups table
(Configuration; the neovim example gains `[keys.leader_groups]` and
`key_hints = ["s k"]`), and the feature (Features).

## Edge cases

- **Too small**: `View` returns "" when not even one row fits; the
  status hint carries on.
- **Theme change**: styles are built per draw from `messages.ChromeStyles()`.
- **A click while a sequence is pending** changes focus without
  cancelling it (unchanged); a context menu stays the one built when it
  opened.
- **`<leader><leader>`** with a displaced key is an ordinary row.
- **The menu and a modal** never show together: opening a modal goes
  through `SetMode`, which closes the menu.

## Testing

- `keyseq`: `Back`, `Arm`, `Title`, `Label`, `Items` (runs, groups,
  `Binding.Group`), `+label` hints, backspace while pending.
- `keyhints`: order, glyphs, columns, paging and the page keys, too
  small, title and footer in the border.
- `chrome.Panel`; `overlay.Place` keeps every byte outside the box,
  including a kitty placeholder row.
- `core`: `key_hints` default, `ResolveLeaderGroups` (groups, the
  `window_commands` table, normalization, every error); `config`:
  `key_hints`, `key_hints_delay`, `[keys.leader_groups]`, `backspace` in
  a sequence, errors naming the field.
- `internal/ui` through `Update`: the menu only after the delay (the
  real tick, `deliveredMsgs`), never for a fast full sequence; root,
  nested group, Backspace, Esc; labels from `[keys.leader_groups]`;
  `ctrl+w`; `g`; marks for `m` and `'`; the context menu in the sidebar
  (with configured sidebar keys), the channel pane, the thread pane, the
  Threads and Activity lists; disabled falls back to the status hint;
  paging; sixel placements under the menu withheld; the scope-table
  lockstep; dialog goldens (leader root, nested group, `ctrl+w`, marks,
  the context menu in the sidebar and in the messages pane, overflow).
- `cmd/slk`: `uiSettings` delivers the settings; the wiki's neovim
  example with `[keys.leader_groups]`.

## Out of scope

- **Composer vim operator-pending hints** (`d`/`c`/`y` in C-NORMAL):
  they live on `feat/compose-vim-mode`; a follow-up adds a hint source
  there.
- **Reaction navigation**: a sub-state of the pane, not a prefix, with
  its own visible cursor; the leader is inert there, so `key_hints`
  cannot open in it either.
- **Modal keys**: every modal's keys are a `HandleKey` switch, not data
  (the confirm prompt's `KeyMap` already draws its keys as buttons), and
  the leader is inert in modals. Listing them needs those switches as
  tables first.
- **Insert, command and search modes**: the leader is Normal-only.
- **`1`–`9` (switch workspace)**: not a `KeyMap` binding (the default
  arm of `handleNormalKey`); listing them needs one.
- **Mouse** clicks on a row (§6).
- **Nerd Font icons** per row: slk does not assume Nerd Fonts, and their
  cell width varies by font.
- **Leader group rows in the help overlay** (`\ f  +find`): the overlay
  lists what runs, sorted by description.
