package session

import (
	"fmt"
	"testing"

	"shh-h/internal/apperror"
)

func TestTerminalStateTransitionsExhaustively(t *testing.T) {
	t.Parallel()

	states := []State{
		StateStarting,
		StateRunning,
		StateClosing,
		StateExited,
		StateFailed,
		StateClosed,
	}
	allowed := map[[2]State]bool{
		{StateStarting, StateRunning}: true,
		{StateStarting, StateClosing}: true,
		{StateStarting, StateExited}:  true,
		{StateStarting, StateFailed}:  true,
		{StateRunning, StateClosing}:  true,
		{StateRunning, StateExited}:   true,
		{StateRunning, StateFailed}:   true,
		{StateClosing, StateClosed}:   true,
		{StateExited, StateClosed}:    true,
		{StateFailed, StateClosed}:    true,
	}

	for _, current := range states {
		for _, next := range states {
			current, next := current, next
			t.Run(fmt.Sprintf("%s_to_%s", current, next), func(t *testing.T) {
				t.Parallel()
				result, err := transitionState(current, next)
				if allowed[[2]State{current, next}] {
					if err != nil || result != next {
						t.Fatalf("transition %q -> %q returned state %q and error %v", current, next, result, err)
					}
					return
				}
				if !apperror.IsCode(err, apperror.CodeConflict) {
					t.Fatalf("rejected transition %q -> %q error code = %q, want %q", current, next, apperror.CodeOf(err), apperror.CodeConflict)
				}
				if result != current {
					t.Fatalf("rejected transition %q -> %q changed state to %q", current, next, result)
				}
			})
		}
	}
}

func TestTerminalStateTransitionsRejectUnknownStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current State
		next    State
	}{
		{name: "unknown source", current: State("unknown"), next: StateClosed},
		{name: "unknown target", current: StateStarting, next: State("unknown")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := transitionState(test.current, test.next)
			if !apperror.IsCode(err, apperror.CodeConflict) || result != test.current {
				t.Fatalf("transition %q -> %q returned state %q and error %v", test.current, test.next, result, err)
			}
		})
	}
}
