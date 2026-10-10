package core

import (
	"errors"
	"testing"

	"github.com/gammons/slk/internal/core/blocks"
	"github.com/gammons/slk/internal/ids"
)

func TestInteractionServiceNilFuncIsUnsupported(t *testing.T) {
	err := NewInteractionService(InteractionServiceFuncs{}).PressAttachmentAction("C1", "1.0", 1, "cb", blocks.LegacyAction{}, true)
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}

func TestInteractionServicePassesArguments(t *testing.T) {
	var got []any
	svc := NewInteractionService(InteractionServiceFuncs{
		PressAttachmentAction: func(ch ids.ChannelID, ts ids.MessageTS, attID int, cb string, act blocks.LegacyAction, eph bool) error {
			got = []any{ch, ts, attID, cb, act.Name, eph}
			return nil
		},
	})
	if err := svc.PressAttachmentAction("C1", "1.0", 7, "cb", blocks.LegacyAction{Name: "ignore"}, true); err != nil {
		t.Fatal(err)
	}
	want := []any{ids.ChannelID("C1"), ids.MessageTS("1.0"), 7, "cb", "ignore", true}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %v, want %v", i, got[i], want[i])
		}
	}
}
