package listfilter

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/chrome"
)

// typed returns a filter that is typing q.
func typed(q string) Filter {
	var f Filter
	f.HandleKey("/")
	for _, r := range q {
		f.HandleKey(string(r))
	}
	return f
}

// kept returns a filter that kept q with enter.
func kept(q string) Filter {
	f := typed(q)
	f.HandleKey("enter")
	return f
}

// TestHandleKey is the key table in the package doc, one row per state
// and key.
func TestHandleKey(t *testing.T) {
	cases := []struct {
		name     string
		start    Filter
		key      string
		consumed bool
		changed  bool
		query    string
		typing   bool
		active   bool
	}{
		{"idle / starts typing", Filter{}, "/", true, false, "", true, true},
		{"idle esc is the dialog's", Filter{}, "esc", false, false, "", false, false},
		{"idle letter is the dialog's", Filter{}, "q", false, false, "", false, false},
		{"idle enter is the dialog's", Filter{}, "enter", false, false, "", false, false},
		{"idle backspace is the dialog's", Filter{}, "backspace", false, false, "", false, false},
		{"idle down is the dialog's", Filter{}, "down", false, false, "", false, false},

		{"typing letter appends", typed("de"), "p", true, true, "dep", true, true},
		{"typing q appends", typed(""), "q", true, true, "q", true, true},
		{"typing j appends", typed("a"), "j", true, true, "aj", true, true},
		{"typing / appends", typed("a"), "/", true, true, "a/", true, true},
		{"typing ' appends", typed(""), "'", true, true, "'", true, true},
		{"typing a multi-byte rune appends", typed("caf"), "é", true, true, "café", true, true},
		{"typing space appends a space", typed("a"), "space", true, true, "a ", true, true},
		{"typing a literal space appends", typed("a"), " ", true, true, "a ", true, true},
		{"typing backspace deletes a rune", typed("café"), "backspace", true, true, "caf", true, true},
		{"typing backspace on empty is consumed", typed(""), "backspace", true, false, "", true, true},
		{"typing enter keeps the filter", typed("dep"), "enter", true, false, "dep", false, true},
		{"typing enter on empty goes idle", typed(""), "enter", true, false, "", false, false},
		{"typing esc clears", typed("dep"), "esc", true, true, "", false, false},
		{"typing esc on empty goes idle", typed(""), "esc", true, false, "", false, false},
		{"typing up moves the dialog", typed("d"), "up", false, false, "d", true, true},
		{"typing down moves the dialog", typed("d"), "down", false, false, "d", true, true},
		{"typing ctrl+n moves the dialog", typed("d"), "ctrl+n", false, false, "d", true, true},
		{"typing ctrl+p moves the dialog", typed("d"), "ctrl+p", false, false, "d", true, true},
		{"typing a named key is the dialog's", typed("d"), "tab", false, false, "d", true, true},
		{"typing a control rune is not text", typed("d"), "\x01", false, false, "d", true, true},

		{"kept esc clears", kept("dep"), "esc", true, true, "", false, false},
		{"kept / resumes typing", kept("dep"), "/", true, false, "dep", true, true},
		{"kept letter is the dialog's", kept("dep"), "q", false, false, "dep", false, true},
		{"kept enter is the dialog's", kept("dep"), "enter", false, false, "dep", false, true},
		{"kept backspace is the dialog's", kept("dep"), "backspace", false, false, "dep", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.start
			consumed, changed := f.HandleKey(c.key)
			if consumed != c.consumed || changed != c.changed {
				t.Errorf("HandleKey(%q) = (%v, %v), want (%v, %v)", c.key, consumed, changed, c.consumed, c.changed)
			}
			if f.Query() != c.query {
				t.Errorf("Query = %q, want %q", f.Query(), c.query)
			}
			if f.Typing() != c.typing {
				t.Errorf("Typing = %v, want %v", f.Typing(), c.typing)
			}
			if f.Active() != c.active {
				t.Errorf("Active = %v, want %v", f.Active(), c.active)
			}
		})
	}
}

func TestReset(t *testing.T) {
	f := typed("dep")
	f.Reset()
	if f.Active() || f.Query() != "" || !f.Matches("anything") {
		t.Errorf("after Reset: active %v, query %q", f.Active(), f.Query())
	}
}

func TestMatches(t *testing.T) {
	cases := []struct {
		query  string
		fields []string
		want   bool
	}{
		{"", nil, true},
		{"", []string{"x"}, true},
		{"dep", []string{"Deploy"}, true},
		{"DEP", []string{"deploy"}, true},
		{"cafe", []string{"Café"}, true},
		{"café", []string{"CAFE"}, true},
		{"loy", []string{"alpha", "deploy"}, true},
		{"zz", []string{"alpha", "deploy"}, false},
		{"zz", nil, false},
	}
	for _, c := range cases {
		f := typed(c.query)
		if got := f.Matches(c.fields...); got != c.want {
			t.Errorf("query %q Matches(%q) = %v, want %v", c.query, c.fields, got, c.want)
		}
	}
}

func testStyles() chrome.Styles {
	return chrome.Styles{
		Input:       lipgloss.NewStyle().BorderStyle(lipgloss.Border{Left: "▌"}).BorderLeft(true).PaddingLeft(1),
		Placeholder: lipgloss.NewStyle().Faint(true),
		Empty:       lipgloss.NewStyle().Italic(true),
		Footer:      lipgloss.NewStyle().Faint(true),
	}
}

func TestInput(t *testing.T) {
	s := testStyles()
	cases := []struct {
		name string
		f    Filter
		idle string
		want string
	}{
		{"typing empty shows the placeholder", typed(""), "hint", chrome.FilterInput(s, "", "Type to filter...")},
		{"typing shows the query", typed("dep"), "hint", chrome.FilterInput(s, "dep", "Type to filter...")},
		{"kept shows filter: query", kept("dep"), "hint", s.Empty.Render("filter: dep")},
		{"idle shows the hint", Filter{}, "hint", s.Footer.Render("hint")},
		{"idle without a hint draws no line", Filter{}, "", ""},
		{"cleared filter is idle again", func() Filter { f := kept("dep"); f.HandleKey("esc"); return f }(), "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.f.Input(s, "Type to filter...", c.idle); got != c.want {
				t.Errorf("Input = %q, want %q", got, c.want)
			}
		})
	}
}
