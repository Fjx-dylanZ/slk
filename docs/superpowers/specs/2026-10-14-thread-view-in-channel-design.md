# View in channel — design

**Date:** 2026-10-14
**Status:** Implemented
**Builds on:** `2026-05-09-channel-history-and-recency-design.md` (the
`Ctrl+h` / `Ctrl+l` history), `2026-10-13-leader-keys-design.md` (the
leader layer, `[keys.leader_map]`), `2026-10-14-adaptive-key-hints-design.md`
(`keyScopes`, `topHints`, the `key_hints` context menu).

## Problem

A thread opened from the Activity view (Enter on a reply or mention in a
thread), from the Threads view, or from a permalink or mark in another
channel shows the thread pane with the list (or another channel) behind
it. Nothing goes from there to the conversation the thread lives in with
the parent message in view: Slack's "View in channel". Today the user has
to find the channel in the sidebar or finder and scroll to the parent by
hand, which closes the thread they were reading.

## Current behavior (references)

- Activity: `ActivitySelectedMsg` with `ThreadTS` calls
  `openThreadForPermalink(channel, threadTS)`; the Activity list stays in
  the messages slot, the thread pane is focused. Threads view: Enter runs
  `openSelectedThreadCmd` and focuses the thread pane. A thread-bearing
  permalink or mark goes through `applyLocation` and also ends focused on
  the thread pane, in the channel view.
- `applyLocation(loc, fromHistory)` (`reducer_links.go`) is the one
  in-app navigation: it refuses while an upload is in flight, resolves the
  channel, records `pendingLinkNav`, and either completes in place (the
  channel is active) or dispatches `ChannelSelectedMsg`.
  `completePendingLinkNav` selects `MessageTS` in the channel pane, or
  `FetchAround`s it on the authoritative pass; a `ThreadTS` opens the
  thread panel focused instead. Every channel switch closes the thread.
- History: `navHistoryStore` holds per-workspace channel visits
  (`Location`s). `ChannelSelectedMsg` pushes the arriving channel and
  folds the departing `currentPosition()` into the entry being left
  (`UpdateCurrent`, a no-op unless the position names that entry's
  channel). Views (Activity, Threads) are not visits and push nothing.
- `''` is the back-jump slot: written by mark jumps and by itself only.

## Design

### 1. The action

`(*App).viewThreadInChannel()` (`mode_normal.go`), shared by the key, the
leader action and the click:

1. No thread open: toast `No thread open`, nothing else.
2. Target: the thread's own channel (`threadPanel.ChannelID()`, the
   active channel when empty, as `currentLocation` does) and its parent
   (`threadPanel.ThreadTS()`), in the active workspace.
3. Navigate there (§2), with the thread kept open beside the channel.
4. The channel cannot be resolved (left, archived): toast
   `Channel for this thread is unavailable`; the thread stays as it was.

The thread is identified by the pane, not by a selection, so it works the
same whichever reply is selected and whichever view the thread was
opened from.

### 2. One navigation path

`applyLocation`'s body becomes `navigate(p *pendingLinkNav, fromHistory)`;
`applyLocation` builds `&pendingLinkNav{Location: loc}` and calls it, so
every existing caller is unchanged. View in channel calls
`navigate(&pendingLinkNav{Location: {channel, MessageTS: parent},
besideThread: true}, false)`. `besideThread` is load state on the pending
navigation, not a `Location` field: a `Location` stays the shape of a
permalink, and nothing records this shape (history and marks record
positions, not panes).

What `besideThread` changes: the thread pane is **kept**, not closed and
reopened. `pendingLinkNav.keepsOpenThread` is true while the pane shows
the navigation's thread, and in that case:

- **In place** (the thread's channel is the active one): the channel view
  is shown and focus moves to the channel pane as for any in-place jump,
  but the thread is not closed. The parent is selected; if the channel's
  fetch is still running, the target stays pending and the
  `MessagesLoadedMsg` arm re-selects it.
- **Another channel**: `ChannelSelectedMsg` switches as usual (history
  push, view, focus, compose retarget), except that
  `reduceChannelSelected` does not close this thread. "A channel switch
  closes the thread" exists because the old channel's thread does not
  belong beside the new one. This thread does belong there, since the
  switch goes to its channel. `completePendingLinkNav` then selects the
  parent as for any jump.
- **Parent not in the buffer**: unchanged pipeline. The authoritative
  pass `FetchAround`s the parent, and `MessagesAroundLoadedMsg` either
  selects it or toasts `Message not found in loaded history` and keeps
  the position.

Keeping the pane rather than reopening it keeps the reply the user was
reading, the thread's scroll and its draft. It costs no refetch. It also
can't come back as a stub: a reopen takes the parent row from the new
buffer or the thread cache, and with both cold (the demo, a never-opened
channel) the parent row comes up blank until a cache read fills it.

Inherited unchanged: the `Upload in progress` refusal, the workspace guard
(`TeamID` on the pending navigation and on `ChannelSelectedMsg`), the
stale guards (a pending navigation for another channel is dropped; a late
`MessagesAroundLoadedMsg` for a channel left since is ignored).

### 3. Focus: the channel pane, on the parent

After the action, focus is on the channel pane with the parent selected,
and the thread pane is open beside it, not focused. Reasons:

- The point of the action is the parent in its context: with focus there,
  `j`/`k` read the conversation around it at once.
- In the stacked layout (narrow terminals) focus decides which pane is
  drawn; focusing the thread would hide the channel the user asked for.
- Everything about the thread still works as usual: `Tab` / `l` /
  `Ctrl+w l` return to it, `Esc`, `q` and `Ctrl+]` close it.
- It is what the same-channel case naturally is: an in-place jump focuses
  the channel pane.

### 4. History and `''`

The history can't represent views. Its entries are channel visits
(`Location`), and neither the Activity nor the Threads view is a visit.
Making them visits would change what `Ctrl+h` does after every `Ctrl+a`,
not just after this action (see Out of scope). So "back" is the channel
history, as for every other navigation:

- **Another channel**: a new visit. The thread's channel is pushed, and
  the departure folded into the entry being left is the active channel's
  messages-pane position, i.e. the channel behind the Activity / Threads
  list, or the channel the user was in (see §8.1 for why that needed a
  fix). `Ctrl+h` returns there, to the message the user was on, with the
  channel view shown. `Ctrl+l` comes back to the thread's channel at the
  position it was left (the parent; or, if the thread pane was focused
  when leaving, the reply, with the thread reopened).
- **Same channel**: no history entry, like a same-channel permalink or
  mark. `Ctrl+h` goes to the visit before.
- **Back to the list**: `Ctrl+a` reopens Activity and the Threads row
  reopens Threads. Both keep their selection, so Enter reopens the thread.
- **`''`** is unchanged. View in channel is not a mark jump, so it neither
  writes nor reads the slot, the same as permalinks, search hits, Activity
  items and history walks.

### 5. Keys

**`gp`**, "go to parent": the second key of the `g` chord, like `gg`. It
works in the thread pane, the one place it is listed. Elsewhere `p` after
`g` behaves as before: it cancels the chord and is handled as typed alone.
That keeps any `p` a user gave the sidebar (`[keys] sidebar_open`) working
after a stray `g`. Checked against the existing keys: `p` is not a
Normal-mode binding, nothing in the default leader layer is affected (the
pending `g` is handled before the leader), and the sidebar's configurable
keys are scoped to the sidebar.

- `KeyMap.ViewInChannel`: keys `p`, help `gp` / "view thread in channel"
  (the same arrangement as `Top`: keys `g`, help `gg`). It shows in the help
  overlay from the KeyMap.
- `keyScopes` row: `where: inThread, when: threadOpen`, kind `chordTail`,
  a new kind for a key that only follows a chord's first key. The context
  menu never lists a `chordTail` row at its top level, where `p` typed
  alone would do nothing. `topHints` (the menu after `g`) lists `Top` and
  every `chordTail` row that applies in the current context, so the menu
  and the dispatch read one table. `\ ?` in the thread pane shows
  `g +go to`, and `g` there lists `g top` and `p view thread in channel`.
  The `g` group's label becomes "go to" everywhere (it was the `Top`
  description, "top").
- A chord tail's key belongs to the chord, so a leader on that key takes
  nothing from it: `chordTails` joins `notDisplaceable` and `helpKeyMap`
  skips it. Without that, `leader = "p"` would drop the `gp` help row and
  offer a `<leader><leader>` that does nothing.

**Leader action `view_in_channel`**, hint "in channel", no default
sequence. The default layer mirrors ctrl chords and this action has none.
Like `close_thread`, it acts on the open thread from any pane: leader
actions are global commands (`leaderActions`). Its help row sorts next to
`gp`. Mapping it is one line, e.g. `view_in_channel = ["g p"]`.

### 6. Mouse

The thread header is a breadcrumb, `# general › Thread from alice · 2
replies`. A click on its channel segment (glyph and name, not ` › `) runs
the action. `renderBreadcrumb` also returns the width of the channel
segment as drawn, which is 0 when the width dropped it. `thread.Model`
keeps that width with the chrome cache, and
`(*thread.Model).BreadcrumbChannelAt(x, y)` hit-tests in `ClickAt`'s
pane-local coordinates. `reduceMouseClick`'s thread branch checks it
first, ahead of the reaction and image hit tests. The breadcrumb looks
the same as before.

### 7. Feedback

- A successful navigation does not toast; the switch is the feedback.
  The messages-pane header and the status bar name the channel the way
  `ChannelSelectedMsg` always does (the `ChannelService.Lookup` name: the
  sidebar's human name for a DM or group DM). The kept thread's
  breadcrumb already names the same conversation (`applyThreadBreadcrumb`
  when it opened): `● Priya Shah › Thread …`.
- Refusals toast: `No thread open`, `Upload in progress`,
  `Channel for this thread is unavailable`. A parent that cannot be found
  gets the existing `Message not found in loaded history`.

### 8. Two fixes on this path

1. **`currentPosition` with another channel's thread focused.** History
   records the active channel's position, but a focused thread opened
   from Activity or Threads can belong to any channel. Its location was
   handed to `UpdateCurrent`, which dropped it because the channels
   differ, so the active channel's entry kept a stale or channel-only
   position. `Ctrl+h` after view in channel then landed on that channel's
   newest message, not where the user was. Now, when the focused thread
   is another channel's, `currentPosition` reads the messages pane behind
   it. Every history departure from such a thread benefits (channel
   switches, mark jumps and permalinks into another channel, walks); the
   `''` slot, which records what is on screen, is unaffected. A thread of
   the active channel still records its selected row. This is a separate
   `fix(ui)` commit.
2. **The Activity row's active marker.** `completePendingLinkNav` showed
   the channel view and cleared the Threads row's marker but not the
   Activity row's. An in-place completion from the Activity view (this
   action, Enter on an Activity item in the active channel, a mark jump)
   therefore left `◉ Activity` marked active over the channel view. It now
   clears both, as `reduceChannelSelected` does. This is a separate
   `fix(ui)` commit.

## Key table

| Keys | Where | Does |
|---|---|---|
| `gp` | thread pane | view the thread's parent in its channel, thread kept open |
| click `# channel` in the thread header | thread pane | same |
| `view_in_channel` (`[keys.leader_map]`) | anywhere a thread is open | same; no default sequence |

## Tests

Through the real `Update` chain (keys `g`, `p` sent to `App.Update`,
commands fed back by `feed`), on `viewInChannelApp`, a fixture with
channel and thread services that record thread fetches and FetchAround
calls:

- Activity → thread (another channel) → `gp`: channel view, active
  channel is the thread's, parent selected, focus on the channel pane,
  the same thread kept (the reply being read still selected, no thread
  fetch), Activity row not marked, history `[C1 C2]`.
- Threads view → thread → `gp`: same end state.
- Channel view, after a permalink into another channel's thread (the
  permalink switched to that channel, thread focused): `gp` completes in
  place: no reload, no thread fetch, the parent is selected, history
  unchanged by `gp`.
- Channel view showing another channel's thread (Activity → thread, then
  `Ctrl+a` back to the channel view): switches to the thread's channel.
- Parent not in the buffer: `FetchAround` for the parent, selected after.
- DM target: the header and breadcrumb show the DM's human name and `●`.
- Upload in flight: refused with the toast, nothing changes.
- Unresolvable channel: toast, thread unchanged.
- No thread open: the leader action toasts; `gp` outside the thread pane
  does nothing.
- History: `Ctrl+h` returns to the channel behind the Activity view at the
  message the user had selected; `Ctrl+l` returns to the parent.
- Key hints: `topHints` lists `p` in the thread pane only; the context
  menu in the thread pane has `g +go to` and no top-level `p`; the help
  overlay lists `gp`; `leader = "p"` keeps the `gp` row.
- Leader action through `TestLeaderMapExtraActions`.
- Click on the breadcrumb's channel name runs it; a click on ` › Thread`
  does not; `renderBreadcrumb`'s channel width per truncation case.

Smoke (tmux, `slk --demo`): the demo has no Activity feed (its
`ActivityService` is the no-op), so the smoke goes through the Threads
view and a channel-view thread; Activity → thread is covered by the
`Update`-chain tests above.

## Out of scope

- **View-aware history** (`Ctrl+h` reopening the Activity or Threads view
  with the thread open). It needs view entries in the stack, and so a new
  rule for which view switches are visits, a push for `Ctrl+a` and the
  Threads row, and an applier for views. That changes back/forward for
  every view switch, so it gets its own design.
- **Recording "thread open beside the channel" in history or marks.** A
  departure records the focused pane's position, as before.
- **`gp` from the Threads / Activity list or the channel pane.** The
  leader action covers those (it acts on the open thread from anywhere).
  Extending `gp` would mean listing `g` in the Activity list, where `gg`
  does nothing.
- **Hover styling for the clickable breadcrumb name.** slk has no hover
  machinery, and an always-on underline would change every thread frame.
- **A demo Activity feed** for the smoke. It belongs to the demo, not to
  this feature.
