# Keybindings

| Key | Mode | Action |
|---|---|---|
| `j` / `k` | Normal | Move down/up in channel list or messages |
| `h` / `l` | Normal | Switch focus between panels |
| `Tab` / `Shift+Tab` | Normal | Cycle focus |
| `Ctrl+w` `h`/`j`/`k`/`l` (or arrows) | Normal | Focus the pane in that direction: sidebar, split windows, thread |
| `Ctrl+w` `s` / `v` | Normal | Split the window stacked / side by side (`:sp` / `:vsp`) |
| `Ctrl+w` `w` | Normal | Cycle windows |
| `Ctrl+w` `q` / `c` | Normal | Close the window (`:q`) |
| `Ctrl+w` `o` | Normal | Close every other window (`:only`) |
| `\` | Normal | Leader key — see [Leader key](#leader-key) |
| `Enter` | Normal (sidebar) | Open the selected channel, DM, Threads or Activity row, or toggle a section header (`[keys] sidebar_open`) |
| `Space` | Normal (sidebar) | Toggle the selected section header (collapse/expand) (`[keys] sidebar_toggle_section`) |
| `Enter` | Normal (message) | Open thread |
| `i` | Normal | Enter insert mode |
| `Esc` | Insert / Command | Return to normal mode |
| `Enter` | Insert | Send message |
| `Shift+Enter` | Insert | Newline |
| `Ctrl+V` | Insert | Smart paste — image / file path / text (use `Ctrl+V`, not the terminal's `Ctrl+Shift+V`) |
| `Ctrl+U` | Insert | Clear compose (text + pending attachments) |
| `Ctrl+E` | Insert | Edit the draft in `$VISUAL` / `$EDITOR` (or `compose.editor`) |
| `Ctrl+O` | Insert (thread) | Toggle "also send to channel" for the next thread reply |
| `Alt+Enter` | Insert (thread) | Send thread reply and broadcast to channel (one-shot) |
| `Ctrl+U` / `Ctrl+D` | Normal | Half-page up / down |
| `Up` | Insert | Previous line; on the first line, jump to start of message |
| `Down` | Insert | Next line; on the last line, jump to end of message |
| `Esc` | Insert, with `[compose] vim_mode` | To compose normal (the status bar shows `C-NORMAL`); a second `Esc` returns to normal mode |
| `h` `j` `k` `l` `w` `b` `e` `ge` `W` `B` `E` `0` `^` `$` `gg` `G` `f` `F` `t` `T` `;` `,` | Compose normal | Vim motions in the draft; counts work (`3w`) |
| `d` `c` `y` + motion or text object, `dd` `cc` `yy` `D` `C` `Y` `x` `X` `s` `S` | Compose normal | Vim operators; text objects `iw` `aw` `iW` `aW` `i"` `a"` `i'` `a'` `` i` `` `` a` `` `i(` `a(` `i[` `a[` `i{` `a{` `i<` `a<` |
| `p` `P` `r` `~` `J` | Compose normal | Put (internal register, shared by both composers), replace, toggle case, join lines |
| `i` `a` `I` `A` `o` `O` | Compose normal | Back to typing |
| `u` / `Ctrl+r` / `.` | Compose normal | Undo / redo / repeat the last change |
| `v` / `V` | Compose normal | Visual / visual-line selection; `d` `c` `y` `p` `~` `u` `U` `J` `r` `o` act on it, `Esc` ends it |
| `Enter` | Compose normal | Send; with a half-typed command or a selection up, cancels that instead |
| `gg` / `G` | Normal | Jump to top / bottom |
| `/` | Normal | Search in channel (vim-style; searches cached history of the current channel) |
| `n` / `N` | Normal | Next / previous search match (wraps) |
| `a` / `A` | Normal | Jump to next / previous unread channel (wraps) |
| `Ctrl+h` / `Ctrl+l` | Normal | Navigate back / forward through visited channels, returning to the message you were reading when you left (`Ctrl+k` also works) |
| `Esc` | Normal (search active) | Clear active search |
| `m{a-z}` | Normal (message) | Set a session mark on the selected message — lost when slk exits |
| `m{A-Z}` | Normal (message) | Set a persistent mark — survives a restart |
| `'{letter}` / `` `{letter} `` | Normal | Jump to a mark: opens the channel, loads history if needed, opens the thread and selects the reply |
| `''` / ` `` ` | Normal | Back-jump to where you jumped from; press again to return |
| `:marks` | Normal | List every mark in the workspace with its channel and message preview |
| `:delmarks abc` | Normal | Delete marks by letter (`:delmarks a b c` works too) |
| `Backspace` | Marks overlay | Delete the selected mark |
| `Ctrl+f` | Any | Search workspace (Slack server-side; supports modifiers like `from:@user`, `in:#channel`, `before:YYYY-MM-DD`) |
| `Ctrl+b` | Any | Toggle sidebar |
| `Ctrl+]` | Any | Toggle thread panel |
| `Ctrl+a` | Normal | Toggle the Activity view (mentions, thread replies, reactions, DMs) |
| `:activity` | Normal | Open the Activity view |
| `u` | Normal (activity) | Toggle the Activity view's unread-only filter |
| `U` | Normal (activity) | Toggle the selected Activity item read/unread |
| `Ctrl+t` / `Ctrl+p` | Any | Fuzzy channel finder |
| `:ws` | Normal | Workspace picker |
| `1`–`9` | Normal | Jump to workspace N |
| `r` | Normal (message) | Open reaction picker |
| `R` | Normal (message) | Quick-toggle existing reactions |
| `E` | Normal (message) | Edit your own message |
| `D` | Normal (message) | Delete your own message (with confirmation) |
| `U` | Normal (message) | Mark selected message and everything newer as unread |
| `S` | Normal (thread) | Save thread to markdown file (`~/.local/share/slk/exports/` or `$XDG_DATA_HOME/slk/exports/`) |
| `y` | Normal (message) | Copy message text |
| `Y` / `C` | Normal (message) | Copy message permalink |
| `F` | Normal (message / thread) | Forward selected message using the channel finder; Enter sends, Esc cancels |
| `O` / `v` | Normal (message) | Open full-screen image preview |
| `Esc` / `q` | Preview | Close preview |
| `Enter` | Preview | Open in system image viewer |
| `h` / `←` | Preview | Previous image (when message has multiple) |
| `l` / `→` | Preview | Next image (when message has multiple) |
| Click | Any (on image) | Open full-screen preview |
| `Ctrl+y` | Any | Switch theme |
| `Alt+y` / `Alt+Shift+y` | Normal | Next / previous theme (wraps; saved per workspace) |
| `Ctrl+s` | Any | Set status (Active / Away / DND snooze) |
| `q` | Normal | Quit (with confirmation) |
| `Q` | Normal | Quit immediately |
| `Ctrl+c` | Any | Quit (with confirmation) |

## Leader key

In Normal mode the leader key (default `\`, set with `[keys] leader` — see
[[Configuration]]) starts a sequence. The status bar lists the keys that can
follow. `Esc` cancels; any other key cancels too and does nothing else. There
is no timeout.

The leader stands in for `Ctrl`: each entry is the letter of the chord it
mirrors, so `\ t` is `Ctrl+t`.

| Keys | Action | Same as |
|---|---|---|
| `\ w` `h`/`j`/`k`/`l` | Focus the pane in that direction | `Ctrl+w h/j/k/l` |
| `\ w` `s` / `v` / `w` / `q` / `c` / `o` | Window commands | `Ctrl+w …` |
| `\ t` | Fuzzy channel finder | `Ctrl+t` / `Ctrl+p` |
| `\ f` | Search workspace | `Ctrl+f` |
| `\ a` | Toggle the Activity view | `Ctrl+a` |
| `\ n` | New message | `Ctrl+n` |
| `\ s` | Set status | `Ctrl+s` |
| `\ y` | Switch theme (this workspace) | `Ctrl+y` |
| `\ Y` | Set the default theme | `Ctrl+Shift+y` |
| `\ b` | Toggle sidebar | `Ctrl+b` |
| `\ ]` | Close the thread panel | `Ctrl+]` |

The leader only works in Normal mode; in Insert mode, prompts and dialogs it
is an ordinary key.

If the leader you configure already does something (`space` toggles a
sidebar section), pressing it twice does that: `space space` toggles the
section (`Enter` on a section header still does too). If the leader is one
of the letters above (say `b`), the doubled leader is the old action and
that letter's entry leaves the layer; its `Ctrl` chord still works. The
help overlay (`?`) lists the leader layer under your leader key.

Directional focus follows what is on screen. The workspace rail is not a
pane. When the terminal is too narrow to show messages and thread side by
side, the thread still counts as being to the right of the messages, so `l`
brings it to the front and `h` goes back; from the sidebar, `l` enters
whichever of the two is in front. Moving to another split window closes an
open thread, as switching windows always has.

## Sidebar keys

`Enter` and `Space` in the sidebar can be changed in `[keys]` (see
[[Configuration]]): `sidebar_open` (default `["enter"]`) and
`sidebar_toggle_section` (default `["space"]`); an empty list unbinds one.
These keys only work while the sidebar has focus, and there they take over
from the key's usual job. A neovim tree-picker setup,
`sidebar_open = ["enter", "l"]`, makes `l` open a channel or view, or
expand / collapse a section header, in the sidebar, while `l` still moves
focus right everywhere else. To leave the sidebar then, use `Tab`,
`Ctrl+w l` / `\ w l`, or open a channel. The help overlay (`?`) shows the
keys you configured.

Other custom keybinding overrides are on the roadmap — see [[Tradeoffs and Non-Goals|Tradeoffs-and-Non-Goals]].
