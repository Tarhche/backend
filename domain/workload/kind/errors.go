package kind

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
)

var (
	// ErrUnknownKind is a kind nothing here is registered for, or a resource
	// of one kind handed to another's strategy.
	ErrUnknownKind = errors.New("unknown kind")

	// ErrUnknownAction is an action a kind does not have, or one that does
	// not run where it was asked: a node's command asked of the control plane,
	// a query sent as a command.
	ErrUnknownAction = errors.New("unknown action")

	// ErrInvalidPayload is what somebody asked with that cannot be used: a
	// payload that cannot be read as its action's or was read and refused, a
	// spec that cannot be read as its kind's, a command or a query carrying
	// another resource than it names. What wraps it says which.
	ErrInvalidPayload = errors.New("invalid payload")

	// ErrUnreachable is an instance that is there and cannot be reached now:
	// not running, or not on a node yet. What wraps it says which.
	ErrUnreachable = errors.New("not reachable")

	// ErrUnseen is an instance whose node could not look where it would be,
	// because the parent it lives in did not answer, so whether it is there is
	// not known.
	ErrUnseen = errors.New("could not be seen")
)

// InvalidError is a payload that was read and refused, with what is wrong
// with each field. It is ErrInvalidPayload as far as errors.Is is concerned,
// so whoever only needs to know that it was refused need not know why.
type InvalidError struct {
	Errors domain.ValidationErrors
}

func (e *InvalidError) Error() string {
	fields := slices.Sorted(maps.Keys(e.Errors))

	described := make([]string, len(fields))
	for i, field := range fields {
		described[i] = field + ": " + e.Errors[field]
	}

	return ErrInvalidPayload.Error() + ": " + strings.Join(described, ", ")
}

// Is reports whether target is ErrInvalidPayload, which every refusal is.
func (e *InvalidError) Is(target error) bool {
	return target == ErrInvalidPayload
}
