# Configuration

Config lives at `~/.config/slk/config.toml`.

## Full example

```toml
[general]
default_workspace = "work"      # the slug, not the team ID
use_slack_sections = true       # use real Slack sidebar sections (default).
                                # set false to use [sections.*] globs instead.
download_dir = "~/Downloads"    # where file attachments (`d` keybinding) are
                                # saved. "~" is expanded to your home directory.

[appearance]
theme = "dracula"
timestamp_format = "3:04 PM"
image_protocol = "auto"   # auto | kitty | sixel | halfblock | off
max_image_rows = 20       # cap inline image height in terminal rows
colored_usernames = false # color each user's name by ID hash (Slack-style)

[animations]
enabled = true
smooth_scrolling = true
typing_indicators = true  # also apps' "working" status (e.g. "Atlas is working...")

[notifications]
enabled = true
on_mention = true
on_dm = true
on_keyword = ["deploy", "incident"]
quiet_hours = "22:00-08:00"   # planned

# notify_command (optional): run INSTEAD of the built-in OS notification for any
# message that would notify (DM / mention / keyword). Executed via `sh -c` with
# $SLK_TITLE and $SLK_BODY set, so you can route notifications through your own
# tooling (terminal-notifier, a multiplexer's notifier, mako, ...). Values are
# passed via the environment, so message text can't inject shell syntax.
# notify_command = 'terminal-notifier -title "$SLK_TITLE" -message "$SLK_BODY"'

# status_command (optional): run on every unread-state change (a message arrives
# or a channel is read) so an external surface can mirror slk's unread state.
# Because it fires on reads too, it can clear a status as well as set one.
# Runs are serialized and coalesced: states never run concurrently or out of
# order, and under a burst intermediate states may be skipped — the newest
# state always runs last, so the surface converges on the current state.
# Executed via `sh -c` with:
#   $SLK_UNREAD        unread channels in the active workspace (mute-filtered)
#   $SLK_OTHER_UNREAD  other workspaces with unread channels (mute-filtered) or threads
#   $SLK_WORKSPACE     active workspace name
#   $SLK_TITLE         the window-title string, e.g. "slk SW (3) +1"
# status_command = 'my-statusbar --slack-unread "$SLK_UNREAD"'

# Both hooks require a POSIX `sh` on $PATH and are unavailable on Windows
# (the built-in OS notification still works there). Hook failures are silent
# in the UI; run slk with SLK_DEBUG=1 and check slk-debug.log ([notify] lines)
# to diagnose a misbehaving command.

# Muted channels and DMs never notify — including on mentions and keywords —
# matching Slack. (This is a behavior change: previously a mention or keyword
# in a muted channel would still notify.)

[compose]
editor = "nvim"   # Ctrl+E editor, used when $VISUAL and $EDITOR are unset

[cache]
message_retention_days = 30
max_db_size_mb = 500
max_image_cache_mb = 200

[marks]
# Vim-style marks. Lowercase marks (ma..mz) are session-only and
# uppercase marks (mA..mZ) survive a restart, matching vim's
# buffer-local / global split.

# Make lowercase marks persist too. Turning this back off is not
# destructive: previously saved lowercase marks stop loading, and
# reappear if you turn it on again.
persist_all = false

# Pressing ' opens the marks overlay so you can see what is set
# without memorising letters. Set false for plain vim behaviour: '
# waits silently for a letter. This never affects `:marks`, which
# always shows the overlay.
show_jump_overlay = true

[keys]
# The Normal-mode leader key (see Keybindings → Leader key). One key: a
# single character, "space", or either after ctrl+ / alt+ ("ctrl+space").
# Default '\' (a TOML literal string; the basic-string form is "\\").
# Refused at startup: esc, ctrl+c, 1-9, and keys many terminals send as
# another key — ctrl+ with anything but a letter, space or \ ] ^ _ (ctrl+[
# is esc), ctrl+i (tab), ctrl+m (enter), alt+ with an uppercase letter.
# If the key already did something (space toggles a sidebar section),
# pressing it twice still does.
leader = "space"

# Sidebar keys. Each is a list of keys; leaving it out keeps the default,
# [] unbinds it. They only apply while the sidebar has focus, where they
# win over the key's usual job: with "l" below, l opens in the sidebar
# and still moves focus everywhere else. Besides the spellings the leader
# takes, these accept key names: enter, tab, backspace, delete, insert,
# home, end, pgup, pgdown, up, down, left, right. esc, ctrl+c and ctrl+w
# are refused, and so is a key both lists share. If the leader is one of
# these keys, the leader wins and pressing it twice does the sidebar action.
#
# sidebar_open: open the selected channel, DM, Threads or Activity row, or
# expand / collapse a section header. Default ["enter"].
# sidebar_toggle_section: expand / collapse the section header under the
# cursor. Default ["space"].
#
# nvim-tree / neo-tree style — l or Enter opens and expands:
sidebar_open = ["enter", "l"]
sidebar_toggle_section = ["space"]

# The key-hints menu: after a pause in a key sequence (the leader,
# Ctrl+w, g, m), a panel lists what can follow. See "Key hints" below.
# key_hints = false keeps the status-bar hint only. Default true.
key_hints = true
# Milliseconds a sequence waits before the menu appears; 0 shows it at
# once. 0-10000, default 300.
key_hints_delay = 300

# The leader layer: which keys after the leader run which action. See
# "Leader map" below for the actions, the rules and an nvim-style setup.
[keys.leader_map]
help = ["?"]
key_hints = ["k"]

# Labels for the leader layer's groups, shown as "+label" (see "Leader
# groups" below). The window commands are "windows" unless you say so.
[keys.leader_groups]
w = "window"

# Glob-based channel sections — only consulted when use_slack_sections
# is false (globally or per-workspace), or when Slack's section API is
# unreachable. Otherwise slk reads the user's actual Slack sections.
[sections.Alerts]
channels = ["alerts", "ops", "*-alerts"]
order = 1

# Channels can carry an optional ":<N>" suffix to pin their order
# within the section. Lower numbers sort higher. Entries without a
# suffix fall after annotated ones, in the order they appear.
# This syntax is only honored when use_slack_sections = false;
# in Slack-native mode, channel order comes from Slack.
[sections.Engineering]
channels = ["eng-general:1", "eng-alerts:2", "eng-*"]
order = 2

# Per-workspace settings: keyed by a slug you choose at --add-workspace
# time. team_id ties the slug to the underlying Slack workspace.
[workspaces.work]
team_id = "T01ABCDEF"
order   = 1                     # rail position; 1-based, used by 1-9 keys
theme   = "dracula"             # overrides [appearance].theme
use_slack_sections = false      # this workspace uses [sections.*] globs;
                                # other workspaces still use Slack sections

[workspaces.work.sections.Alerts]
channels = ["alerts", "*-alerts"]
order = 1

[workspaces.work.sections.Engineering]
channels = ["eng-*", "deploys"]
order = 2

# A second workspace with no per-workspace sections — falls back to
# the global [sections.*] above.
[workspaces.side]
team_id = "T02XYZ"
order   = 2

# Inline color overrides on top of the active theme
[theme]
primary = "#4A9EFF"
accent = "#50C878"
background = "#1A1A2E"
text = "#E0E0E0"
```

## Section resolution

When `use_slack_sections = true` (the default) and Slack's section endpoint
is reachable, slk reads the user's actual sidebar sections — names, emoji,
linked-list order, and channel membership — directly from Slack and keeps
them live via WebSocket events. Any `[sections.*]` or
`[workspaces.<slug>.sections.*]` blocks in `config.toml` are ignored in this
mode (a one-line info note is emitted to the debug log on first connect so
the shadowing isn't silent). Set `use_slack_sections = false` globally, or
per-workspace, to opt into glob-based sections instead.

Per-workspace `[workspaces.<slug>.sections.*]` blocks fully replace the
global `[sections.*]` for that workspace. Workspaces that define no
sections of their own fall back to the global table.

### Ordering channels within a section

Each entry in a section's `channels` list may carry an optional `:<N>`
suffix where `N` is a non-negative integer. Channels matched by an
annotated pattern sort ahead of channels matched by un-annotated
patterns; among annotated channels, lower `N` wins; un-annotated
channels keep the order Slack returned them in.

```toml
[sections.Engineering]
channels = ["eng-general:1", "eng-alerts:2", "eng-*"]
order = 2
```

In the example above, `#eng-general` is pinned to the top of the
Engineering section, followed by `#eng-alerts`, followed by every
other `eng-*` channel in Slack-API order.

This syntax is only honored when `use_slack_sections = false` (or
when Slack's section endpoint is unreachable and slk falls back to
glob mode). In Slack-native mode, channel order within a section
comes from Slack and the `:<N>` suffix is ignored along with the
rest of the `[sections.*]` block.

### Limitations of Slack-native sections

Slack-native sections are read-only — section editing still happens in the official client; slk
reflects the results. The `stars` section type (Slack's "Starred" feature) is rendered
when non-empty, with the header `Starred`. Sections of type `slack_connect`,
`salesforce_records`, and `agents` are hidden. Sections with more than 10 channels may be returned
only partially by Slack's API on initial load; the missing channels
temporarily fall into the catch-all bucket and migrate into their correct
section as WebSocket events fire or the workspace reconnects. A debug-log
warning identifies which sections were truncated.

## Workspace order

The `order` field controls workspace position in the rail and the mapping
for the `1`–`9` digit keys. Positive values sort ascending (lowest first);
workspaces without an `order` (or with `order = 0`) sort after explicitly
ordered ones, alphabetically by slug. Tokens on disk that have no
`[workspaces.<slug>]` block at all sort last, alphabetically by team ID.
The order is stable across runs. Previously the rail order depended on
which workspace's WebSocket connected first; it is now deterministic
regardless of network timing, even without an explicit `order` set.

Legacy configs that key the block by raw team ID
(`[workspaces.T01ABCDEF]`) keep working unchanged.

## Terminal-palette themes (`ANSI Dark`, `ANSI Light`)

Two built-in themes use ANSI 16 color codes exclusively rather than
fixed RGB values. They inherit the user's terminal color palette, so
changing your terminal colorscheme (light/dark, solarized,
accessibility palettes, etc.) immediately changes slk's UI colors to
match.

```toml
[appearance]
theme = "ANSI Dark"   # or "ANSI Light"
```

Pick the variant whose background matches your terminal's background.

**Trade-off:** selection-row highlights and compose-input tints are
still computed as RGB approximations, so the tint regions of those
elements use truecolor rather than your palette. The rest of the UI
honors the palette.

## Custom themes

Drop `.toml` files into `~/.config/slk/themes/`:

```toml
name = "My Theme"

[colors]
primary      = "#BD93F9"
accent       = "#50FA7B"
warning      = "#FFB86C"
error        = "#FF5555"
background   = "#282A36"
surface      = "#343746"
surface_dark = "#21222C"
text         = "#F8F8F2"
text_muted   = "#6272A4"
border       = "#44475A"

# Optional sidebar/rail overrides — lets you have a darker sidebar with a
# lighter message pane (Slack's default look). Fall back to
# background/text/text_muted/surface_dark when omitted.
sidebar_background = "#19171D"
sidebar_text       = "#D1D2D3"
sidebar_text_muted = "#9A9B9E"
rail_background    = "#19171D"
```

Every built-in theme now sets a channels-panel (sidebar) background that is
perceptibly distinct from the message pane. When writing a custom theme,
set `sidebar_background` to a clearly darker (or, on near-black themes, a
slightly lighter) shade than `background` for the same effect.

Switch themes live with `Ctrl+y`, or cycle through them with `Alt+y` /
`Alt+Shift+y`.

## Marks

`m` followed by a letter records the selected message; `'` followed by
that letter jumps back to it. A mark records the channel, the message,
and — when you are reading inside a thread — the thread and the reply,
so `'a` can put you back on one message in a 200-reply thread.

Case decides lifetime, as in vim:

| | |
|---|---|
| `ma` … `mz` | session marks, gone when slk exits |
| `mA` … `mZ` | persistent, stored in the local SQLite cache |

`persist_all = true` makes lowercase marks persist as well. Turning it
off later does not delete them — they stop loading, and come back if you
turn it on again.

`''` is the back-jump: it returns you to wherever you were before the
last jump, whether or not the jump changed channel. Pressing it again
returns you to the mark, so `''` toggles between two places.

Marks belong to the workspace they were set in. The same letter in two
workspaces is two independent marks, and jumping to a mark from another
workspace is refused rather than switching workspaces.

A mark whose message has since been deleted still opens its channel and
tells you the message was not found (a deleted thread reply opens its
thread with nothing selected); the mark is kept either way. Marks
are only removed when you remove them, with `:delmarks` or `Backspace`
in the `:marks` overlay.

## Leader map

`[keys.leader_map]` changes what follows the leader (see
[[Keybindings]] → Leader key). Each entry is an action and a list of
sequences: the keys you type after the leader, separated by spaces. Keys
are spelled as for `leader`, plus key names such as `enter` or `tab`; `"f f"`
is f twice, and `"space"` is the space bar.

- An action you set gets exactly your sequences, replacing its default.
  `[]` takes it out of the layer; its `Ctrl` chord still works.
- An action you leave out keeps its default, unless that default clashes
  with a sequence you set: the same keys, or one that begins the other
  (`s` and `s g`). Your sequence wins, and the default is dropped.
- Two sequences you set may not clash, since slk has no timeout to tell
  `f` from `f f`. slk refuses to start and names the action.
- A sequence of several keys makes a group. The status bar and the
  key-hints menu show it as `f +…` after the leader, or `f +find` with a
  label from `[keys.leader_groups]` (below), then what can follow `f`.
- `backspace` cannot be part of a sequence: while one is pending it goes
  back a level.
- A sequence that starts with the leader's own key (`"space"` with
  `leader = "space"`, so `space space`) beats the doubled-leader fallback
  (which would toggle a sidebar section). That action stays on its other
  keys (`Enter` on a section header).

| Action | Default | Does |
|---|---|---|
| `window_commands` | `w` | the window commands (`h`/`j`/`k`/`l`, `s`, `v`, …), as after `Ctrl+w` |
| `channel_finder` | `t` | switch channel (`Ctrl+t`) |
| `workspace_search` | `f` | search workspace (`Ctrl+f`) |
| `toggle_activity` | `a` | toggle the Activity view (`Ctrl+a`) |
| `new_message` | `n` | new message (`Ctrl+n`) |
| `set_status` | `s` | set status (`Ctrl+s`) |
| `switch_theme` | `y` | switch theme for this workspace (`Ctrl+y`) |
| `set_default_theme` | `Y` | set the default theme (`Ctrl+Shift+y`) |
| `toggle_sidebar` | `b` | toggle the sidebar (`Ctrl+b`) |
| `toggle_thread` | `]` | close the thread panel (`Ctrl+]`) |
| `key_hints` | `?` | the key-hints menu of the keys that act in the focused pane (see Key hints) |
| `help` | — | show keybindings (`?`) |
| `close_thread` | — | close the thread panel (`q`) |
| `nav_back` / `nav_forward` | — | back / forward through visited channels (`Ctrl+h` / `Ctrl+l`) |
| `next_unread` / `prev_unread` | — | next / previous unread channel (`a` / `A`) |
| `next_theme` / `prev_theme` | — | next / previous theme (`Alt+y` / `Alt+Shift+y`) |
| `command_mode` | — | the `:` command line |
| `quit` | — | quit, with confirmation (`Q`) |

### Leader groups

`[keys.leader_groups]` names the groups of the layer: each entry is a
sequence after the leader (spelled as in `[keys.leader_map]`) and the
label the status bar, the menu title and the menu row show after a `+`.

```toml
[keys.leader_groups]
f = "find"       # with channel_finder = ["f f"]
"g c" = "chat"   # a group inside a group, with a sequence like "g c x"
w = "panes"      # window_commands' table
```

A group is a sequence some longer sequence begins with (`f` of `f f`),
or `window_commands`' own sequence, whose table is a group too; it is
labelled `windows` unless you set one. A label for anything else (a
sequence that runs an action, or that nothing begins with) stops slk at
startup with an error naming the sequence. So does an empty label, or one
with a control character such as a newline: a label is one line.

### Neovim-style keys

Space as the leader, `l` opening sidebar rows, and telescope-style groups:

```toml
[keys]
leader = "space"
sidebar_open = ["enter", "l"]
[keys.leader_map]
channel_finder = ["f f", "space"]
workspace_search = ["/", "s g"]
key_hints = ["s k"]
toggle_sidebar = ["e"]
help = ["?"]
close_thread = ["q"]
[keys.leader_groups]
f = "find"
s = "search"
```

`space f f` and `space space` open the channel finder, `space /` and
`space s g` search the workspace, `space s k` lists the keys for the
focused pane, `space e` toggles the sidebar, `space ?` shows the
keybindings and `space q` closes the thread. `space s` is now the
`s g` / `s k` group, so `set_status` loses its default `s` (it stays on
`Ctrl+s`), and `help` takes `?` from `key_hints`. After `space`, the menu
lists `f +find`, `s +search` and `w +windows` below the actions.
`space w h/j/k/l` still moves between panes. Section headers toggle with
`Enter` or `l`, since `space space` is the channel finder.

## Key hints

After a pause in a key sequence — the leader, `Ctrl+w`, `g`, `m` (and `'`
with `show_jump_overlay = false`) — a panel at the bottom right lists
what can follow, like neovim's which-key. Its border says what you typed
(`\ w +windows`); plain keys come first, sorted by key, then groups as
`+label`. `m` and `'` list your marks with where they point.

- Typing a listed key carries on as usual: the menu only shows what is
  pending. `Backspace` goes back a level, `Esc` cancels.
- Once it is open, the next level shows at once.
- When a list is longer than the screen it takes several columns, then
  pages: `Ctrl+d` / `Ctrl+u` turn the page (the footer says so).
- `key_hints` in the leader layer (`\ ?` by default) opens the menu at
  once with the keys that act where you are: the sidebar (with your
  `sidebar_open` / `sidebar_toggle_section` keys), the messages, the
  thread, or the Threads and Activity lists. A key there does what it
  does in Normal mode; anything else just closes the menu.

| Setting | Default | |
|---|---|---|
| `[keys] key_hints` | `true` | `false` keeps only the status-bar hint (the `key_hints` action still opens its menu) |
| `[keys] key_hints_delay` | `300` | milliseconds before the menu appears; `0` shows it at once; up to `10000` |

The status bar keeps the one-line hint too: it is what you see while the
delay runs, with the menu off, and in a terminal too small for the panel.

## Data paths (XDG)

| Path | Contents |
|---|---|
| `~/.config/slk/` | Configuration, custom themes |
| `~/.local/share/slk/` | SQLite cache, tokens |
| `~/.cache/slk/` | Avatars, image cache |
