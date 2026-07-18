package session

import (
	"fmt"

	"shh-h/internal/apperror"
)

func transitionState(current, next State) (State, error) {
	if !allowsStateTransition(current, next) {
		return current, apperror.New(
			apperror.CodeConflict,
			fmt.Sprintf("Terminal session cannot transition from %q to %q.", current, next),
		)
	}
	return next, nil
}

func allowsStateTransition(current, next State) bool {
	switch current {
	case StateStarting:
		return next == StateRunning || next == StateClosing || next == StateExited || next == StateFailed
	case StateRunning:
		return next == StateClosing || next == StateExited || next == StateFailed
	case StateClosing:
		return next == StateClosed
	case StateExited, StateFailed:
		return next == StateClosed
	default:
		return false
	}
}
