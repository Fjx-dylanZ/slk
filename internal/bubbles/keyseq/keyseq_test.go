package keyseq

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// recorder returns an Action that appends name to *log, and the log.
func recorder() (func(name string) Action, *[]string) {
	var log []string
	return func(name string) Action {
		return func(tea.KeyMsg) tea.Cmd {
			log = append(log, name)
			return nil
		}
	}, &log
}

// tables builds a root with a leader ("\\") whose layer has an action
// and a window prefix shared with a ctrl+w root prefix.
func tables(act func(string) Action) (root, win *Map) {
	win = New(
		Binding{Keys: []string{"h", "left"}, Hint: "focus", Desc: "focus pane", Action: act("left")},
		Binding{Keys: []string{"j", "down"}, Hint: "focus", Desc: "focus pane", Action: act("down")},
		Binding{Keys: []string{"s"}, Hint: "split", Desc: "split window", Action: act("split")},
		Binding{Keys: []string{"q"}, Hint: "close", Desc: "close window", Action: act("close")},
		Binding{Keys: []string{"c"}, Hint: "close", Desc: "close window", Action: act("close")},
	)
	leader := New(
		Binding{Keys: []string{"w"}, Hint: "window", Desc: "window commands", Next: win},
		Binding{Keys: []string{"t"}, Hint: "channel", Desc: "switch channel", Action: act("finder")},
	)
	root = New(
		Binding{Keys: []string{"\\"}, Next: leader},
		Binding{Keys: []string{"ctrl+w"}, Next: win},
	)
	return root, win
}

func TestLookupFirstBindingWins(t *testing.T) {
	act, log := recorder()
	m := New(
		Binding{Keys: []string{"x"}, Action: act("first")},
		Binding{Keys: []string{"y", "x"}, Action: act("second")},
	)
	b, ok := m.Lookup("x")
	if !ok {
		t.Fatal("x not found")
	}
	b.Action(nil)
	if b, ok := m.Lookup("y"); !ok || len(b.Keys) != 2 {
		t.Fatalf("y = %+v %v, want the second binding", b, ok)
	}
	if _, ok := m.Lookup("z"); ok {
		t.Fatal("z must not be found")
	}
	if !slices.Equal(*log, []string{"first"}) {
		t.Fatalf("ran %v, want [first]", *log)
	}
}

func TestSequencePress(t *testing.T) {
	type step struct {
		key          string
		wantConsumed bool
		wantAction   string // "" = no action returned
		wantPending  bool
	}
	cases := []struct {
		name  string
		steps []step
	}{
		{"plain key is not consumed", []step{{"x", false, "", false}}},
		{"leader arms", []step{{"\\", true, "", true}}},
		{"leader action", []step{{"\\", true, "", true}, {"t", true, "finder", false}}},
		{"nested prefix", []step{{"\\", true, "", true}, {"w", true, "", true}, {"s", true, "split", false}}},
		{"second key of a binding", []step{{"ctrl+w", true, "", true}, {"left", true, "left", false}}},
		{"esc cancels", []step{{"\\", true, "", true}, {"esc", true, "", false}}},
		{"unbound key cancels, consumed", []step{{"\\", true, "", true}, {"D", true, "", false}}},
		{"after a cancel the next key is plain", []step{{"\\", true, "", true}, {"esc", true, "", false}, {"t", false, "", false}}},
		{"esc while idle is not consumed", []step{{"esc", false, "", false}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			act, log := recorder()
			root, _ := tables(act)
			var s Sequence
			for i, st := range tc.steps {
				before := len(*log)
				a, consumed := s.Press(root, st.key)
				if consumed != st.wantConsumed {
					t.Fatalf("step %d %q: consumed = %v, want %v", i, st.key, consumed, st.wantConsumed)
				}
				if a != nil {
					a(nil)
				}
				var got string
				if len(*log) > before {
					got = (*log)[len(*log)-1]
				}
				if got != st.wantAction {
					t.Fatalf("step %d %q: action = %q, want %q", i, st.key, got, st.wantAction)
				}
				if s.Pending() != st.wantPending {
					t.Fatalf("step %d %q: pending = %v, want %v", i, st.key, s.Pending(), st.wantPending)
				}
			}
		})
	}
}

func TestSequenceReset(t *testing.T) {
	act, _ := recorder()
	root, _ := tables(act)
	var s Sequence
	s.Press(root, "\\")
	s.Reset()
	if s.Pending() {
		t.Fatal("Reset left the sequence pending")
	}
	if lead, entries := s.Hint(); lead != "" || entries != nil {
		t.Fatalf("idle hint = %q %v, want empty", lead, entries)
	}
}

func TestSequenceHint(t *testing.T) {
	act, _ := recorder()
	root, _ := tables(act)
	var s Sequence
	s.Press(root, "\\")
	lead, entries := s.Hint()
	if lead != "\\ …" {
		t.Errorf("lead = %q, want %q", lead, "\\ …")
	}
	if want := []string{"w window", "t channel"}; !slices.Equal(entries, want) {
		t.Errorf("entries = %q, want %q", entries, want)
	}
	s.Press(root, "w")
	lead, entries = s.Hint()
	if lead != "\\ w …" {
		t.Errorf("nested lead = %q, want %q", lead, "\\ w …")
	}
	if want := []string{"h/j focus", "s split", "q/c close"}; !slices.Equal(entries, want) {
		t.Errorf("nested entries = %q, want %q", entries, want)
	}
}

func TestHintsSkipEmptyAndKeepPrefixesApart(t *testing.T) {
	sub := New()
	m := New(
		Binding{Keys: []string{"a"}, Hint: "same"},
		Binding{Keys: []string{"p"}, Hint: "same", Next: sub},
		Binding{Keys: []string{"b"}, Hint: "same"},
		Binding{Keys: []string{"x"}},
		Binding{Keys: []string{"c"}, Hint: "same"},
	)
	// The prefix p must not merge with its neighbours, and the hint-less
	// x breaks the run between b and c.
	want := []string{"a same", "p same", "b same", "c same"}
	if got := m.Hints(); !slices.Equal(got, want) {
		t.Fatalf("Hints = %q, want %q", got, want)
	}
}

func TestHelpFlattensPrefixes(t *testing.T) {
	act, _ := recorder()
	root, _ := tables(act)
	leader, _ := root.Lookup("\\")
	got := leader.Next.Help("\\")
	want := []Entry{
		{Keys: "\\ w", Desc: "window commands"},
		{Keys: "\\ t", Desc: "switch channel"},
		{Keys: "\\ w h/j", Desc: "focus pane"},
		{Keys: "\\ w s", Desc: "split window"},
		{Keys: "\\ w q/c", Desc: "close window"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Help =\n%q\nwant\n%q", got, want)
	}
	// The root's prefixes have no Desc of their own: only their
	// children are listed, under each prefix.
	rows := root.Help("")
	if !slices.Contains(rows, Entry{Keys: "ctrl+w s", Desc: "split window"}) {
		t.Fatalf("root Help lacks the ctrl+w rows: %q", rows)
	}
	if slices.ContainsFunc(rows, func(e Entry) bool { return e.Keys == "\\" || e.Keys == "ctrl+w" }) {
		t.Fatalf("root Help lists a Desc-less prefix row: %q", rows)
	}
}
