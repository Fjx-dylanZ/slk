package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Keys is the [keys] section: keybinding settings.
//
// Besides the leader it holds the action bindings: one *[]string field
// per Normal-mode action whose keys can be set, named by its toml key.
// Unset (nil) keeps the action's default keys and an empty list unbinds
// it. Load normalizes every key (normalizeKey) and rejects a key that
// two actions share. A new configurable action is a new field here plus
// its entry in the UI's action table (keyActions, internal/ui); cmd/slk
// tests that the two name the same actions.
type Keys struct {
	// Leader is the key that starts a leader sequence in normal mode
	// (<leader>w h, <leader>t, …). Empty keeps the UI's default, "\".
	// Load normalizes it to the string slk matches keys against (see
	// normalizeLeader) and rejects keys that cannot be a leader.
	Leader string `toml:"leader"`

	// SidebarOpen opens the selected sidebar row: a channel or DM, the
	// Threads or Activity view, or a section header, which it expands
	// or collapses. Default ["enter"].
	SidebarOpen *[]string `toml:"sidebar_open"`
	// SidebarToggleSection expands or collapses the section header
	// under the sidebar cursor. Default ["space"].
	SidebarToggleSection *[]string `toml:"sidebar_toggle_section"`
}

// keysAction is one action binding field of Keys and its [keys] name.
type keysAction struct {
	name string
	keys **[]string
}

// actions lists k's action binding fields in declaration order.
func (k *Keys) actions() []keysAction {
	v := reflect.ValueOf(k).Elem()
	var out []keysAction
	for i := range v.NumField() {
		if p, ok := v.Field(i).Addr().Interface().(**[]string); ok {
			name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("toml"), ",")
			out = append(out, keysAction{name: name, keys: p})
		}
	}
	return out
}

// KeyActionNames lists the [keys] names of the configurable actions.
func KeyActionNames() []string {
	var k Keys
	var names []string
	for _, a := range k.actions() {
		names = append(names, a.name)
	}
	return names
}

// Bindings returns the configured action keys by [keys] name. An unset
// action is absent; one set to [] maps to an empty, non-nil list.
func (k Keys) Bindings() map[string][]string {
	out := map[string][]string{}
	for _, a := range k.actions() {
		if *a.keys != nil {
			out[a.name] = append([]string{}, **a.keys...)
		}
	}
	return out
}

// normalize validates and normalizes the leader and every action
// binding in place. A key repeated within one action is kept once; a key
// in two actions is an error, since one key cannot do two things.
func (k *Keys) normalize() error {
	leader, err := normalizeLeader(k.Leader)
	if err != nil {
		return fmt.Errorf("[keys] leader: %w", err)
	}
	k.Leader = leader
	owner := map[string]string{} // normalized key → the action that has it
	for _, a := range k.actions() {
		if *a.keys == nil {
			continue
		}
		keys := []string{}
		for _, s := range **a.keys {
			key, err := normalizeBindingKey(s)
			if err != nil {
				return fmt.Errorf("[keys] %s: %w", a.name, err)
			}
			switch other, taken := owner[key]; {
			case !taken:
				owner[key] = a.name
				keys = append(keys, key)
			case other != a.name:
				return fmt.Errorf("[keys] %s: %q is also bound to %s; a key can do only one of them", a.name, key, other)
			}
		}
		*a.keys = &keys
	}
	return nil
}

// namedKeys are the key names a [keys] key may be besides a single
// character, as bubbletea spells them. esc is not one: it cancels.
var namedKeys = []string{
	"space", "enter", "tab", "backspace", "delete", "insert",
	"home", "end", "pgup", "pgdown", "up", "down", "left", "right",
}

// normalizeKey validates one [keys] key and returns it in the form slk
// matches keys against (bubbletea's key strings): a single printable
// character ("\\", ",", "l"), a named key (namedKeys), or a character or
// space after "ctrl+" and/or "alt+". Modifier and key-name spellings
// are case-insensitive, a ctrl+ letter is lower-cased, and " " means
// space.
//
// Rejected: anything else, including words, shift+ (write the shifted
// character) and a modifier on a named key other than space; esc (it
// cancels sequences and dialogs); ctrl+c (slk intercepts it before
// normal mode); and keys a terminal without keyboard enhancements sends
// as some other key, so the binding would silently never fire: ctrl+
// with anything but a letter, space or one of \ ] ^ _ (ctrl+[ is esc,
// ctrl+@ is ctrl+space, ctrl+, is a plain comma or nothing), ctrl+i
// (tab), ctrl+m (enter), and alt+ with an uppercase letter (alt+shift+y,
// alt+Y or alt+shift+Y by terminal).
func normalizeKey(s string) (string, error) {
	if s == " " {
		return "space", nil
	}
	var ctrl, alt bool
	base := s
	// Split modifiers off the front. A "+" right at the start or end is
	// the key itself ("+", "ctrl++"), not a separator.
	for {
		i := strings.Index(base, "+")
		if i <= 0 || i == len(base)-1 {
			break
		}
		switch strings.ToLower(base[:i]) {
		case "ctrl":
			ctrl = true
		case "alt":
			alt = true
		default:
			return "", fmt.Errorf("%q: unsupported modifier %q (use ctrl+ or alt+, or the shifted character itself)", s, base[:i])
		}
		base = base[i+1:]
	}
	switch name := strings.ToLower(base); {
	case name == "esc" || name == "escape":
		return "", fmt.Errorf("%q: esc cancels sequences and dialogs, so it cannot do anything else", s)
	case name == "space":
		base = "space"
	case slices.Contains(namedKeys, name):
		if ctrl || alt {
			return "", fmt.Errorf("%q: modifiers go with a character or space, not with %s", s, name)
		}
		base = name
	case utf8.RuneCountInString(base) == 1:
		r, _ := utf8.DecodeRuneInString(base)
		if !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return "", fmt.Errorf("%q is not a printable key", s)
		}
		if ctrl {
			// Terminals report a shifted ctrl letter as ctrl+shift+x,
			// never ctrl+X, so ctrl+X would never match.
			base = strings.ToLower(base)
		}
	default:
		return "", fmt.Errorf("%q is not a single key: use one character such as \"\\\\\" or \",\", a key name such as \"space\" or \"enter\", or a character or space after ctrl+ or alt+", s)
	}
	var key string
	if ctrl {
		key += "ctrl+"
	}
	if alt {
		key += "alt+"
	}
	key += base
	switch {
	case key == "ctrl+c":
		return "", fmt.Errorf("%q: ctrl+c quits from every mode, so it cannot do anything else", s)
	case ctrl && base == "i":
		return "", fmt.Errorf("%q: most terminals send ctrl+i as tab, so it would never arrive", s)
	case ctrl && base == "m":
		return "", fmt.Errorf("%q: most terminals send ctrl+m as enter, so it would never arrive", s)
	case ctrl && base != "space" && !isCtrlCodeKey(base):
		return "", fmt.Errorf("%q: most terminals cannot send ctrl+%s as itself; with ctrl+ use a letter, space or one of \\ ] ^ _", s, base)
	case alt && !ctrl && unicode.IsUpper([]rune(base)[0]):
		return "", fmt.Errorf("%q: terminals report alt with a shifted letter differently (alt+shift+%s, alt+%s, …); use alt+%s or another key",
			s, strings.ToLower(base), base, strings.ToLower(base))
	}
	return key, nil
}

// isCtrlCodeKey reports whether ctrl+k reaches slk as ctrl+k from any
// terminal: k has its own control code (a letter, or one of \ ] ^ _).
func isCtrlCodeKey(k string) bool {
	if len(k) != 1 {
		return false
	}
	c := k[0]
	return (c >= 'a' && c <= 'z') || strings.IndexByte(`\]^_`, c) >= 0
}

// normalizeLeader validates a [keys] leader value (normalizeKey's rules)
// and returns it normalized; "" stays "" (unset). On top of
// normalizeKey's rejections a leader may not be a named key other than
// space (tab, enter, the arrows… keep their own jobs), nor 1–9 (they
// switch workspaces and have no binding a doubled leader could hand
// back).
func normalizeLeader(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	key, err := normalizeKey(s)
	if err != nil {
		return "", err
	}
	switch {
	case key != "space" && slices.Contains(namedKeys, key):
		return "", fmt.Errorf("%q: a leader is a character or space (after ctrl+ or alt+ if you like), not a named key", s)
	case len(key) == 1 && key[0] >= '1' && key[0] <= '9':
		return "", fmt.Errorf("%q: 1-9 switch workspaces", s)
	}
	return key, nil
}

// normalizeBindingKey validates one key of an action binding
// (normalizeKey's rules) and returns it normalized. ctrl+w is refused
// too: it starts window commands before any action sees it.
func normalizeBindingKey(s string) (string, error) {
	if s == "" {
		return "", errors.New("an empty key")
	}
	key, err := normalizeKey(s)
	if err != nil {
		return "", err
	}
	if key == "ctrl+w" {
		return "", fmt.Errorf("%q: ctrl+w starts window commands, so it cannot do anything else", s)
	}
	return key, nil
}
