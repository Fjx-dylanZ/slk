package config

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Keys is the [keys] section: keybinding settings.
type Keys struct {
	// Leader is the key that starts a leader sequence in normal mode
	// (<leader>w h, <leader>t, …). Empty keeps the UI's default, "\".
	// Load normalizes it to the string slk matches keys against (see
	// normalizeLeader) and rejects keys that cannot be a leader.
	Leader string `toml:"leader"`
}

// normalizeLeader validates a [keys] leader value and returns it in the
// form slk matches keys against (bubbletea's key strings): a single
// printable character ("\\", ",", ";"), "space", or either of those
// after "ctrl+" and/or "alt+". Modifier and "space" spellings are
// case-insensitive, a ctrl+ letter is lower-cased, " " means space, and
// "" stays "" (unset).
//
// Rejected: any other named key or word (esc cancels a sequence, and
// tab, enter, f1… are not offered), shift+ (write the shifted
// character), 1–9 (they switch workspaces and have no binding a doubled
// leader could hand back), ctrl+c (slk intercepts it before normal
// mode, so it could never start a sequence), and keys a terminal
// without keyboard enhancements sends as some other key, so the leader
// would silently never fire: ctrl+ with anything but a letter, space or
// one of \ ] ^ _ (ctrl+[ is esc, ctrl+@ is ctrl+space, ctrl+, is a plain
// comma or nothing), ctrl+i (tab), ctrl+m (enter), and alt+ with an
// uppercase letter (alt+shift+y, alt+Y or alt+shift+Y by terminal).
func normalizeLeader(s string) (string, error) {
	switch s {
	case "":
		return "", nil
	case " ":
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
	switch {
	case strings.EqualFold(base, "space"):
		base = "space"
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
		return "", fmt.Errorf("%q is not a single key: use one character such as \"\\\\\" or \",\", \"space\", or either after ctrl+ or alt+", s)
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
	case !ctrl && !alt && len(base) == 1 && base[0] >= '1' && base[0] <= '9':
		return "", fmt.Errorf("%q: 1-9 switch workspaces", s)
	case key == "ctrl+c":
		return "", fmt.Errorf("%q: ctrl+c quits from every mode, so it cannot start a sequence", s)
	case ctrl && base == "i":
		return "", fmt.Errorf("%q: most terminals send ctrl+i as tab, so it would never start a sequence", s)
	case ctrl && base == "m":
		return "", fmt.Errorf("%q: most terminals send ctrl+m as enter, so it would never start a sequence", s)
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
