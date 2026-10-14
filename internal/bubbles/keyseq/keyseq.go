// Package keyseq dispatches multi-key sequences — vim's <leader> maps and
// prefix chords such as ctrl+w — from a declarative table.
//
// A Map lists the keys valid at one step of a sequence. A Binding either
// completes a sequence (its Action runs) or is a prefix (its Next Map
// takes the following key). A Sequence is the state of one sequence being
// typed. The host owns the tables and runs the actions; keyseq resolves
// keys and describes a table for a which-key hint and a help overlay.
//
// Keys are tea.Key.String() forms ("h", "left", "ctrl+w", "space", "\\"),
// the same strings key.Binding matches.
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
	// Hint is the short which-key label ("split"). Empty leaves the
	// binding out of Hints.
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
}

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
// display order. A run of consecutive bindings with the same Hint is one
// label with the keys joined by "/" ("h/j/k/l focus").
func (m *Map) Hints() []string {
	var out []string
	for _, r := range runs(m.bindings, func(b Binding) string { return b.Hint }) {
		out = append(out, r.keys+" "+r.text)
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
		out = append(out, Entry{Keys: join(prefix, r.keys), Desc: r.text})
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
	keys string // their labels joined by "/"
	text string
}

// runs groups the bindings with a non-empty text(b) into runs of
// consecutive equal texts. A binding without text ends the run before
// it, and a prefix never shares a run: its keys lead somewhere different
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
		isPrefix := b.Next != nil
		if n := len(out); extend && !isPrefix && out[n-1].text == t {
			out[n-1].keys += "/" + b.Keys[0]
		} else {
			out = append(out, run{keys: b.Keys[0], text: t})
		}
		extend = !isPrefix
	}
	return out
}

func join(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + " " + k
}

// Sequence is the state of one sequence being typed: the Map the next key
// is looked up in and the keys typed so far. The zero value is idle.
type Sequence struct {
	next  *Map
	typed []string
}

// Pending reports whether a prefix has been typed, so the next key
// continues the sequence.
func (s *Sequence) Pending() bool { return s.next != nil }

// Reset abandons the sequence being typed, if any.
func (s *Sequence) Reset() {
	s.next = nil
	s.typed = nil
}

// Press feeds key k.
//
// Idle, k is looked up in root: a prefix starts a sequence, an action
// completes one, and any other key is not consumed (false), so the
// caller handles it as a plain key.
//
// Pending, k is looked up in the map the last prefix armed and is always
// consumed: "esc" and keys the map lacks cancel the sequence, and nothing
// else happens.
//
// Press returns the action k completes, or nil.
func (s *Sequence) Press(root *Map, k string) (Action, bool) {
	m, pending := root, s.next != nil
	if pending {
		m = s.next
		if k == "esc" {
			s.Reset()
			return nil, true
		}
	}
	b, ok := m.Lookup(k)
	if !ok {
		s.Reset()
		return nil, pending
	}
	if b.Next != nil {
		s.next = b.Next
		s.typed = append(s.typed, k)
		return nil, true
	}
	s.Reset()
	return b.Action, true
}

// Hint describes the pending sequence for a which-key display: lead is
// the keys typed so far followed by " …", entries the armed map's Hints.
// Both are empty when idle.
func (s *Sequence) Hint() (lead string, entries []string) {
	if s.next == nil {
		return "", nil
	}
	return strings.Join(s.typed, " ") + " …", s.next.Hints()
}
