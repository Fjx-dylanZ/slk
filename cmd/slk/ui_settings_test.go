package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/demo"
	"github.com/gammons/slk/internal/ui"
)

// [keys] leader reaches the App through uiSettings: the configured key
// starts a leader sequence, whose hint the status bar draws.
func TestApplyUISettingsInstallsTheLeader(t *testing.T) {
	d, err := demo.New("hero", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app := ui.NewApp()
	applyUISettings(app, uiSettings{
		TimestampFormat: demo.TimestampFormat,
		MouseWheelLines: 3,
		Leader:          "space",
	})
	d.Install(app)
	deliverDemo(t, app, tea.WindowSizeMsg{Width: 200, Height: 40}, 0)
	for _, msg := range d.StartupMsgs() {
		deliverDemo(t, app, msg, 0)
	}
	deliverDemo(t, app, tea.KeyPressMsg{Code: ' ', Text: " "}, 0)
	frame := ansi.Strip(app.View().Content)
	if want := "space …  space toggle section  w window"; !strings.Contains(frame, want) {
		t.Fatalf("frame lacks the leader hint %q:\n%s", want, frame)
	}
}

// config.Keys and the App's action table must name the same
// configurable actions: a [keys] field the UI does not know would be
// accepted and then silently ignored.
func TestKeyActionsAgreeWithConfig(t *testing.T) {
	got, want := ui.KeyActionNames(), config.KeyActionNames()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("ui actions %q, config [keys] actions %q", got, want)
	}
}

// [keys] action bindings reach the App through uiSettings: with
// sidebar_open = ["enter", "l"], l on a sidebar channel opens it (by
// default l would only move focus, leaving #general open).
func TestApplyUISettingsInstallsKeyBindings(t *testing.T) {
	d, err := demo.New("hero", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app := ui.NewApp()
	applyUISettings(app, uiSettings{
		TimestampFormat: demo.TimestampFormat,
		MouseWheelLines: 3,
		KeyBindings:     map[string][]string{"sidebar_open": {"enter", "l"}},
	})
	d.Install(app)
	deliverDemo(t, app, tea.WindowSizeMsg{Width: 200, Height: 40}, 0)
	for _, msg := range d.StartupMsgs() {
		deliverDemo(t, app, msg, 0)
	}
	// \ w h focuses the sidebar (cursor on #general); j moves to #design.
	for _, r := range `\whjl` {
		deliverDemo(t, app, tea.KeyPressMsg{Code: r, Text: string(r)}, 0)
	}
	lines := strings.Split(ansi.Strip(app.View().Content), "\n")
	status := lines[len(lines)-1]
	if !strings.Contains(status, "#design") {
		t.Fatalf("status bar %q: l did not open #design", status)
	}
}

// nvimExampleFromWiki returns the toml block wiki/Configuration.md shows
// under "### Neovim-style keys", so the test below runs what the wiki
// promises, verbatim.
func nvimExampleFromWiki(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "wiki", "Configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(data), "### Neovim-style keys")
	if ok {
		_, rest, ok = strings.Cut(rest, "```toml\n")
	}
	block, _, closed := strings.Cut(rest, "```")
	if !ok || !closed || !strings.Contains(block, "[keys.leader_map]") {
		t.Fatal(`wiki/Configuration.md has no "### Neovim-style keys" toml block with a [keys.leader_map]`)
	}
	return block
}

// The wiki's nvim-style [keys] example loads and does what the wiki
// says, through config.Load, uiSettings and the App's Update chain.
func TestNvimExampleConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(nvimExampleFromWiki(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("the wiki's example does not load: %v", err)
	}
	d, err := demo.New("hero", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app := ui.NewApp()
	applyUISettings(app, uiSettings{
		TimestampFormat: demo.TimestampFormat,
		MouseWheelLines: 3,
		Leader:          cfg.Keys.Leader,
		KeyBindings:     cfg.Keys.Bindings(),
		LeaderLayer:     cfg.Keys.LeaderLayer(),
	})
	d.Install(app)
	deliverDemo(t, app, tea.WindowSizeMsg{Width: 200, Height: 40}, 0)
	for _, msg := range d.StartupMsgs() {
		deliverDemo(t, app, msg, 0)
	}
	press := func(keys ...string) {
		for _, k := range keys {
			switch k {
			case "space":
				deliverDemo(t, app, tea.KeyPressMsg{Code: ' ', Text: " "}, 0)
			case "esc":
				deliverDemo(t, app, tea.KeyPressMsg{Code: tea.KeyEscape}, 0)
			case "enter":
				deliverDemo(t, app, tea.KeyPressMsg{Code: tea.KeyEnter}, 0)
			default:
				r := []rune(k)[0]
				deliverDemo(t, app, tea.KeyPressMsg{Code: r, Text: k}, 0)
			}
		}
	}
	frame := func() string { return ansi.Strip(app.View().Content) }
	expect := func(step, want string, present bool) {
		t.Helper()
		if got := strings.Contains(frame(), want); got != present {
			t.Fatalf("%s: frame contains %q = %v, want %v:\n%s", step, want, got, present, frame())
		}
	}

	press("space")
	expect("space", "space …  w window  f …  space channel  / search  s …", true)
	press("f", "f")
	expect("space f f", "Switch Channel", true)
	press("esc", "space", "space")
	expect("space space", "Switch Channel", true)
	press("esc", "space", "?")
	expect("space ?", "Keybindings", true)
	press("esc")
	expect("start", "⚑ Threads", true)
	press("space", "e")
	expect("space e", "⚑ Threads", false)
	press("space", "e", "enter")
	expect("enter in the messages pane", "› Thread", true)
	press("space", "q")
	expect("space q", "› Thread", false)
}
