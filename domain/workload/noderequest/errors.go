package noderequest

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Code says why a request was not answered, in a word every side knows.
type Code string

const (
	// CodeNotFound is a VM, or a docker object in one, that is not there.
	CodeNotFound Code = "not_found"

	// CodeNotRunning is a request only a running VM can answer, about one that
	// is not running.
	CodeNotRunning Code = "not_running"

	// CodeNotDocker is a docker request about a VM that is not a Docker VM.
	CodeNotDocker Code = "not_docker"

	// CodeDockerUnavailable is a Docker VM whose dockerd did not come up in the
	// time it was given.
	CodeDockerUnavailable Code = "docker_unavailable"

	// CodeInvalid is a request docker refused as it stood. The message is
	// docker's own.
	CodeInvalid Code = "invalid"

	// CodeTimeout is a request that took longer than it was allowed.
	CodeTimeout Code = "timeout"

	// CodeInternal is anything else.
	CodeInternal Code = "internal"
)

// Error is why a node did not answer a request.
//
// It is an error of its own, so it can be handed on as it arrived, and it is
// also the domain's errors under another name: errors.Is matches a not_found
// against domain.ErrNotExists, a not_running against vm.ErrNotRunning, and so
// on, so whoever reads one never has to know it crossed the wire.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if len(e.Message) == 0 {
		return string(e.Code)
	}

	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is reports whether the domain error target is what this error's code
// stands for.
func (e *Error) Is(target error) bool {
	for _, meaning := range meanings {
		if e.Code == meaning.code && target == meaning.err {
			return true
		}
	}

	return false
}

// meanings are the domain errors the codes stand for, in the order an error is
// checked against them. CodeInternal stands for none of them.
var meanings = []struct {
	code Code
	err  error
}{
	{code: CodeNotFound, err: domain.ErrNotExists},
	{code: CodeNotRunning, err: vm.ErrNotRunning},
	{code: CodeNotDocker, err: vm.ErrNotDocker},
	{code: CodeDockerUnavailable, err: docker.ErrUnavailable},
	{code: CodeInvalid, err: docker.ErrInvalid},
	{code: CodeTimeout, err: context.DeadlineExceeded},
}

// ErrorOf is what a node replies with when answering failed with err: the code
// the domain's error stands for, and what the error says. One that already is
// an Error is passed on as it is.
func ErrorOf(err error) *Error {
	if err == nil {
		return nil
	}

	var replied *Error
	if errors.As(err, &replied) {
		return replied
	}

	code := CodeInternal
	for _, meaning := range meanings {
		if errors.Is(err, meaning.err) {
			code = meaning.code

			break
		}
	}

	return &Error{Code: code, Message: err.Error()}
}

// Failed is the reply to a request that could not be answered because of err.
func Failed(err error) Reply {
	return Reply{Error: ErrorOf(err)}
}
