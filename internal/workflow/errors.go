// Package workflow moves submissions through their workflow's state machine.
package workflow

import "errors"

var (
	ErrUnknownTransition = errors.New("unknown transition")
	ErrInvalidState      = errors.New("transition is not allowed from the submission's current state")
	ErrForbidden         = errors.New("you are not allowed to perform this transition")
	ErrNoWorkflow        = errors.New("submission's form has no workflow")
	ErrStateConflict     = errors.New("submission state changed since it was loaded")
)
