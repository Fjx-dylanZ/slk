// Package listfilter is the "/" filter of a list dialog that does not
// filter as you type: the query, whether the user is typing it, which
// keys it takes, the match, and the dialog's filter line. It holds no
// rows. A dialog keeps its own rows, selection and keys, offers each key
// to HandleKey first, refilters its rows through Matches when the query
// changes, and draws Input as its chrome.Frame.Input line.
//
// Keys arrive as the strings the dialogs already receive ("/", "enter",
// "esc", "backspace", "space", "up", single printable runes, …):
//
//	idle         "/"                  start typing
//	typing       printable rune       appended (including "/", "q", "j")
//	typing       "space"              a space appended
//	typing       "backspace"          deletes the last rune
//	typing       "enter"              stop typing, keep the filter
//	typing       "esc"                stop typing, clear the filter
//	filter kept  "/"                  resume typing the kept query
//	filter kept  "esc"                clear the filter
//
// Every other key, including up/down/ctrl+n/ctrl+p while typing, is left
// to the dialog, so its selection moves through the filtered rows.
package listfilter

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/text"
)

// Filter is one dialog's filter state. The zero value is idle with an
// empty query.
type Filter struct {
	query  string
	folded string // text.Fold(query), so Matches folds the query once
	typing bool
}

// HandleKey applies key to the filter. consumed reports that the key was
// the filter's and the dialog must not act on it; changed reports that
// the query changed, so the dialog refilters its rows and resets its
// selection.
func (f *Filter) HandleKey(key string) (consumed, changed bool) {
	if !f.typing {
		switch {
		case key == "/":
			f.typing = true
			return true, false
		case key == "esc" && f.query != "":
			f.setQuery("")
			return true, true
		}
		return false, false
	}
	switch key {
	case "enter":
		f.typing = false
		return true, false
	case "esc":
		f.typing = false
		if f.query == "" {
			return true, false
		}
		f.setQuery("")
		return true, true
	case "backspace":
		if f.query == "" {
			return true, false
		}
		_, size := utf8.DecodeLastRuneInString(f.query)
		f.setQuery(f.query[:len(f.query)-size])
		return true, true
	case "space":
		f.setQuery(f.query + " ")
		return true, true
	}
	if r, size := utf8.DecodeRuneInString(key); size == len(key) && r != utf8.RuneError && unicode.IsPrint(r) {
		f.setQuery(f.query + key)
		return true, true
	}
	return false, false
}

func (f *Filter) setQuery(q string) {
	f.query = q
	f.folded = text.Fold(q)
}

// Query is the filter text, typed or kept.
func (f Filter) Query() string { return f.query }

// Typing reports whether the user is typing the query: printable keys
// are filter text, not the dialog's commands.
func (f Filter) Typing() bool { return f.typing }

// Active reports whether the filter is being typed or a query is kept,
// which is when Input draws the filter line.
func (f Filter) Active() bool { return f.typing || f.query != "" }

// Reset returns the filter to idle with an empty query, for a dialog
// that opens or closes.
func (f *Filter) Reset() { *f = Filter{} }

// Matches reports whether any of fields contains the query, case- and
// accent-insensitively (text.Fold). An empty query matches everything.
func (f Filter) Matches(fields ...string) bool {
	if f.folded == "" {
		return true
	}
	for _, field := range fields {
		if strings.Contains(text.Fold(field), f.folded) {
			return true
		}
	}
	return false
}

// Input is the dialog's chrome.Frame.Input line: the filter input with
// placeholder while typing, "filter: <query>" in s.Empty while a query
// is kept, and otherwise idle in s.Footer — or no line when idle is "".
func (f Filter) Input(s chrome.Styles, placeholder, idle string) string {
	switch {
	case f.typing:
		return chrome.FilterInput(s, f.query, placeholder)
	case f.query != "":
		return s.Empty.Render("filter: " + f.query)
	case idle != "":
		return s.Footer.Render(idle)
	}
	return ""
}
