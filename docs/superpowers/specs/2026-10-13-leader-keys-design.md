# Leader keys — design

**Date:** 2026-10-13
**Status:** Proposed
**Builds on:** `2026-06-11-window-management-design.md` (the `ctrl+w` chord, the
window tree, §7 "thread follows the focused window"),
`2026-09-24-thread-stacked-layout-design.md` (the stacked layout and its focus
rule).

## Problem

1. **No leader key.** Multi-key commands exist only as hand-written pending
   flags in `internal/ui/mode_normal.go`: `pendingWinCmd` (`ctrl+w`),
   `pendingTop` (`gg`), `pendingMark` (`m`), `pendingJumpMark` (`'`). Each one
   is its own `App` field, its own branch at the top of `handleNormalMode`,
   its own hint string and its own disarm in `SetMode`. Adding a vim-style
   `<leader>` layer that way would mean one more flag per chord.
2. **The "open something" keys are awkward ctrl chords**, several of which
   collide with the terminal around slk: `ctrl+b` (tmux's prefix), `ctrl+a`
   (screen's prefix), `ctrl+s` (XOFF when the tty has `ixon`),
   `ctrl+shift+y` (needs the kitty keyboard protocol or `modifyOtherKeys`;
   most terminals cannot send it), `ctrl+]` (layout-dependent).
3. **`ctrl+w h/j/k/l` only moves between split windows.** `handleWindowChord`
   (`internal/ui/windows.go`) sends h/j/k/l to `navigateWindow`, which calls
   `wintree.NavigateDir` from the *focused window's* rect inside the message
   region. Consequences today:
   - no directional key reaches the sidebar or the thread pane (only
     `Tab`/`Shift+Tab`/`h`/`l` cycle panels);
   - with the sidebar or thread focused, `ctrl+w h/j/k/l` still navigates
     from the focused window, as if focus were there: from the thread with
     one window, `ctrl+w h` does nothing;
   - in the Threads and Activity views the message region shows one list,
     but `ctrl+w h/j/k/l` walks the (undrawn) window tree behind it.

## Current behavior (references)

- `KeyMap` (`internal/ui/keys.go`): hardcoded `key.Binding`s; keyless
  help-only entries (`WinSplit`, `WinNavigate`, …) document the `ctrl+w`
  chords in the help overlay (`help.FromKeyMap`).
- `handleNormalMode`: the `pendingWinCmd` branch runs first; the
  `WindowPrefix` arm arms it and sets the hint `ctrl+w …` through
  `statusbar.SetHelpHint`. Esc or an unmapped key cancels silently and is not
  processed (vim). `SetMode` disarms it.
- `gg`: an unmatched second key cancels the chord **and** is then handled on
  its own.
- `space` = `ToggleSection` (collapse the sidebar section header under the
  cursor); `Enter` on a header toggles it too (`handleEnter`), and so does a
  click.
- Panels: `PanelWorkspace` (the rail; never focusable), `PanelSidebar`,
  `PanelMessages` (the window tree in the channel view, one list in the
  Threads/Activity views), `PanelThread`. Columns come from
  `panelLayout.Compute`; the thread stacks behind the message region when
  there is no room for both (`threadInFront`, `threadDrawnAlone`).
- Config has no `[keys]` section; display settings reach the App through
  `cmd/slk/ui_settings.go` (`uiSettings` → `applyUISettings`), which demo mode
  also uses.

## Design

### 1. `internal/bubbles/keyseq`: a declarative sequence table

A self-contained component (no `internal/ui` import, no I/O):

```go
type Action func(tea.KeyMsg) tea.Cmd

type Binding struct {
	Keys   []string // key strings (tea.Key.String()); Keys[0] is the label
	Hint   string   // short which-key label ("split")
	Desc   string   // help-overlay description
	Action Action   // run when this key completes a sequence
	Next   *Map     // non-nil: a prefix; the next key is looked up here
}

func New(bindings ...Binding) *Map
func (m *Map) Lookup(k string) (Binding, bool)  // first binding listing k
func (m *Map) Hints() []string                  // "h/j/k/l focus", "s split", …
func (m *Map) Help(prefix string) []Entry       // flattened help rows

type Sequence struct{ … }                       // zero value = idle
func (s *Sequence) Press(root *Map, k string) (Action, bool)
func (s *Sequence) Pending() bool
func (s *Sequence) Reset()
func (s *Sequence) Hint() (lead string, entries []string)
```

- `Press` idle: `k` is looked up in `root`; a prefix arms the sequence, an
  action completes it, anything else is *not consumed* (the caller handles
  `k` as a plain key). Pending: `k` is looked up in the armed map and is
  always consumed; `esc` and keys the map lacks cancel.
- Consecutive bindings with the same `Hint` (or `Desc`) are shown once with
  their labels joined by `/`, so h/j/k/l read `h/j/k/l focus`.
- One `Sequence` replaces `pendingWinCmd`. The table is data, so future
  user-configurable bindings add `Binding`s instead of flags.

### 2. The App's tables (`internal/ui/keyseqs.go`)

`(*App).newKeySequences()` builds, from the App's `KeyMap`:

- **window commands**: the former `handleWindowChord` switch as a `Map`
  (same keys, same functions: `splitWindow`, `cycleWindow`, `closeWindow`,
  `onlyWindow`, and h/j/k/l → `focusPaneDir`). Descriptions come from the
  `KeyMap`'s `Win*` help entries, so they cannot drift.
- **the leader layer**: `w` → the *same* window-command `Map` pointer, so
  `<leader>w x` and `ctrl+w x` are one code path; plus the actions below.
  Each action's description is read from the `KeyMap` binding it mirrors.
- **the root**: `Leader` (→ leader layer) and `WindowPrefix` (→ window
  commands). The leader is first, so it wins a collision.

The actions the leader reuses are the bodies of their `handleNormalMode`
arms, moved into methods both call (`openChannelFinder`,
`openWorkspaceSearch`, `toggleActivityView`, `enterNewMessage`,
`openPresenceMenu`, `openWorkspaceThemeSwitcher`, `openGlobalThemeSwitcher`)
plus the existing `ToggleSidebar` / `ToggleThread`. No new actions.

### 3. Dispatch (Normal mode only)

`handleNormalMode` becomes:

1. pending sequence → `pressKeySeq` (consumes the key);
2. the existing `gg` / `m` / `'` pending branches, unchanged;
3. reaction-nav sub-state, unchanged (it swallows keys, as it does `ctrl+w`
   today);
4. `pressKeySeq` against the root: the leader or `ctrl+w` arms a sequence;
5. otherwise `handleNormalKey`: the ephemeral-selection guard and the
   existing `switch`, minus the `WindowPrefix` arm.

The leader therefore never acts in Insert mode, the `:`/`/` prompts or any
modal: those modes have their own handlers in `modeHandlers` and never reach
`handleNormalMode`. `SetMode` resets a pending sequence (as it disarmed
`pendingWinCmd`), so a global intercept such as the `ctrl+c` prompt cannot
strand one.

**Esc** cancels silently. **An unbound key** cancels silently and is *not*
processed on its own — the `ctrl+w` (and vim) rule, not `gg`'s. A leader is a
deliberate command prefix; a slip after it must not run `D` (delete), `Q`
(quit), `1`–`9` (switch workspace) or similar. **No timeout**, like every
existing chord: a timeout drops slow input and needs a tick, and the
which-key hint (below) makes the pending state visible.

### 4. Which-key hint

While a sequence is pending the status bar's hint area shows the keys typed
so far and every continuation:

```
\ …  w window  t channel  f search  a activity  n new msg  s status  y theme  Y default theme  b sidebar  ] thread
\ w …  h/j/k/l focus  s split  v vsplit  w cycle  q/c close  o only
ctrl+w …  h/j/k/l focus  s split  v vsplit  w cycle  q/c close  o only
```

New `statusbar.SetKeyHint(lead, entries)`: the lead, then as many entries as
fit the free space, then `…` when some were left out; dropped whole only when
not even the lead fits (today's `SetHelpHint` rule). `SetHelpHint` replaces
it; completing or cancelling restores the resting `? for keybindings`.

### 5. Directional pane focus (`focusPaneDir`)

h/j/k/l after `ctrl+w` or `<leader>w` move focus to the pane on screen in
that direction, across **sidebar, message-region windows and thread**:

- **Panes and geometry.** From one `panelLayout.Compute`: the sidebar column
  (when visible); the message region at the width the window tree fills
  (`windowBounds`), split into the window rects (`wintree.ComputeRects`) in
  the channel view, or one pane in the Threads/Activity views; the thread
  column to its right (when open). The workspace rail is not a focus target
  (it never was).
- **Stacked layout.** When the terminal is too narrow for messages and thread
  side by side, the thread still counts as sitting right of the message
  region, where it is drawn with room: `l` brings it to the front and `h`
  goes back, the same panes `Tab` alternates. Directions keep their meaning
  when the terminal is resized. One exception: when the thread is the pane
  in front and focus is on the sidebar, `l` enters the thread — the pane
  the user sees there — instead of pulling the hidden channel forward.
- **Rule.** A pane is a neighbour when it touches the current pane's edge in
  that direction with positive overlap on the other axis (the existing
  `wintree` rule, now `wintree.Adjacent`). Entering the message region from
  the sidebar or thread picks the **focused window** when it is a neighbour
  (it owns the open thread); otherwise the neighbour with the largest
  overlap, ties to the earliest in screen order (`wintree.Nearest`, which
  replaces `NavigateDir`). No neighbour is a silent no-op.
- **Effect.** Moving to another window goes through `focusWindow`, unchanged
  (so §7 still closes the thread, and an upload still refuses). Moving to the
  sidebar, the thread, or back to the focused window from them is a plain
  panel focus change like `Tab` (`cancelEdit`, `clearSelections`).

**Behavior change for existing users (requested):** `ctrl+w h/j/k/l` now
shares this: from the leftmost window `ctrl+w h` focuses the sidebar, from
the rightmost `ctrl+w l` the thread, and from the sidebar/thread the move
starts at the focused pane, not at the focused window. In the
Threads/Activity views the message region is one pane. Window-to-window
moves are unchanged (same adjacency rule, same tie-break). The help row
`ctrl+w h/j/k/l` now reads "focus pane in direction".

### 6. Configuration

```toml
[keys]
leader = "space"   # default: "\\" (TOML) — i.e. a backslash; '\' also works
```

- `internal/config` parses and normalizes `keys.leader`: one key, written as
  slk matches keys — a single printable character (`\`, `,`, `;`, `-` …),
  `space` (or `" "`), or that with `ctrl+` / `alt+` in front.
- Rejected at startup with an error naming the key (like the existing
  `[workspaces]` validation): anything else (`esc`, `tab`, `enter`, `f1`,
  words, `shift+…`), `1`–`9` (workspace switching; not bindings the leader
  could hand back, see below), `ctrl+c` (intercepted before Normal mode,
  so it could never start a sequence), and keys a terminal without
  keyboard enhancements sends as a different key, so the leader would
  silently never fire: `ctrl+` with anything but a letter, `space` or one
  of `\ ] ^ _` (`ctrl+[` is `esc`, `ctrl+@` is `ctrl+space`, `ctrl+,` has
  no control code), `ctrl+i` (`tab`), `ctrl+m` (`enter`), and `alt+` with
  an uppercase letter (reported as `alt+shift+y`, `alt+Y` or `alt+shift+Y`
  depending on the terminal).
- Unset or empty keeps the default. Delivery follows the other display
  settings: `cfg.Keys.Leader` → `uiSettings.Leader` → `App.SetLeader`, which
  rebinds `KeyMap.Leader` and rebuilds the tables. Demo mode passes nothing,
  so it runs the default.

**Default leader: `\`** (vim's own default `mapleader`). It is unbound in
slk, so no existing user loses a key. `space` was the alternative, but space
is `ToggleSection`, and keeping both under one key needs either a timeout or
context-dependent behaviour (space on a section header toggles, elsewhere
leads), which makes the leader unreliable exactly where the user is. Users
who prefer space set `leader = "space"` and keep section toggling (below).

**A leader that displaces a binding.** The leader wins. Pressing the leader
twice (`<leader><leader>`) runs the displaced key's own Normal-mode action,
and the which-key hint and help list it under the displaced description. For
`leader = "space"` that is `space space` = toggle section; `Enter` and a
click on a section header toggle it too, as today. The entry exists only when
the key is a Normal-mode binding in `KeyMap` (`Leader`, the sequence
prefixes and the insert-only `ctrl+o` / `ctrl+e` do not count), and it comes
first in the leader layer. A layer letter equal to the leader would be
unreachable then, so it is left out of the layer (its ctrl chord still
works). A leader of `ctrl+w` displaces the window prefix; window commands
stay on `<leader>w`.

### 7. Help overlay and docs

`?` lists `KeyMap` as today plus rows generated from the leader layer with
the configured key: `\ t  switch channel`, `\ w h/j/k/l  focus pane in
direction`, … (`help.FromKeyMap(km, extra...)` merges and sorts them). The
`KeyMap` rows are first rewritten for the leader (`helpKeyMap`): a binding on
the leader's key loses that key (`Y/C copy permalink` becomes `C` for a `Y`
leader; a row with no key left is dropped), and keyless chord rows under the
leader's key (`ctrl+w s / :sp` for a `ctrl+w` leader) are dropped. Insert-only
rows are untouched.
Because descriptions are the mirrored bindings', each leader row sorts next
to its ctrl chord. `KeyMap` gains one real binding, `Leader`. The wiki
Keybindings page gets the leader table and the `ctrl+w` rows it lacked;
Configuration documents `[keys]`.

### 8. Configurable sidebar keys (amendment, 2026-10-14)

**Problem.** In the sidebar, Enter activates the selected row and space
toggles a section header. Tree pickers in neovim (nvim-tree, neo-tree) use
`l` or Enter to open a file and expand or collapse a directory. Upstream uses
space for a reason (`ToggleSection`, issue #184), so the defaults stay; the
keys become configurable.

**Current behavior.** `handleEnter` (`internal/ui/app.go`) had a sidebar
branch: the Threads row → `ThreadsViewActivatedMsg`, the Activity row →
`ActivityViewActivatedMsg`, a section header → `ToggleCollapseSelected`, a
channel or DM → `ChannelSelectedMsg` (whose reducer moves focus to the
messages pane). `KeyMap.ToggleSection` (space) was an arm in
`handleNormalMode` that toggled a header only with the sidebar focused. `h`/`l`
(`Left`/`Right`) are `FocusPrev`/`FocusNext` everywhere.

**Config.** Two `[keys]` lists:

```toml
[keys]
sidebar_open = ["enter", "l"]       # default ["enter"]
sidebar_toggle_section = ["space"]  # default ["space"]; [] unbinds
```

Each is a `*[]string` in `config.Keys`: nil (unset) keeps the default, an
empty list unbinds the action. `config.Keys.Bindings()` hands the UI the
set ones by name. Keys go through the leader's normalizer, now shared:
`normalizeKey` accepts a single character, the named keys `space`,
`enter`, `tab`, `backspace`, `delete`, `insert`, `home`, `end`, `pgup`,
`pgdown` and the arrows, and a character or space after `ctrl+`/`alt+`. It
rejects `esc`, `ctrl+c`, `shift+`, modifiers on other named keys, and the
spellings a terminal delivers as another key (§6). `normalizeLeader` adds the
leader's own rules (no named key but space, no 1–9). A binding also refuses
`ctrl+w` (window commands take it first) and a key that another action
already has (one key cannot do two things). A key repeated within one list is
kept once. Errors name the field (`[keys] sidebar_open: …`).

**Action table.** `internal/ui/keyactions.go` lists the configurable actions:
`{name, KeyMap binding, scope panel, run}`. `KeyMap` gains `SidebarOpen`
(default `enter`, "open row / toggle section (sidebar)"); `ToggleSection`
keeps space. `App.SetKeyBindings(map[string][]string)` rebinds by name and
rebuilds the sequence tables. The sidebar branch of `handleEnter` became
`openSidebarRow`, the `sidebar_open` action; `handleEnter` is Enter in the
content panes only, so with `sidebar_open = ["l"]` Enter in the sidebar does
nothing. The `ToggleSection` arm left `handleNormalMode`. A future
configurable action is a table row, a `KeyMap` binding and a `config.Keys`
field; `cmd/slk` tests that config and UI name the same actions.

**Scope and precedence.** `handleNormalKey` first offers the key to the
actions scoped to the focused panel, so in the sidebar `l` opens when
configured, and everywhere else it is still `FocusNext`. Before that come
key sequences: the leader wins a sidebar key, and `ctrl+w` (refused in
config) keeps window commands. Leaving the sidebar stays possible through
Tab, `ctrl+w l`, `<leader>w l` or opening a channel. A scoped key always
consumes the press in its panel: `l` configured to toggle does nothing on a
channel row rather than moving focus.

**Leader interplay.** `<leader><leader>` is built from the configured keys
(`displacedBindings` walks the live `KeyMap`), not from space: with
`sidebar_toggle_section = ["x"]` and `leader = "x"`, `x x` toggles; with the
toggle moved off space, `space space` offers nothing. When the displaced key
means different things in different panels (leader `l` with `sidebar_open`
on `l`), the entry is labelled with every action (`right; open row / toggle
section (sidebar)`) and runs the one for the focused panel.

**Help.** The two actions' rows show their configured keys (`enter/l  open
row / toggle section (sidebar)`); an unbound action has no row.

### 9. Configurable leader layer (amendment, 2026-10-14)

**Problem.** The layer in §2 is fixed. Users coming from neovim expect
their own conventions: `<space>ff` / `<space><space>` for the finder,
`<space>/` or `<space>sg` to search, `<space>e` for the tree, `<space>?`
for help, `<space>q` to close. The defaults stay exactly as they are.

**Schema.** A sub-table of `[keys]`, named `leader_map` (vim's `map`: a
mapping from a sequence to an action). `[keys.leader]` is impossible, since
`leader` is already the key string, and `leader_map` reads as "what follows
the leader", the same way `nmap <leader>ff` does in vim.

```toml
[keys.leader_map]
channel_finder = ["f f", "space"]   # action = sequences after the leader
workspace_search = ["/", "s g"]
```

A sequence is keys separated by spaces, each spelled by `normalizeKey`
(§8): `"f f"`, `"space"`, `"/"`, `"ctrl+x"`, `"enter"`. A set action gets
exactly its sequences, replacing its defaults; `[]` removes it from the
layer; an unset action keeps its defaults.

**Registry.** `core.LeaderActions()` names every leader action, with its
default sequences, in hint order. `leaderActions` (`internal/ui/
leader_layer.go`) binds each name to its hint label, the `KeyMap` binding
whose description it shows, and its handler; `TestLeaderActionsMatchCore`
keeps the two in step. The names live in `core` because both sides need
them: the config layer validates a map against the layer it changes, and
the TUI builds the layer. `core` is where the TUI and `cmd/slk`'s
collaborators already share values, and the table is data (names and
default sequences) plus one pure function, so the no-I/O and import rules
hold: `config` already imports `core`, and `ui` reads it.

This registry is separate from §8's `keyActions` on purpose. Those are keys
scoped to a panel, configured as `[keys]` fields with `KeyMap` defaults, and
they act on the sidebar selection. These are sequences in the leader layer,
configured in `[keys.leader_map]` with `core` defaults, and every one is a
global command. No action is in both, so one registry would leave half of
each row unused.

Actions: the default layer's `window_commands` (`w`, the shared `ctrl+w`
table), `channel_finder` (`t`), `workspace_search` (`f`),
`toggle_activity` (`a`), `new_message` (`n`), `set_status` (`s`),
`switch_theme` (`y`), `set_default_theme` (`Y`), `toggle_sidebar` (`b`) and
`toggle_thread` (`]`). With no default: `help` and `close_thread` (the `?`
and `q` arms, extracted as `openHelp` / `closeThreadView`), plus the other
global Normal-mode commands that are one existing call: `nav_back` /
`nav_forward`, `next_unread` / `prev_unread`, `next_theme` / `prev_theme`,
`command_mode` and `quit`. Left out:
- anything that acts on the selection or the focused pane (message ops,
  `i`, `/`, `n`/`N`, scrolling, `gg`/`G`, Tab, reactions);
- keys that take an argument (marks, `1`–`9`);
- the sidebar resize keys `[`/`]` (a resize plus a settings save, not one
  call);
- the `:ws` / `:marks` commands, which go through the command line.

**Resolution and validation** (`core.ResolveLeaderMap`, called by
`config.Load` with `normalizeKey`; errors name `[keys.leader_map] <action>`):
- an unknown action, an empty sequence, or a key `normalizeKey` refuses is
  an error;
- two configured sequences that are equal, or where one begins the other
  (`f` and `f f`: there is no timeout, so the longer could never be typed),
  are an error, also within one action. A repeat within one action is kept
  once;
- **explicit beats implicit**: a default sequence that collides with a
  configured one is dropped, and its action keeps its ctrl chord and any
  other sequences. This is what makes the neovim example load: `"s g"`
  makes `s` a group, so `set_status` loses its default `s`. It also means
  `channel_finder = ["f f"]` loads on its own and takes `f` from
  `workspace_search`.

`config.Keys.LeaderLayer()` returns the resolved layer (`[]core.LeaderSeq`,
each marked explicit or default); `uiSettings.LeaderLayer` →
`App.SetLeaderLayer` installs it. nil (no map) keeps
`core.DefaultLeaderLayer()`; a map that removes everything is an empty,
non-nil layer.

**Building.** `newLeaderLayer` inserts each sequence into a tree. Keys
before the last make groups: a group binding has the which-key hint `…`
(the hint after the leader reads `f …`, and after `f`, `space f …  f
channel`). Bindings take the description of the `KeyMap` binding they
mirror, so help rows read `space f f  switch channel` and sort next to
`ctrl+t`. Esc and unbound keys inside a group cancel, as anywhere.

**`<leader><leader>`.** Explicit beats implicit here too. A configured
sequence that starts with the leader's key wins, and the displaced-key
fallback is left out: with `leader = "space"` and `channel_finder =
["space"]`, `space space` opens the finder, and the section toggle it would
have reached stays on `Enter` / `sidebar_open`. Otherwise the fallback
still wins over a *default* sequence on the leader's key (the §2 rule:
`leader = "b"` drops the default `b` sidebar entry).

**The neovim example** (in `wiki/Configuration.md`, run verbatim by
`cmd/slk`'s `TestNvimExampleConfig`):

```toml
[keys]
leader = "space"
sidebar_open = ["enter", "l"]
[keys.leader_map]
channel_finder = ["f f", "space"]
workspace_search = ["/", "s g"]
toggle_sidebar = ["e"]
help = ["?"]
close_thread = ["q"]
```

## Key table (default leader `\`)

| Keys | Action | Mirrors |
|---|---|---|
| `\` | start a leader sequence (which-key hint) | — |
| `\ w h/j/k/l` (or arrows) | focus pane left/down/up/right | `ctrl+w h/j/k/l` |
| `\ w s` / `\ w v` | split stacked / side by side | `ctrl+w s` / `v` |
| `\ w w` | cycle windows | `ctrl+w w` |
| `\ w q` / `\ w c` | close window | `ctrl+w q` / `c` |
| `\ w o` | close other windows | `ctrl+w o` |
| `\ t` | switch channel (finder) | `ctrl+t` / `ctrl+p` |
| `\ f` | search workspace | `ctrl+f` |
| `\ a` | toggle the Activity view | `ctrl+a` |
| `\ n` | new message | `ctrl+n` |
| `\ s` | set status (presence menu) | `ctrl+s` |
| `\ y` | switch theme (per workspace) | `ctrl+y` |
| `\ Y` | set default theme | `ctrl+shift+y` |
| `\ b` | toggle sidebar | `ctrl+b` |
| `\ ]` | close thread | `ctrl+]` |
| `<leader><leader>` | displaced key's own action (only when one is displaced) | — |
| `Esc` / unbound key | cancel, nothing else happens | — |

The mnemonic is one rule: **the leader stands in for `ctrl`** — every entry is
the letter of the ctrl chord it mirrors (`w` for `ctrl+w`, `t` for `ctrl+t`,
…). The layer is the Normal-mode ctrl chords that open something or toggle
layout. Left out on purpose: `ctrl+u`/`ctrl+d` (scrolling, vim-native and
comfortable), `ctrl+h`/`ctrl+l` (history; `<leader>h`/`l` would read like
`<leader>w h`/`l`), `?` (already one key), and insert-mode chords (the
leader is Normal-only).

## Edge cases

- **Mode changes mid-sequence** (ctrl+c prompt, a reducer opening a modal):
  `SetMode` resets the sequence and restores the hint.
- **`g`, `m`, `'` pending, then the leader:** those branches run first, as
  before; `gg`'s fallback hands the leader to step 4, which arms it.
- **Reaction navigation:** swallows keys before step 4; neither the leader
  nor `ctrl+w` starts there (unchanged for `ctrl+w`).
- **Bootstrap loading / image preview:** keys never reach the mode handler;
  the leader is inert.
- **Upload in progress:** window focus changes refuse with the existing
  toast; sidebar/thread focus changes are allowed, like `Tab`.
- **Sidebar hidden:** `h` from the leftmost pane is a no-op (the rail is not
  focusable). **No thread:** `l` from the rightmost window is a no-op.
- **Leader equal to a layer letter** (`leader = "y"` displaces copy-message):
  `y y` is the displaced action; the layer's own `y` (theme) could never be
  reached, so it is left out of the table, the hint and the help, and keeps
  its ctrl chord.
- **Narrow status bar:** the hint keeps the lead and as many entries as fit,
  then `…`.

## Testing

- `keyseq`: lookup, prefixes, `Press` (idle/pending, esc, unbound), hint and
  help merging.
- `statusbar.SetKeyHint`: fits, truncates with `…`, drops when the lead does
  not fit, `SetHelpHint` replaces it.
- `wintree`: `Adjacent`, `Nearest` (the former `NavigateDir` cases).
- `config`: `[keys] leader` normalization and every rejection; the action
  bindings (unset vs `[]`, normalization, repeats, every rejection,
  conflicts between actions).
- `internal/ui` (`runKeyCases` and the real `Update` chain): default leader
  dispatch for each layer entry; configured leaders including `space` with
  `space space` and `Enter` toggling a section; `<leader>w`/`ctrl+w`
  h/j/k/l across sidebar, messages, thread and splits, stacked layout,
  Threads view, hidden sidebar, focused-window preference and §7; Esc and
  unbound keys cancel without side effects; the leader is ignored in Insert
  mode and in modals; the hint; the help overlay rows. §8: the defaults
  (`l` still moves focus in the sidebar), `sidebar_open = ["enter", "l"]` on
  headers, channels (through `Update`: the channel opens and focus moves),
  the Threads and Activity rows and in the messages pane, unbinding,
  moving the toggle, the leader on a sidebar key, the help rows.
- `cmd/slk`: config and UI name the same actions; `uiSettings` delivers the
  leader, the bindings and the leader layer; the wiki's neovim example
  loads and behaves as described.
- §9: `core` resolution (defaults, removal, replacement, explicit dropping
  a default, every error, an empty layer); `config` (`[keys.leader_map]`
  normalization and errors naming the action); `internal/ui` (the registry
  matches `core`, the neovim layer through `Update`, group hints, help
  rows, removal and replacement, the extra actions).

## Out of scope

- **More configurable actions, and user-defined sequences outside the
  leader layer.** §8's table is the place for more panel-scoped keys (only
  the two sidebar actions are configurable now), and §9's registry for
  more leader actions.
- **Moving `gg`, `m`, `'` onto `keyseq`.** `m`/`'` take any letter and `gg`
  re-processes an unmatched key; both need table features (a wildcard, a
  fallback) this feature does not use.
- **A timeout**, a which-key popup window (since designed in
  `2026-10-14-adaptive-key-hints-design.md`), focusing the workspace rail,
  and `ctrl+w =`/`<`/`>`/`+`/`-` resizing (not implemented today either).
