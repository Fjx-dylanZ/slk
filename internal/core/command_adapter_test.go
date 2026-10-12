package core

import (
	"errors"
	"testing"

	"github.com/gammons/slk/internal/ids"
)

func TestCommandServiceNilFuncIsUnsupported(t *testing.T) {
	err := NewCommandService(CommandServiceFuncs{}).Run("C1", "", "/giphy", "cats")
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}

func TestCommandServicePassesArguments(t *testing.T) {
	var got []any
	svc := NewCommandService(CommandServiceFuncs{
		Run: func(ch ids.ChannelID, th ids.ThreadTS, cmd, args string) error {
			got = []any{ch, th, cmd, args}
			return nil
		},
	})
	if err := svc.Run("C1", "10.0", "/giphy", "cats"); err != nil {
		t.Fatal(err)
	}
	want := []any{ids.ChannelID("C1"), ids.ThreadTS("10.0"), "/giphy", "cats"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCommandErrorMessageIsTheCode(t *testing.T) {
	if got := (&CommandError{Code: "unknown_command"}).Error(); got != "unknown_command" {
		t.Errorf("Error() = %q, want unknown_command", got)
	}
}
