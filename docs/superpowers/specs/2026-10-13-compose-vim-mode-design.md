# Vim editing in the composer — design

**Date:** 2026-10-13
**Status:** Implemented on `feat/compose-vim-mode`; awaiting review.

## 1. Problem

slk is modal at the application level (`NORMAL` / `INSERT` / `COMMAND` …),
but the composer itself is a plain `charm.land/bubbles/v2/textarea`: once
you are typing, the only editing keys are the textarea's emacs-style ones
(`ctrl+a`, `alt+f`, `ctrl+k`, …). A vim user who wants to fix a typo three
words back, change the word inside quotes, or delete a line of a multi-line
draft has to leave for `$EDITOR` with `ctrl+e`, which suspends the TUI.

This adds native vim editing — NORMAL / INSERT / VISUAL sub-modes, counts,
motions, operators, text objects, registers, undo and `.` — inside both
composers, opt-in by config.

## 2. Current behaviour

- Both composers are `internal/ui/compose.Model` (`a.compose`,
  `a.threadCompose`), wrapping `textarea.Model`. `compose.Model.Update`
  routes keys to the emoji / mention / channel pickers first, then to the
  textarea, then re-checks the picker triggers (`@`, `#`, `:xy`).
- App-level `ModeInsert` is entered from Normal mode by `i`
  (`handleNormalMode`, `mode_normal.go`), `E` (`beginEditOfSelected`,
  `app.go`), the action-dialog restore (`afterConfirmPrompt`) and
  `/msg` / new-message (`reducer_new_message.go`). Each calls
  `SetMode(ModeInsert)` and the composer's `Focus()`.
- `handleInsertMode` (`mode_insert.go`) owns every key while composing: the
  upload guard, Esc (close a picker, else cancel an edit / leave insert),
  `ctrl+v` smart paste, `ctrl+e` `$EDITOR`, `ctrl+u` clear, `ctrl+o`
  broadcast, Up/Down jump on the first/last line, Enter send / commit
  edit / slash command, `shift+enter` / `ctrl+j` newline, and forwards the
  rest to the composer (plus the typing indicator).
- The status bar's mode segment is `statusbar.Model.SetMode(fmt.Stringer)`,
  fed `ui.Mode` by `App.SetMode`; `statusbar.View` styles `"INSERT"` and
  `"COMMAND"` specially.
- `[compose]` in `internal/config` has one key, `editor`, wired in
  `cmd/slk/main.go` via `app.SetComposeEditor`. Display settings shared by
  `run()` and the hidden `--demo` mode go through `uiSettings` /
  `applyUISettings` (`cmd/slk/ui_settings.go`).

## 3. Design

### 3.1 Opt-in

`[compose] vim_mode = true` (default `false`). `run()` passes it through
`uiSettings.ComposeVimMode` → `applyUISettings` →
`App.SetComposeVimMode(on)`, which turns the engine on in both composers.
With it off nothing changes: no new code runs on any key path (every new
branch is behind `compose.Model.vim != nil`).

The demo has no config; `SLK_DEMO_COMPOSE_VIM=1` turns it on there, next to
the existing `SLK_DEMO_IMAGES` (useful for a future GIF, and the tmux
smoke).

### 3.2 Shape

- **`internal/bubbles/vimedit`** — a pure engine: a state machine over a
  `Buffer{Text, Cursor}` (cursor is a byte offset) and a `Key{Text, Name}`.
  No bubbletea, no textarea, no I/O; imports only the standard library and
  `rivo/uniseg`. It owns the sub-mode, the pending command (counts,
  operator, prefix), the visual anchor, the desired column for `j`/`k`,
  the last `f`/`t`, undo/redo history, the open insert session and the
  last repeatable change. Exhaustively unit-tested on strings.
- **`compose.Model` adapter** (`internal/ui/compose/vim.go`) — translates
  `tea.KeyMsg` into `vimedit.Key`, hands the engine the textarea's value and
  cursor, and writes the result back. When the text changed it rebuilds the
  textarea around the cursor (the text after it, then the text before it
  inserted at the start), which lands the cursor exactly in linear time;
  a pure move walks the cursor with the textarea's public API, a page at a
  time over long distances (each step re-scrolls the viewport, which costs
  every row above the cursor, so a row-by-row walk is quadratic). Paints
  the visual selection. In INSERT the adapter steps aside entirely: typing,
  pickers, paste and attachments run through the existing code unchanged.
  A blurred composer takes no keys, with vim mode on as with it off.
- **App glue** (`internal/ui/compose_vim.go`, `mode_insert.go`) — routes
  Esc and Enter, keeps app-level keys working, labels the status bar.

App-level mode stays `ModeInsert` for the whole composing session: the vim
sub-modes are the composer's state, not new `ui.Mode` values, so the
composer owns the keyboard until it is left and no app Normal-mode key can
fire from compose NORMAL.

Third-party vim textareas for Go were considered and not taken: none plug
into `bubbles/v2/textarea` (they replace it, losing the pickers, drafts,
attachments and paste handling built on it), and adding a module needs
network the build environment does not have.

### 3.3 Sub-mode transitions

| From | Key | To |
|---|---|---|
| app Normal | `i` (and every other way into `ModeInsert`) | INSERT |
| INSERT | `Esc`, no picker open | NORMAL (cursor steps left, as in vim) |
| INSERT | `Esc`, a picker open | INSERT (the picker closes, as today) |
| NORMAL | `i a I A o O s S C`, `c{motion}` | INSERT |
| NORMAL | `v` / `V` | VISUAL / V-LINE |
| VISUAL / V-LINE | `Esc`, `v`/`V` again, an operator | NORMAL (INSERT for `c`) |
| NORMAL with a partial command (`2`, `d`, `f`, …) | `Esc` | NORMAL, partial command dropped |
| idle NORMAL | `Esc` | leaves the composer: app Normal, exactly as Esc does today |

During an edit (`E`) the same table holds, except that the final Esc from
idle NORMAL cancels the edit (today's Esc behaviour), and Enter commits it.

### 3.4 Enter

Enter **sends** from idle NORMAL, exactly as from INSERT (slash commands,
attachments, edit commit and `alt+enter` broadcast included). A chat
composer's Enter means "send" in every client and in slk's own INSERT; vim's
NORMAL-mode Enter (down a line, to its first non-blank) is redundant with
`j^`. A user leaving INSERT to fix a typo and pressing Enter expects the
message to go.

With a partial command pending (a count, an operator, `f`/`t`/`r`/`g`
waiting for its argument) or a visual selection active, Enter **cancels
that** instead of sending — exactly like Esc. Sending in the middle of
`d2…` or with a selection up is almost certainly an accident, and a send
cannot be taken back.

`shift+enter` / `ctrl+j` (newline in INSERT) do nothing in NORMAL/VISUAL;
`o` / `O` open lines.

### 3.5 App keys while in NORMAL / VISUAL

Kept, same meaning as in INSERT: `ctrl+v` smart paste, `ctrl+e` `$EDITOR`,
`ctrl+u` clear the composer, `ctrl+o` broadcast toggle (thread), Enter /
`alt+enter` (§3.4), the upload guard, `ctrl+c` (global). The status bar's
`ctrl+…` app keys that today do not work while typing still do not.

Changed only in NORMAL/VISUAL: Up/Down are `k`/`j` (the first/last-line
"jump to start/end" shortcut stays an INSERT feature), Backspace is `h`
(removing the last attachment with Backspace on an empty composer stays an
INSERT feature), and no typing indicator is sent for NORMAL-mode keys.
Every other key is the composer's: unbound keys are swallowed, never
reaching the textarea or the app.

### 3.6 Status bar and cursor

The mode segment shows `INSERT` (unchanged), `C-NORMAL`, `VISUAL` or
`V-LINE`. `C-NORMAL` is not `NORMAL` so the user can tell compose NORMAL
(Esc leaves the composer) from app NORMAL. `C-NORMAL` uses the NORMAL style;
`VISUAL` / `V-LINE` use the `COMMAND` (warning) style, the way vim themes
colour visual mode differently. `App.syncComposeModeLabel` sets it after
every insert-mode key and wherever a selection can end without one (a
bracketed paste, `$EDITOR` returning, a draft switch);
`SetMode(ModeInsert)` already shows `INSERT`, which is the sub-mode every
entry lands in.

Cursor shape: the composer draws the textarea's *virtual* cursor (a
reverse-video cell; the App never positions the terminal cursor), so it is a
block in every sub-mode, and a bar in INSERT is not available without
plumbing the textarea's real cursor through `App.View` (out of scope, §6).
NORMAL's cursor sits on a character, INSERT's between them, so the block's
position already differs as in vim's block-cursor terminals.

The visual selection is painted with `styles.SelectionStyle()` over the
textarea's rendered rows. The rows a byte range occupies come from the
textarea's own soft wrap: a scratch textarea of the same width reports each
row's start through its public `LineInfo()`, so the painter cannot drift
from the textarea's wrapping. Only lines that can wrap pay for that: a
line narrower than the wrap width is one row (the textarea's wrap only
starts a row once a row plus its pending word reaches the width), a
shortcut a test holds in step with the textarea; the walk stops below the
last visible row. `lipgloss.StyleRanges` applies the style by cell,
ANSI-safe; the cursor cell is left as the textarea drew it. An empty line
inside the selection shows one painted cell, the last line of a `V`
selection and an empty last line of the draft included.

### 3.7 Text model: graphemes

The engine splits the buffer into grapheme clusters (`uniseg`) and works on
cluster indices; every cursor it returns is a cluster boundary, so no motion
or operator splits an emoji (ZWJ sequences, flags, skin tones) or a
combining sequence, and nothing slices a multi-byte rune. A newline is a
cluster of its own and doubles as vim's "end of line" position. A cursor the
engine receives mid-cluster (the textarea is rune-based, so INSERT can leave
it between the two runes of a flag) snaps to the cluster start.

Display columns (`j`/`k` keep the column the cursor is aimed at) are cluster
widths from `uniseg`, the same function the textarea wraps with, so `j` over
CJK or emoji lands visually under the cursor.

Word classes follow vim's `utf_class`: blanks; punctuation; word characters
(letters, digits, `_`); emoji; and CJK ideographs, Hiragana, Katakana and
Hangul as classes of their own — so `w` stops between `foo` and `日本語`,
and `W` treats any non-blank run as one WORD.

### 3.8 Commands

Counts multiply (`2d3w` = 6 words) and are capped at 999.

| Group | Keys |
|---|---|
| Left/right | `h` `l` (`←` `→` `Backspace` `Space`), `0` (`Home`) `^` `$` (`End`) |
| Up/down | `j` `k` (`↓` `↑`) — logical lines, keeping the aimed-at display column; `gg` `G` (`{count}G` / `{count}gg` go to a line) |
| Words | `w` `b` `e` `ge`, `W` `B` `E` `gE` — ported from vim's `fwd_word` / `bck_word` / `end_word` / `bckend_word`, including `cw` = `ce` and `dw` stopping at end of line |
| Find | `f` `F` `t` `T` `{char}` (the char may be any grapheme, emoji included), `;` `,` |
| Insert | `i` `a` `I` `A` `o` `O` (a count repeats the text on Esc: `3ihi<Esc>`) |
| Operators | `d` `c` `y` + any motion or text object; `dd` `cc` `yy`; `D` `C` `Y` (`Y` = `yy`, vim's default); `x` `X` `s` `S` |
| Text objects | `iw` `aw` `iW` `aW`, `i"` `a"` `i'` `a'` `` i` `` `` a` ``, `i(` `a(` `ib` `ab` (also `)`), `i[` `a[` (`]`), `i{` `a{` `iB` `aB` (`}`), `i<` `a<` (`>`) |
| Edit | `r{char}`, `~`, `J`, `p` `P` |
| History | `u`, `ctrl+r`, `.` |
| Visual | `v` `V`; motions and text objects extend; `o` swaps ends; `d` `x` `X` `D` `c` `s` `C` `S` `R` `y` `Y` `p` `P` `~` `u` `U` `J` `r{char}` |

Vim rules kept because users' fingers rely on them: an exclusive motion that
ends at column 0 of a later line stops at the end of the previous line, and
becomes linewise when it started in the indent (`:help exclusive-linewise`);
`i{` on a block whose braces sit on their own lines operates on the inner
lines linewise (`di{` leaves `{` / `}`, `ci{` leaves an empty line to type
on), and when only the open brace ends its line the object starts on the
next line (`di{` on `{⏎  foo }` leaves `{⏎}`); `;` after `t` does not get
stuck on the adjacent match, unless a count is given; `$` in VISUAL takes
in the line break (`v$d` joins the line with the next, `v$y` yanks it),
also on the lines `j`/`k` reach after it.

**Register.** One unnamed register, charwise or linewise, shared by both
composers (vim's registers are global): yank in a thread, put in the
channel. It is internal; nothing touches the system clipboard (`internal/ui`
does no I/O). `App.clipboardWrite` exists, so a follow-up could mirror yanks
to the clipboard behind a setting (§6).

**Undo.** Snapshots of text and cursor, one per NORMAL command and one per
insert session (from the command that opened it, deletion of `c` included,
to the Esc that closed it), 100 deep. `u` / `ctrl+r` restore the snapshot
and put the cursor where the change began. History is the conversation's:
it is cleared when the composer's text is replaced from outside the editor
(draft switch, send / clear, starting or cancelling an `E` edit, `$EDITOR`
returning, `ctrl+v` text paste), so `u` can never resurrect another
conversation's or a sent message's text. A bracketed paste in NORMAL is one
undoable step.

**`.`** repeats the last change with its count (a new count replaces it):
operator commands, `x` `X` `s` `S` `r` `~` `J` `p` `P` `D` `C`, and the
insert commands with the text their session added. The inserted text is
read back from the buffer when the session ends, so it is only repeatable
when the session was a plain insertion at the point it started (typing,
pickers, paste); a session that also backspaced over older text or moved
elsewhere to type leaves `.` with nothing to repeat. Visual-mode changes
are not repeatable (`.` after one does nothing).

### 3.9 Integration points

- `compose.Model.Focus` → `EnterInsert` (a fresh insert session unless one
  is already open); `Blur` → `Blur` (closes the session, drops pending and
  visual state). Every way into `ModeInsert` calls `Focus`, so every entry
  lands in INSERT.
- `SetValue`, `Reset`, `SetDraftContext`, `OfferDraft` → `Reset` (history
  cleared, cursor normalised for NORMAL).
- `Update`: in NORMAL/VISUAL every key goes to the engine; a bracketed paste
  is checkpointed then inserted by the textarea; in INSERT nothing changes.
- New methods: `SetVimMode(*vimedit.Register)`, `VimMode()`, `VimNormal()`,
  `VimBusy()`, `VimEscape()` (all report vim off for a blurred composer).
- The help overlay lists three vim entries while vim mode is on
  (`KeyMap.setComposeVimHelp`), none when it is off.
- `handleInsertMode`: Esc asks `VimEscape()` after the picker checks;
  Enter asks `VimBusy()`; newline keys are ignored in NORMAL; Up/Down jumps
  and the typing indicator only in INSERT; the mode label is synced on the
  way out.

## 4. Edge cases

- Empty buffer: every motion is a no-op, `x`/`dd` delete nothing, `p` of a
  linewise register yields a blank line plus the text (vim), `u` with no
  history does nothing.
- End of line: NORMAL never rests on the newline of a non-empty line; `a`/`A`
  may place INSERT's cursor there.
- Multi-line: `j`/`k`/`dd`/`J`/`o` operate on logical lines (§6 on wrapped
  rows).
- Emoji / CJK: cluster-exact, display-width-aware (§3.7).
- The textarea's own limits still apply after a put (40 000-char limit,
  1 000-line cap): it truncates, and the adapter re-reads its value and
  clamps the cursor, so an oversized put cannot desync the two.
- Pickers only open in INSERT; any text change the engine makes closes them,
  so no stale picker offset survives an edit (`closePickers`).
- `ctrl+e` while in NORMAL: the edited text comes back through `SetValue`
  (history reset); the composer is still focused, so the sub-mode stays.

## 5. Key table (vim_mode on)

| Key | Where | Action |
|---|---|---|
| `Esc` | INSERT | to NORMAL (a picker open: closes it first) |
| `Esc` | NORMAL | leave the composer (cancels an edit); with a partial command, drops it |
| `Esc` | VISUAL / V-LINE | to NORMAL |
| `Enter` / `alt+enter` | NORMAL | send / commit edit; with a partial command or selection, cancel it |
| `i a I A o O` | NORMAL | to INSERT |
| `v` / `V` | NORMAL | VISUAL / V-LINE |
| `h j k l w b e ge W B E gE 0 ^ $ gg G f F t T ; ,` | NORMAL, VISUAL | motions (counts allowed) |
| `d c y` + motion / text object, `dd cc yy D C Y x X s S` | NORMAL | operators |
| `p P r ~ J` | NORMAL | put, replace, toggle case, join |
| `u` / `ctrl+r` / `.` | NORMAL | undo / redo / repeat |
| `d x X D c s C S R y Y p P ~ u U J r o` | VISUAL | act on the selection; `o` swaps ends |
| `ctrl+v ctrl+e ctrl+u ctrl+o` | any sub-mode | as in INSERT today |

## 6. Out of scope

- **Cursor bar in INSERT** — needs the terminal cursor, which slk does not
  use (virtual cursor; §3.6).
- **`gj` / `gk` (soft-wrapped rows)** — `j`/`k` move by logical lines as in
  vim with `wrap`; screen-row motions would need the textarea's wrap on every
  key.
- **System clipboard** — register stays internal; a `"+` / `clipboard=`
  style integration through `App.clipboardWrite` is a possible follow-up.
- **Named registers, macros (`q`), marks, `/` `?` `*` `#` `n` `%`, `R`
  replace mode, visual block (`ctrl+v` is smart paste), `gu`/`gU`/`g~`
  operators, `>`/`<`, counts on text objects (`d2iw`), `:` commands** —
  not needed for composing chat messages; each is additive later.
- **Insert-mode vim keys** (`ctrl+w`, `ctrl+o`, `ctrl+r {reg}`) — INSERT
  keeps today's textarea bindings so pickers, paste and broadcast are
  untouched.
- **Configurable labels / per-mode cursor colours.**

## 7. Tests

- `internal/bubbles/vimedit`: table tests driving key strings over
  `text|marker` buffers — every motion, operator, text object, counts,
  `cw`/`dw` and exclusive-linewise rules, visual modes, register kinds,
  `p`/`P`, undo/redo, `.` (including insert repeats and visual
  non-repeatability), sessions and `Reset`, empty buffer, end of line,
  multi-line, emoji/ZWJ/flags, CJK columns and word classes, counts cap.
- `internal/ui/compose`: adapter round trips (cursor written back exactly,
  multi-line and wrapped text, viewport follows), key translation, paste in
  NORMAL, selection painting, picker isolation, draft reset.
- `internal/ui`: vim off = unchanged (Esc leaves, label `INSERT`); Esc
  INSERT→NORMAL→app Normal with labels; Enter sends from NORMAL, cancels a
  partial command or selection; pickers (Esc closes the picker, not INSERT);
  the thread composer; the `E` edit flow (Esc→NORMAL, Enter commits, Esc
  cancels); app Normal keys do not fire from compose NORMAL; register shared
  across composers.
- `internal/config`, `cmd/slk`: the key loads, defaults off, reaches the
  App; the demo switch.
