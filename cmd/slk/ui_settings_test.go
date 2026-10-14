package main

import (
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
