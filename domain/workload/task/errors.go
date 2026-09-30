package task

import "errors"

var (
	ErrInvalidStateTransition = errors.New("invalid state transition")
)
