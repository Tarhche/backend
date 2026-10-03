package task

import "errors"

var (
	ErrInvalidStateTransition = errors.New("invalid state transition")

	// ErrNotSupported is a runtime asked for something it cannot do — dial a
	// run it publishes ports for instead, say — as opposed to something that
	// went wrong while it tried.
	ErrNotSupported = errors.New("not supported by this runtime")
)
