# Phase 4 — Extract the modal chrome substrate — Design

> **Status:** design approved in brainstorming 2026-10-12; spec awaiting review.
> **Tracking doc:** [`../plans/2026-09-06-architecture-refactor.md`](../plans/2026-09-06-architecture-refactor.md), Phase 4 (addresses F6 and F7 cause #1).
> **Measured at:** `cf9742c6` (main, after #305 merged).
> **Related:** RFC #236 (self-contained components in `internal/bubbles/`);
> #247 (slash commands), whose `/` autocomplete picker is the next consumer of
> this package.

## 1. Goal

`internal/ui` has a dozen dialogs that each re-implement the same chrome: the
bordered box, the filter line, the selected-row marker, the list window and
the scrollbar. Phase 4 moves that chrome into one package,
`internal/bubbles/chrome`, and migrates every dialog and composer popup onto
it. What each dialog *does* (matching, row content, `Open`, `HandleKey`) stays
in the dialog: *extract the substrate, not the widget*.

Success is behaviour-preserving and checkable:

- one box renderer (`chrome.Box`, with `chrome.Frame` as the list-dialog
  layout on top of it), replacing 10 `renderBox` implementations,
  confirmprompt's box, userprofile's box and `CustomSnoozeView`'s box;
- one list window (`chrome.Window`), replacing 7 `visibleWindow`
  implementations;
- zero hand-rolled scrollbars (6 today); one thumb formula, in
  `bubbles/scrollbar`;
- every dialog package asserts a `chrome` interface at compile time;
- every App golden **cell-identical** to the baseline taken after §6 step 0.

### 1.1 Non-goals

- Changing what any dialog looks like or does. Width clamps, row caps,
  matchers and empty-state wording are kept exactly, however inconsistent.
  A bug found on the way is recorded (§8) and raised separately.
- Unifying `Open(...)` (6 signatures) or `HandleKey` (9 return types). These
  are behaviour.
- Moving the dialogs into `internal/bubbles/<widget>` (RFC #236's later
  stages), giving them `Update`/`tea.Model`, or dissolving `styles`.
- Moving `internal/ui/overlay` (the dimmed backdrop) into `internal/bubbles`,
  and collapsing the per-dialog arms of `view_overlays.go`'s `applyOverlays`.
  Both are follow-ups (§9).

## 2. Measured state

Non-test lines per package at `cf9742c6`:

| Package | Lines | `renderBox` | `visibleWindow` | Hand-rolled scrollbar |
|---|---|---|---|---|
| channelfinder | 830 | yes | yes | yes |
| newmessagepicker | 656 | yes (`view.go`) | yes | yes |
| searchresults | 617 | yes (takes `termHeight`) | yes (takes `termHeight`) | yes (4 lines per row) |
| reactionpicker | 553 | yes | yes (over frecent-or-filtered) | yes |
| help | 489 | yes (takes `termHeight`) | yes (takes `termHeight`) | yes |
| themeswitcher | 376 | yes | yes | yes |
| presencemenu | 373 | yes | — (renders every row) | — |
| workspacefinder | 372 | yes | yes | yes |
| reactionsview | 228 | yes (takes `termHeight`) | — (persistent `offset`) | — |
| linkpicker | 200 | yes (`view.go`) | — (renders every row) | — |
| userprofile | 640 | `layout` (card, no list) | — | — |
| confirmprompt (`internal/bubbles`) | 528 | `View` | — | — |
| mentionpicker | 335 | `View` (popup) | — (truncates to 7) | — |
| emojipicker | 237 | `View` (popup) | — (truncates to 5) | — |
| channelpicker | 178 | `View` (popup) | — (truncates to 5) | — |

The tracking doc's 13-package cluster (it counts confirmprompt, not
userprofile or reactionsview) was 5,295 lines on 2026-09-06 and is **5,744**
today: channelfinder grew 110 lines and confirmprompt 348 (the N-button
rewrite in #302/#304).

What is uniform, verified by reading each renderer:

- **Box.** `Border(RoundedBorder) · BorderForeground(Primary) ·
  BorderBackground(bg) · Background(bg) · Padding(1,1) · Width(w)` over content
  passed through `ReapplyBgAfterResets(content, BgANSI()+FgANSI())`. Identical
  in every centered dialog. userprofile differs only in `Padding(0,1)`.
- **Layout.** `title + "\n" + input + "\n\n" + rows` (+ footer where present).
  Hence `listTopOffset = 5` in six packages and `BoxSize` = rows + 7.
- **Filter line.** `"█ " + placeholder` or `query + "█"` inside a `▌`
  left-bordered, `PaddingLeft(1)` style.
- **Selected row.** `▌` in `Accent`, then the row in `Primary`+bold; others get
  a leading space and `TextPrimary`.
- **Window.** "Keep `selected` in view": `start = selected - max + 1` once
  `selected ≥ max`, clamped to `total`. Six copies differ only in where `max`
  comes from (constant 10 or 12; `termHeight`-derived in help and
  searchresults).
- **Scrollbar.** `thumb = max²/total` (≥ 1), `start = offset·(max-thumb)/(total-max)`,
  `█` in `Primary` on a `│` track in `Border`. The same formula
  `bubbles/scrollbar.Overlay` already implements; its doc comment says it was
  copied from these dialogs.
- **Popups.** mention, emoji and channel pickers draw the same
  `RoundedBorder · BorderForeground(Primary) · Background(SurfaceDark) ·
  Width(width-2)` box, with a `▌ ` marker in `Accent`, and no title, input,
  scrollbar or background re-apply.

## 3. The package: `internal/bubbles/chrome`

Pure functions and a `Styles` struct; no state, no `tea.Model`. It obeys the
existing `internal/bubbles` rules (`TestBubblesReachTheAppOnlyThroughCore`):
no `internal/ui` import, no `slack-go`, no I/O. It does not read `styles.*`.

### 3.1 API

```go
package chrome

// Styles is everything chrome draws with.
type Styles struct {
	Box         lipgloss.Style // centered dialog: border, padding, background
	Popup       lipgloss.Style // composer popup box
	Title       lipgloss.Style
	Input       lipgloss.Style // the ▌-bordered filter line
	Placeholder lipgloss.Style
	Row         lipgloss.Style
	RowSelected lipgloss.Style
	Marker      lipgloss.Style // the ▌ before the selected row
	PopupMarker lipgloss.Style // the "▌ " before a popup's selected row
	Empty       lipgloss.Style // "No matching …"
	Footer      lipgloss.Style
	Background  color.Color
	ScrollTrack color.Color
	ScrollThumb color.Color
	BaseANSI    string // re-asserted after every SGR reset inside Box
}

// Window returns the [start, end) slice of a list of total rows that keeps
// selected visible in at most maxRows rows. The one visibleWindow.
func Window(selected, total, maxRows int) (start, end int)

// FilterInput renders the filter line: "█ "+placeholder when query is
// empty, query+"█" otherwise.
func FilterInput(s Styles, query, placeholder string) string

// List draws the visible window of a list. Rows are the pre-rendered
// contents of rows [Start, Start+len(Rows)), each RowHeight lines (default
// 1) joined by "\n". Render prefixes the marker (selected) or a space,
// pads each line to the content width, and appends a scrollbar gutter
// when Total exceeds the window. The thumb is computed in row units and
// painted on every line of a row (searchresults' 4-line rows).
type List struct {
	Rows      []string
	Start     int // index of Rows[0] in the full list
	Selected  int // absolute index; outside the window means none
	Total     int
	Width     int // inner box width the rows occupy
	RowHeight int
}
func (l List) Render(s Styles) []string
func (l List) ContentWidth() int // width a row's own content gets
func (l List) Scrolls() bool     // Total > len(Rows)

// Box is the one renderBox: content, with s.BaseANSI re-asserted after
// every SGR reset, inside box at the given outer width. Dialogs whose
// layout is not a list (confirmprompt, userprofile, presencemenu's
// CustomSnoozeView) call it directly with their own content and, for
// userprofile, their own box style.
func Box(s Styles, box lipgloss.Style, content string, width int) string

// Frame is the list-dialog layout on top of Box: title, input, a blank
// line, the body, an optional footer, inside s.Box. Input "" omits the
// input line (linkpicker, reactionsview); Empty is drawn in place of an
// empty Body.
type Frame struct {
	Title  string
	Input  string
	Body   []string
	Empty  string
	Footer []string
}
func (f Frame) Render(s Styles, width int) string
func (f Frame) BodyTop(s Styles) int // box row of Body[0], for ClickRow
func (f Frame) Height(s Styles) int  // rendered height, for BoxSize

// Popup draws a composer popup: rows (marker applied by List or by the
// caller) inside s.Popup at width. Returns "" for no rows.
func Popup(s Styles, rows []string, width int) string
```

The exact field set is settled during implementation against the goldens:
a field the migrations do not need is dropped, and a per-dialog quirk that
cannot be expressed is handled in the dialog, not by adding a knob (§3.3).

### 3.2 The declared contract

`boxedOverlay` and `clickableOverlay` move out of
`internal/ui/reducer_modal_click.go` into `chrome`, joined by the
visibility pair every dialog already has:

```go
type Dialog interface {
	IsVisible() bool
	Close()
}
type Boxed interface {
	BoxSize(termWidth, termHeight int) (int, int)
}
type Clickable interface {
	Boxed
	ClickRow(termWidth, termHeight, localY int) bool
}
```

Each package asserts what it satisfies, e.g. in channelfinder
`var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)`.
`reducer_modal_click.go` keeps `pointClickable` (userprofile, confirmprompt's
adapter) and uses `chrome.Boxed`/`chrome.Clickable` in place of its own types.
Popups are driven by `compose`, not the router, and assert `chrome.Dialog`
only.

### 3.3 What stays in each dialog

Fields (`selected`, `query`, `filtered`, `offset`…), matching and ranking, the
content of each row (channel glyphs and status, recipient pills, emoji images
and kitty flushes, 4-line search hits, key/description columns), `Open`,
`HandleKey`, `boxWidth` and its clamps, the row cap, and footer wording. A
dialog builds its rows as strings and passes them to `List` and `Frame`.

Known per-dialog differences and where they live:

| Difference | Stays in |
|---|---|
| Width clamps (`[30,80]`, `[40,80]`, `[35,50]`, `[30,60]`, `[36,60]`, `×7/10 ≥ 40`, `≤ termWidth-2`) | each `boxWidth` |
| Row cap (10, 12, `termHeight-10` in `[5,20]`, `visibleRowCap`) | each dialog, passed to `Window` |
| reactionsview's persistent `offset` | reactionsview; it passes its offset as `List.Start` |
| reactionpicker's post-composite clamp to `termHeight` and kitty flushes | reactionpicker |
| searchresults' head-side truncation of a long query | searchresults, before `FilterInput` |
| help's tri-state input line and two-line footer | help builds `Frame.Input` / `Frame.Footer` |
| newmessagepicker's pill bar | newmessagepicker builds `Frame.Input` |
| userprofile's `Padding(0,1)` card and section dropping | userprofile, with its own `Styles.Box` |
| `BoxSize` measured by rendering (help, newmessagepicker) | unchanged; `Frame.Height` may replace the render where it is exact |

### 3.4 Where the outliers land

- **confirmprompt** gains `Chrome chrome.Styles` in place of its `Box` and
  `BaseANSI` fields; `View` keeps its own title/body/buttons layout and calls
  `chrome.Box`. `Title`, `Body`, `Button`, `ButtonActive`, `Footer` stay its
  own. Its exported `Styles` changes shape; the only constructor outside its
  tests is `confirmPromptStyles()` in `internal/ui/confirm.go`.
- **userprofile** calls `chrome.Box` with its own `Padding(0,1)` style.
- **linkpicker, reactionsview** use `Frame` with no input, and `List`.
- **presencemenu** uses `Frame` and `List` with every row visible (no
  `Window`). `CustomSnoozeView` (title, blank, input, blank, hint) calls
  `chrome.Box` and `FilterInput`.
- **Popups** use `List` (no scrollbar: their lists are already cut to 5 or
  7) and `Popup`. They keep cutting, not scrolling.

## 4. Theme

`internal/ui/messages` gains `ChromeStyles() chrome.Styles`, built from the
current theme on every call. It sits beside `messages.ThemeListStyles()`, the
precedent for mapping the theme onto a `bubbles` component, and next to
`BgANSI`/`FgANSI`, which `BaseANSI` needs (`styles` cannot import
`messages`). Dialogs call it while drawing; nothing is stored, so
`applyTheme` does not change and a theme switch needs no push.
`confirmPromptStyles()` builds its `Chrome` field from it, so the prompt's
existing snapshot-and-push stays as it is.

## 5. Tests

**`chrome` package** (stdlib `testing`, white-box, as the repo does):
`Window` at the edges (empty, `total < max`, selection at both ends,
`maxRows ≤ 0`); `Box` re-asserting `BaseANSI` after a reset; `List` with and without a scrollbar, with `RowHeight > 1`,
with the selection outside the window; `Frame` with and without input, empty
body, footer; `BodyTop`/`Height` equal to the row and line count of
`Render`'s output for each shape; `Popup` empty and non-empty. A thumb test
pins `List`'s thumb to `scrollbar`'s formula.

**`bubbles/scrollbar`** gains `Thumb(total, offset, visible) (start, end int)`,
extracted from `Overlay` (which calls it), so `List` and `Overlay` share one
formula.

**App goldens** are the behaviour lock (§6 step 0). Per-package tests stay as
they are. A package test that breaks because its string comparison was
byte-level but the screen is unchanged is updated in that package's commit,
with the reason in the commit message.

## 6. Migration order

Each step is a commit (or a few) with `go build ./...`, `go vet ./...`,
`go test ./... -race`, `gofmt -l .` and `golangci-lint run` green.

**Step 0 — characterize (test-only).** New App-level goldens via
`newGoldenApp` + `compareGolden`, one per dialog in an overflowing state
(scrollbar visible, selection mid-list, where the dialog scrolls), plus an
empty/no-match state where the dialog has one, and each popup open above the
composer. Around 20 new files in `internal/ui/testdata/golden/`. Then save
the full golden directory as the cell baseline for
`SLK_GOLDEN_BASELINE=<dir> go test ./internal/ui -run TestGoldenCellsMatchBaseline`.

**Step 1 — the package.** `internal/bubbles/chrome` with its tests,
`scrollbar.Thumb`, `messages.ChromeStyles()`, and the interfaces moved out of
`reducer_modal_click.go`. No dialog uses it yet.

**Steps 2–13 — dialogs, one per commit, simplest first:** themeswitcher,
workspacefinder, presencemenu, channelfinder, reactionpicker,
newmessagepicker, help, searchresults, linkpicker, reactionsview,
userprofile, confirmprompt. Each commit deletes that package's
`renderBox`/`visibleWindow`/scrollbar, adds its compile-time assertion, and
passes the cell baseline.

**Steps 14–16 — popups:** emojipicker, channelpicker, mentionpicker.

**Step 17 — close-out.** `AGENTS.md`: add `chrome` to "UI chrome", and remove
the `renderBox`/`visibleWindow` entry from "Known duplication". Tracking doc:
Phase 4 "Achieved", status table, target-end-state rows. Re-measure §2.

### 6.1 Golden rule

Every golden stays **cell-identical** to the step-0 baseline: the same
characters and colours in the same cells. Bytes may change (for example,
padding emitted as one styled run instead of `strings.Repeat`). Such a golden
is re-blessed in the commit that changed it, after `TestGoldenCellsMatchBaseline`
passes. A cell difference is a migration bug, never a re-bless.

### 6.2 PRs

Three stacked PRs, as Phase 3 did:

1. steps 0–1 (goldens and the package);
2. steps 2–13 (the dialogs);
3. steps 14–17 (popups and close-out).

## 7. Exit criteria

| Criterion | Bar |
|---|---|
| `renderBox` implementations | 0 outside `chrome` (`Box` is the one) |
| `visibleWindow` implementations | 0 outside `chrome` (`Window` is the one) |
| Hand-rolled scrollbar thumb math | 0 outside `bubbles/scrollbar` |
| Compile-time `chrome` assertions | every dialog and popup package |
| Goldens | all cell-identical to the step-0 baseline |
| `go test ./... -race` | PASS |
| Cluster line count | measured and recorded; **not a gate** |

The tracking doc's "below ~3,000 lines" target is retired. Moving only the
chrome removes an estimated 1,200–1,400 lines, leaving the cluster near
4,400; what remains is matching and row content, which stay by design.
The tracking doc is updated to say so in step 17.

## 8. Risks

- **Silent visual drift.** Most dialogs have no golden today. Step 0 exists
  for this; no migration commit lands before its dialog has a golden.
- **Hit-testing drift.** `ClickRow` depends on `listTopOffset` matching the
  render. `Frame.BodyTop` replaces the constant and is tested against
  `Render`, and the existing click tests (`reducer_modal_click` tests,
  `clickConfirmText`) must stay green.
- **Over-fitting `Frame`.** If a dialog needs a knob only it uses, the
  dialog keeps that piece of rendering instead (§3.3).
- **Bugs found in flight** are recorded here as addenda and raised as issues,
  not fixed in a migration commit.

## 9. Follow-ups (not in this phase)

- Move `internal/ui/overlay` to `internal/bubbles/overlay` (RFC #236's target
  layout) and give `chrome` a dimmed-composite helper; the per-dialog
  `ViewOverlay` bodies then collapse.
- Collapse `applyOverlays` in `view_overlays.go` onto a slice of overlays.
- The `/` command picker for #247, built on `chrome.List` + `chrome.Popup`.
- Converging width clamps and matchers, if wanted, one per PR.
