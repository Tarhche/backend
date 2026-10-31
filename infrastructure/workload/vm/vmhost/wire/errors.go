package wire

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

var (
	// ErrInvalid is a request a vmhost could not read: a body or a header
	// that is not what the API speaks. It is a mistake on the asking side,
	// so asking again the same way fails the same way.
	ErrInvalid = errors.New("the request cannot be read")

	// ErrUnavailable is a vmhost that is not answering: its socket is not
	// there, nothing is listening on it, or it is shutting down.
	ErrUnavailable = errors.New("the vmhost is not answering")
)

// Code says why a vmhost did not do what it was asked, in a word both sides
// know.
type Code string

const (
	CodeNotFound       Code = "not_found"
	CodeAlreadyExists  Code = "already_exists"
	CodeNotRunning     Code = "not_running"
	CodeNotDocker      Code = "not_docker"
	CodeNoCapacity     Code = "no_capacity"
	CodeEngineMismatch Code = "engine_mismatch"
	CodeQuotaExceeded  Code = "quota_exceeded"
	CodeInvalid        Code = "invalid"
	CodeTimeout        Code = "timeout"
	CodeCanceled       Code = "canceled"
	CodeUnavailable    Code = "unavailable"

	// CodeInternal is anything else.
	CodeInternal Code = "internal"
)

// statusClientClosedRequest is what a request whose client went away is
// answered with, as nginx calls it. Nobody reads it: the client is gone.
const statusClientClosedRequest = 499

// meanings are the errors each code stands for, in the order an error is
// checked against them, and the status a response with the code has.
// CodeInternal stands for none of them.
var meanings = []struct {
	code   Code
	err    error
	status int
}{
	{code: CodeNotFound, err: domain.ErrNotExists, status: http.StatusNotFound},
	{code: CodeAlreadyExists, err: domain.ErrAlreadyExists, status: http.StatusConflict},
	{code: CodeNotRunning, err: vm.ErrNotRunning, status: http.StatusConflict},
	{code: CodeNotDocker, err: vm.ErrNotDocker, status: http.StatusConflict},
	{code: CodeNoCapacity, err: vm.ErrNoCapacity, status: http.StatusInsufficientStorage},
	{code: CodeEngineMismatch, err: vm.ErrEngineMismatch, status: http.StatusUnprocessableEntity},
	{code: CodeQuotaExceeded, err: vm.ErrQuotaExceeded, status: http.StatusForbidden},
	{code: CodeInvalid, err: ErrInvalid, status: http.StatusBadRequest},
	{code: CodeTimeout, err: context.DeadlineExceeded, status: http.StatusGatewayTimeout},
	{code: CodeCanceled, err: context.Canceled, status: statusClientClosedRequest},
	{code: CodeUnavailable, err: ErrUnavailable, status: http.StatusServiceUnavailable},
}

// Error is why a vmhost did not do what it was asked.
//
// It is the domain's errors under another name: errors.Is matches a not_found
// against domain.ErrNotExists, a no_capacity against vm.ErrNoCapacity and so
// on, so whoever holds one never has to know it crossed the socket.
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

// Is reports whether target is the error this error's code stands for.
func (e *Error) Is(target error) bool {
	for _, meaning := range meanings {
		if e.Code == meaning.code && target == meaning.err {
			return true
		}
	}

	return false
}

// Status is the HTTP status a response carrying this error has.
func (e *Error) Status() int {
	for _, meaning := range meanings {
		if e.Code == meaning.code {
			return meaning.status
		}
	}

	return http.StatusInternalServerError
}

// ErrorOf is what a vmhost answers when doing what it was asked failed with
// err: the code the error stands for, and what the error says. One that
// already is an Error is passed on as it is.
func ErrorOf(err error) *Error {
	if err == nil {
		return nil
	}

	var answered *Error
	if errors.As(err, &answered) {
		return answered
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
