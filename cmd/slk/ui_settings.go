package main

import (
	"time"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/styles"
	versionpkg "github.com/gammons/slk/internal/version"
)

// uiSettings is the display configuration run() and runDemo() both hand
// the App. It carries values rather than the config file so that demo
// mode can build one without loading any config.
type uiSettings struct {
	TimestampFormat  string
	TypingIndicators bool
	StaleAfter       time.Duration
	MouseWheelLines  int
	ColoredUsernames bool
	ThemeOverrides   core.Theme
	// Leader is [keys] leader, already normalized by config.Load; ""
	// keeps the App's default.
	Leader string
	// KeyBindings are the [keys] action bindings by name
	// (config.Keys.Bindings): an absent action keeps its default keys,
	// an empty list unbinds it.
	KeyBindings map[string][]string
	// ComposeVimMode is [compose] vim_mode: vim editing in the composer.
	ComposeVimMode bool
	// LeaderLayer is the effective leader layer config.Load resolved
	// from [keys.leader_map] (config.Keys.LeaderLayer); nil keeps the
	// App's default layer.
	LeaderLayer []core.LeaderSeq
	// LeaderGroups labels the leader layer's groups by sequence, as
	// config.Load resolved [keys.leader_groups]
	// (config.Keys.GroupLabels); nil keeps the App's default labels.
	LeaderGroups map[string]string
	// KeyHints is [keys] key_hints, the key-hints menu; nil keeps the
	// App's default (on).
	KeyHints *bool
	// KeyHintsDelay is [keys] key_hints_delay; nil keeps the App's
	// default.
	KeyHintsDelay *time.Duration
}

// applyUISettings installs s on app. The timestamp formatter feeds the
// optimistic instant-display path on send: the App mints a placeholder
// before the chat.postMessage round-trip and needs a Timestamp string
// that renders identically to messages arriving through the load path.
func applyUISettings(app *ui.App, s uiSettings) {
	app.SetHelpFooter(versionpkg.ModalFooter(version))
	app.SetNowTimestampFormatter(func() string {
		return time.Now().Format(s.TimestampFormat)
	})
	app.SetTypingEnabled(s.TypingIndicators)
	app.SetSidebarStaleThreshold(s.StaleAfter)
	app.SetMouseWheelLines(s.MouseWheelLines)
	app.SetColoredUsernames(s.ColoredUsernames)
	app.SetThemeItems(styles.ThemeNames())
	app.SetThemeOverrides(s.ThemeOverrides)
	app.SetKeyBindings(s.KeyBindings)
	app.SetLeaderLayer(s.LeaderLayer)
	app.SetLeaderGroups(s.LeaderGroups)
	app.SetLeader(s.Leader)
	app.SetComposeVimMode(s.ComposeVimMode)
	if s.KeyHints != nil {
		app.SetKeyHintsEnabled(*s.KeyHints)
	}
	if s.KeyHintsDelay != nil {
		app.SetKeyHintsDelay(*s.KeyHintsDelay)
	}
}
