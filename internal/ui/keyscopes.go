// internal/ui/keyscopes.go
//
// Where each Normal-mode KeyMap binding acts, as data (adaptive key
// hints design §5): the table the key_hints context menu is built
// from. One row per binding; TestKeyScopesCoverTheKeyMap fails when a
// binding has no row, so a new key cannot be left out of the menu. The
// configurable sidebar actions are the exception: keyActions already
// says where they act, and the menu reads it there.
package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/bubbles/keyseq"
)

// hintContext is where Normal-mode keys land: the focused pane and, for
// the messages panel, which list the view puts there. A set of them
// (a bit each) says where a binding acts.
type hintContext uint8

const (
	inSidebar      hintContext = 1 << iota // the sidebar, in any view
	inChannel                              // the channel pane (the channel view)
	inThreadsList                          // the Threads view's list
	inActivityList                         // the Activity view's list
	inThread                               // the thread pane

	// inPanes is where a message is selected: message operations act
	// on the channel or thread pane's selection. The Threads and
	// Activity lists hide the channel pane (beginForwardOfSelected and
	// currentLocation refuse them for that reason).
	inPanes    = inChannel | inThread
	everywhere = inSidebar | inChannel | inThreadsList | inActivityList | inThread
)

// hintContext is the context focus is in now.
func (a *App) hintContext() hintContext {
	switch a.focusedPanel {
	case PanelSidebar:
		return inSidebar
	case PanelThread:
		return inThread
	}
	switch a.view {
	case ViewThreads:
		return inThreadsList
	case ViewActivity:
		return inActivityList
	}
	return inChannel
}

// name is the context as the menu's title says it.
func (c hintContext) name() string {
	switch c {
	case inSidebar:
		return "sidebar"
	case inThreadsList:
		return "threads"
	case inActivityList:
		return "activity"
	case inThread:
		return "thread"
	}
	return "messages"
}

// scopeKind is how a binding dispatches, which the menu needs for
// precedence and for drawing groups.
type scopeKind uint8

const (
	plainKey scopeKind = iota
	// prefixKey starts a key sequence (the leader, ctrl+w): the root
	// table takes it before any panel-scoped key, and the menu shows it
	// as a group.
	prefixKey
	// chordKey starts a chord handleNormalKey arms (the g of gg): a
	// group in the menu.
	chordKey
)

// keyScope is one Normal-mode KeyMap binding and where it acts: the
// contexts in where, when (if set) also holds. label names a group in
// the menu ("+leader"); empty uses the binding's description.
type keyScope struct {
	binding func(*KeyMap) *key.Binding
	where   hintContext
	when    func(*App) bool
	kind    scopeKind
	label   string
}

// State a few bindings need besides the context.
var (
	threadOpen = func(a *App) bool { return a.threadVisible }
	// searchable: / opens a prompt where searchScopeForFocus has a scope
	// (adaptive search); stepsSearch: n/N step where searchStepsHere
	// says the active search's matches are drawn.
	searchable   = func(a *App) bool { _, ok := a.searchScopeForFocus(); return ok }
	stepsSearch  = func(a *App) bool { return a.searchStepsHere() }
	activityView = func(a *App) bool { return a.view == ViewActivity }
	sidebarShown = func(a *App) bool { return a.sidebarVisible }
	// composerShown: i lands in a composer on screen (the channel
	// view's, or an open thread's where the thread pane takes it).
	composerShown = func(a *App) bool {
		return a.view == ViewChannels || a.threadVisible && (a.focusedPanel == PanelThread || a.view == ViewThreads)
	}
	// markable: m has a message to name (currentLocation); the
	// Activity view names nothing outside the thread pane.
	markable = func(a *App) bool { return a.view != ViewActivity || a.focusedPanel == PanelThread }
)

// keyScopes says where every Normal-mode KeyMap binding acts
// (handleNormalKey and the root key sequences), in KeyMap order.
// Insert-only bindings (insertOnly) and keyless help-only rows have
// none; the sidebar's configurable keys are in keyActions.
var keyScopes = []keyScope{
	{binding: func(k *KeyMap) *key.Binding { return &k.Up }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.Down }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.Left }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.Right }, where: everywhere},
	// In the thread pane Enter only follows a forwarded message's link.
	{binding: func(k *KeyMap) *key.Binding { return &k.Enter }, where: inChannel | inThreadsList | inActivityList},
	// Never listed: in the menu, Esc closes it.
	{binding: func(k *KeyMap) *key.Binding { return &k.Escape }},
	{binding: func(k *KeyMap) *key.Binding { return &k.InsertMode }, where: everywhere &^ inActivityList, when: composerShown},
	{binding: func(k *KeyMap) *key.Binding { return &k.CommandMode }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.SearchMode }, where: everywhere, when: searchable},
	{binding: func(k *KeyMap) *key.Binding { return &k.SearchNext }, where: everywhere, when: stepsSearch},
	{binding: func(k *KeyMap) *key.Binding { return &k.SearchPrev }, where: everywhere, when: stepsSearch},
	{binding: func(k *KeyMap) *key.Binding { return &k.WorkspaceSearch }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.Tab }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ShiftTab }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ToggleSidebar }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.SidebarGrow }, where: everywhere, when: sidebarShown},
	{binding: func(k *KeyMap) *key.Binding { return &k.SidebarShrink }, where: everywhere, when: sidebarShown},
	{binding: func(k *KeyMap) *key.Binding { return &k.ToggleThread }, where: everywhere, when: threadOpen},
	{binding: func(k *KeyMap) *key.Binding { return &k.FuzzyFinder }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.FuzzyFinderAlt }, where: everywhere},
	// handleGoToTop has no Activity branch.
	{binding: func(k *KeyMap) *key.Binding { return &k.Top }, where: everywhere &^ inActivityList, kind: chordKey},
	{binding: func(k *KeyMap) *key.Binding { return &k.Bottom }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.PageUp }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.PageDown }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.HalfPageUp }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.HalfPageDown }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.Quit }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.QuitConfirm }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.CloseThreadView }, where: everywhere, when: threadOpen},
	{binding: func(k *KeyMap) *key.Binding { return &k.Reaction }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.ReactionNav }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.Edit }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.Delete }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.CopyMessage }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.CopyPermalink }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.ForwardMessage }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.OpenPreview }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.OpenLink }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.DownloadFile }, where: inPanes},
	// In the Activity list U toggles the selected item's read state.
	{binding: func(k *KeyMap) *key.Binding { return &k.MarkUnread }, where: inPanes | inActivityList},
	{binding: func(k *KeyMap) *key.Binding { return &k.MarkSet }, where: everywhere, when: markable},
	{binding: func(k *KeyMap) *key.Binding { return &k.JumpMark }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.NextUnread }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.PrevUnread }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ActivityView }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ActivityUnread }, where: everywhere, when: activityView},
	{binding: func(k *KeyMap) *key.Binding { return &k.NewMessage }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ThemeSwitcher }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ThemeSwitcherGlobal }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ThemeNext }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.ThemePrev }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.PresenceMenu }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.NavBack }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.NavForward }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.Help }, where: everywhere},
	{binding: func(k *KeyMap) *key.Binding { return &k.SaveThread }, where: inThread},
	{binding: func(k *KeyMap) *key.Binding { return &k.RespondButtons }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.ListReactions }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.UserProfile }, where: inPanes},
	{binding: func(k *KeyMap) *key.Binding { return &k.Leader }, where: everywhere, kind: prefixKey, label: "leader"},
	{binding: func(k *KeyMap) *key.Binding { return &k.WindowPrefix }, where: everywhere, kind: prefixKey, label: windowsLabel},
}

// contextKeyMap is the context menu's table and title: a binding per
// key that acts in the current context, whose action runs that key as
// if typed in Normal mode (so the leader starts a sequence and g arms
// gg, as always). Keys are claimed in dispatch order — the root
// sequence keys, then the focused panel's configurable keys
// (keyActions), then the rest — and a claimed key leaves later
// bindings. The KeyMap is read as the leader leaves it (helpKeyMap), and
// Esc and Backspace belong to the menu.
func (a *App) contextKeyMap() (*keyseq.Map, string) {
	km := helpKeyMap(a.keys, a.keys.Leader.Keys()[0])
	ctx := a.hintContext()
	run := func(msg tea.KeyMsg) tea.Cmd { return handleNormalMode(a, msg) }
	taken := map[string]bool{"esc": true, "backspace": true}
	var bindings []keyseq.Binding
	add := func(b key.Binding, group bool, label string) {
		shown, hidden := shownKeys(b)
		shown = slices.DeleteFunc(shown, func(k string) bool { return taken[k] })
		hidden = slices.DeleteFunc(hidden, func(k string) bool { return taken[k] })
		if len(shown) == 0 {
			return // what is left is an alias the help never shows
		}
		desc := b.Help().Desc
		if group && label == "" {
			label = desc
		}
		hint := desc
		if group {
			hint = label
		}
		for i, k := range shown {
			taken[k] = true
			kb := keyseq.Binding{Keys: []string{k}, Hint: hint, Desc: desc, Action: run, Group: group}
			if i == len(shown)-1 {
				kb.Keys = append(kb.Keys, hidden...)
			}
			bindings = append(bindings, kb)
		}
		for _, k := range hidden {
			taken[k] = true
		}
	}
	applies := func(s keyScope) bool { return s.where&ctx != 0 && (s.when == nil || s.when(a)) }
	for _, s := range keyScopes {
		if s.kind == prefixKey && applies(s) {
			add(*s.binding(&km), true, s.label)
		}
	}
	for _, act := range keyActions {
		if act.scope == a.focusedPanel {
			add(*act.binding(&km), false, "")
		}
	}
	for _, s := range keyScopes {
		if s.kind != prefixKey && applies(s) {
			add(*s.binding(&km), s.kind != plainKey, s.label)
		}
	}
	return keyseq.New(bindings...), "keys: " + ctx.name()
}

// shownKeys splits b's keys into those its help label names ("k/up",
// "ctrl+l") and the rest: aliases the help leaves out (ctrl+k for
// ctrl+l) that still work. A label that names none of them ("gg",
// "PgUp") shows them all.
func shownKeys(b key.Binding) (shown, hidden []string) {
	label := strings.Split(b.Help().Key, "/")
	for _, k := range b.Keys() {
		if slices.Contains(label, k) {
			shown = append(shown, k)
		} else {
			hidden = append(hidden, k)
		}
	}
	if len(shown) == 0 {
		return hidden, nil
	}
	return shown, hidden
}
