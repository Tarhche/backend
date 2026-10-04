package runs

import (
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// The supervisor's failures are the contract's own errors, a code a client
// acts on and a message for people, so that what reaches the client is what
// the supervisor meant rather than something the server guessed at. Anything
// else that goes wrong is internal, and its text is the message.

// newError is a failure with one of the contract's codes.
func newError(code string, format string, args ...any) error {
	return &api.Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func notFound(id string) error {
	return newError(api.CodeNotFound, "there is no run %s", id)
}

func notRunning(id string) error {
	return newError(api.CodeNotRunning, "run %s is not running", id)
}

// Code is the contract's code for what went wrong: the code of the api.Error
// it wraps, and internal for anything else.
func Code(err error) string {
	var apiError *api.Error
	if errors.As(err, &apiError) {
		return apiError.Code
	}

	return api.CodeInternal
}
