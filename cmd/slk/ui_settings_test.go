package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
