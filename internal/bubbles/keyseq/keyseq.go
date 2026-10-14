// Package keyseq dispatches multi-key sequences — vim's <leader> maps and
// prefix chords such as ctrl+w — from a declarative table.
//
// A Map lists the keys valid at one step of a sequence. A Binding either
// completes a sequence (its Action runs) or is a prefix (its Next Map
// takes the following key). A Sequence is the state of one sequence being
// typed. The host owns the tables and runs the actions; keyseq resolves
// keys and describes a table for a which-key hint, a which-key menu
// (Items) and a help overlay.
//
// Keys are tea.Key.String() forms ("h", "left", "ctrl+w", "space", "\\"),
// the same strings key.Binding matches. While a sequence is pending,
// "esc" cancels it and "backspace" goes back a step: both belong to the
// sequence, not to any map.
package keyseq

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Action is what a Binding runs when its key completes a sequence. It
// receives the completing key.
type Action func(tea.KeyMsg) tea.Cmd

// Binding is one entry of a Map.
type Binding struct {
	// Keys select the binding. Keys[0] is the label hints and help show.
	Keys []string
	// Hint is the short which-key label ("split"); for a group, its
	// label ("windows"). Empty leaves the binding out of Hints.
	Hint string
	// Desc is the help-overlay description. Empty leaves the binding's
	// own row out of Help (a prefix's children are still listed).
	Desc string
	// Action runs when the binding completes a sequence. Ignored for a
	// prefix.
	Action Action
	// Next, when non-nil, makes the binding a prefix: the key after it
	// is looked up in Next.
	Next *Map
	// Group marks a binding with no Next whose Action starts another
	// pending state the host tracks itself (vim's gg): hints and menus
	// show it as a group, like a prefix.
	Group bool
}

// isGroup reports whether b leads to more keys: a prefix, or a Group.
func (b Binding) isGroup() bool { return b.Next != nil || b.Group }

// Map is the set of bindings valid at one step of a sequence, in display
// order. The first binding listing a key wins it.
type Map struct {
	bindings []Binding
}

// New builds a Map from bindings, in display order.
func New(bindings ...Binding) *Map {
	return &Map{bindings: bindings}
}

// Lookup returns the first binding listing k.
func (m *Map) Lookup(k string) (Binding, bool) {
	for _, b := range m.bindings {
		for _, bk := range b.Keys {
			if bk == k {
				return b, true
			}
		}
	}
	return Binding{}, false
}

// Hints returns the which-key labels for the map, "<keys> <hint>" in
// display order; a group's is "<keys> +<hint>" ("w +windows"). A run of
// consecutive bindings with the same Hint is one label with the keys
// joined by "/" ("h/j/k/l focus").
func (m *Map) Hints() []string {
	var out []string
	for _, r := range runs(m.bindings, func(b Binding) string { return b.Hint }) {
		text := r.text
		if r.group {
			text = "+" + text
		}
		out = append(out, r.keys()+" "+text)
	}
	return out
}

// Item is one row of a which-key menu: the keys (each binding's label),
// what they do, and whether they lead to more keys.
type Item struct {
	Keys  []string
	Desc  string
	Group bool
}

// Items describes the map for a which-key menu, in display order. An
// action's text is its Desc (its Hint when it has none), a group's its
// Hint, the label. A run of consecutive bindings with the same text is
// one item with each binding's label (h/j/k/l → focus pane).
func (m *Map) Items() []Item {
	text := func(b Binding) string {
		if b.isGroup() || b.Desc == "" {
			return b.Hint
		}
		return b.Desc
	}
	var out []Item
	for _, r := range runs(m.bindings, text) {
		out = append(out, Item{Keys: r.labels, Desc: r.text, Group: r.group})
	}
	return out
}

// Entry is one help-overlay row.
type Entry struct {
	Keys string
	Desc string
}

// Help flattens the map into help rows whose keys start with prefix
// (the keys that lead to this map, e.g. "\\" for a leader layer). A run
// of consecutive bindings with the same Desc is one row. A prefix
// binding contributes its own row, then its Next map's rows under
// "prefix key".
func (m *Map) Help(prefix string) []Entry {
	var out []Entry
	for _, r := range runs(m.bindings, func(b Binding) string { return b.Desc }) {
		out = append(out, Entry{Keys: join(prefix, r.keys()), Desc: r.text})
	}
	for _, b := range m.bindings {
		if b.Next != nil && len(b.Keys) > 0 {
			out = append(out, b.Next.Help(join(prefix, b.Keys[0]))...)
		}
	}
	return out
}

// run is consecutive bindings sharing one non-empty text.
type run struct {
	labels []string // each binding's label, Keys[0]
	text   string
	group  bool
}

// keys is the run's labels joined by "/" ("h/j/k/l").
func (r run) keys() string { return strings.Join(r.labels, "/") }

// runs groups the bindings with a non-empty text(b) into runs of
// consecutive equal texts. A binding without text ends the run before
// it, and a group never shares a run: its keys lead somewhere different
// from its neighbours'.
func runs(bindings []Binding, text func(Binding) string) []run {
	var out []run
	extend := false // whether the next binding may join out[len(out)-1]
	for _, b := range bindings {
		t := text(b)
		if t == "" || len(b.Keys) == 0 {
			extend = false
			continue
		}
		group := b.isGroup()
		if n := len(out); extend && !group && out[n-1].text == t {
			out[n-1].labels = append(out[n-1].labels, b.Keys[0])
		} else {
			out = append(out, run{labels: []string{b.Keys[0]}, text: t, group: group})
		}
		extend = !group
	}
	return out
}

func join(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + " " + k
}

// Sequence is the state of one sequence being typed: a stack of steps,
// each the Map the next key is looked up in and how it was reached. The
// zero value is idle.
type Sequence struct {
	steps []step
}

// step is one level of a pending sequence: next takes the following
// key; key is the key typed to arm it ("" for Arm), label the arming
// binding's Hint (Arm's title).
type step struct {
	next  *Map
	key   string
	label string
}

// Pending reports whether a prefix has been typed, so the next key
// continues the sequence.
func (s *Sequence) Pending() bool { return len(s.steps) > 0 }

// Reset abandons the sequence being typed, if any.
func (s *Sequence) Reset() {
	s.steps = nil
}

// Arm starts a sequence at m as if a prefix had armed it: a menu the
// host opens itself. title stands in for the keys typed (Title).
func (s *Sequence) Arm(m *Map, title string) {
	s.steps = []step{{next: m, label: title}}
}

// Back returns to the step before the last prefix; from the first step
// it cancels the sequence. It reports whether the sequence is still
// pending.
func (s *Sequence) Back() bool {
	if len(s.steps) > 0 {
		s.steps = s.steps[:len(s.steps)-1]
	}
	return s.Pending()
}

// Next returns the map the next key is looked up in, or nil when idle.
func (s *Sequence) Next() *Map {
	if len(s.steps) == 0 {
		return nil
	}
	return s.steps[len(s.steps)-1].next
}

// Title is the keys typed so far, separated by spaces ("\\ w"), after
// the title an Arm started with; "" when idle.
func (s *Sequence) Title() string {
	parts := make([]string, 0, len(s.steps))
	for _, st := range s.steps {
		if st.key == "" {
			parts = append(parts, st.label)
		} else {
			parts = append(parts, st.key)
		}
	}
	return strings.Join(parts, " ")
}

// Typed returns the keys typed so far (an Arm's title is not a key).
func (s *Sequence) Typed() []string {
	var out []string
	for _, st := range s.steps {
		if st.key != "" {
			out = append(out, st.key)
		}
	}
	return out
}

// Label is the Hint of the binding that armed the current map (a
// group's label), or "" when it has none, after Arm, or idle.
func (s *Sequence) Label() string {
	if n := len(s.steps); n > 0 && s.steps[n-1].key != "" {
		return s.steps[n-1].label
	}
	return ""
}

// Press feeds key k.
//
// Idle, k is looked up in root: a prefix starts a sequence, an action
// completes one, and any other key is not consumed (false), so the
// caller handles it as a plain key.
//
// Pending, k is looked up in the map the last prefix armed and is always
// consumed: "esc" and keys the map lacks cancel the sequence, and nothing
// else happens; "backspace" goes back a step (Back).
//
// Press returns the action k completes, or nil.
func (s *Sequence) Press(root *Map, k string) (Action, bool) {
	m, pending := root, s.Pending()
	if pending {
		m = s.Next()
		switch k {
		case "esc":
			s.Reset()
			return nil, true
		case "backspace":
			s.Back()
			return nil, true
		}
	}
	b, ok := m.Lookup(k)
	if !ok {
		s.Reset()
		return nil, pending
	}
	if b.Next != nil {
		s.steps = append(s.steps, step{next: b.Next, key: k, label: b.Hint})
		return nil, true
	}
	s.Reset()
	return b.Action, true
}

// Hint describes the pending sequence for a which-key display: lead is
// Title followed by " …", entries the armed map's Hints. Both are empty
// when idle.
func (s *Sequence) Hint() (lead string, entries []string) {
	if !s.Pending() {
		return "", nil
	}
	return s.Title() + " …", s.Next().Hints()
}
