package domain

import "errors"

var (
	ErrNotExists     = errors.New("not exists")
	ErrAlreadyExists = errors.New("already exists")

	// ErrForbidden is what somebody may not do to this particular thing. A
	// route lets through anybody holding either the permission over all of
	// them or the one over their own; this is the second of those meeting
	// something that is not theirs.
	ErrForbidden = errors.New("forbidden")
)
