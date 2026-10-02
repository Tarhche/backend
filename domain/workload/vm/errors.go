package vm

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/khanzadimahdi/testproject/domain"
)

var (
	// ErrNotFound is a VM, image, network or command vmhost does not hold.
	ErrNotFound = fmt.Errorf("%w: vmhost holds no such thing", domain.ErrNotExists)

	// ErrInvalid is a request that cannot be carried out as it is written.
	ErrInvalid = errors.New("the request is not one vmhost can carry out")

	// ErrConflict is a request that clashes with what vmhost holds: a name
	// another VM answers to, a network asked for with the other answer about
	// routing out.
	ErrConflict = errors.New("the request clashes with what vmhost holds")

	// ErrCapacity is a VM vmhost has no room for: its memory, CPU or disk
	// would go past vmhost's budget. Another node may have room.
	ErrCapacity = errors.New("vmhost has no room for this vm")

	// ErrNotRunning is a VM asked for what only a running one can do.
	ErrNotRunning = errors.New("the vm is not running")

	// ErrNetworkInUse is a network that still has VMs plugged into it.
	ErrNetworkInUse = errors.New("the network still has vms plugged into it")

	// ErrImage is an image that cannot be made into a disk: it cannot be
	// pulled, it has no variant for the host's platform, its registry is not
	// one images may come from, or converting it failed.
	ErrImage = errors.New("the image cannot be made into a disk")

	// ErrUnavailable is vmhost unable to run VMs at all right now: no KVM,
	// no VMM, a fabric it cannot set up.
	ErrUnavailable = errors.New("vmhost cannot run vms right now")
)

// The codes vmhost's errors travel under, so that what the driver gets back
// is the error vmhost had, rather than a status code to guess from.
const (
	CodeNotFound     = "not_found"
	CodeInvalid      = "invalid"
	CodeConflict     = "conflict"
	CodeCapacity     = "capacity"
	CodeNotRunning   = "not_running"
	CodeNetworkInUse = "network_in_use"
	CodeImage        = "image"
	CodeUnavailable  = "unavailable"
	CodeInternal     = "internal"
)

// ErrorResponse is how vmhost answers a request it could not carry out.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// codes are the errors that travel, their codes and the statuses they are
// answered with.
var codes = []struct {
	err    error
	code   string
	status int
}{
	{err: ErrNotFound, code: CodeNotFound, status: http.StatusNotFound},
	{err: ErrInvalid, code: CodeInvalid, status: http.StatusBadRequest},
	{err: ErrConflict, code: CodeConflict, status: http.StatusConflict},
	{err: ErrCapacity, code: CodeCapacity, status: http.StatusConflict},
	{err: ErrNotRunning, code: CodeNotRunning, status: http.StatusConflict},
	{err: ErrNetworkInUse, code: CodeNetworkInUse, status: http.StatusConflict},
	{err: ErrImage, code: CodeImage, status: http.StatusUnprocessableEntity},
	{err: ErrUnavailable, code: CodeUnavailable, status: http.StatusServiceUnavailable},
}

// Describe is how vmhost answers an error: its status, and the code and
// message the driver reads it back from. An error that is none of the ones
// above is vmhost's own failure.
func Describe(err error) (int, ErrorResponse) {
	for _, c := range codes {
		if errors.Is(err, c.err) {
			return c.status, ErrorResponse{Code: c.code, Message: err.Error()}
		}
	}

	return http.StatusInternalServerError, ErrorResponse{Code: CodeInternal, Message: err.Error()}
}

// Err is the error an answer carries, as the driver reads it back: the same
// one vmhost had, so errors.Is(err, ErrCapacity) holds on either side.
func (r ErrorResponse) Err() error {
	for _, c := range codes {
		if r.Code == c.code {
			return fmt.Errorf("%w: %s", c.err, r.Message)
		}
	}

	return fmt.Errorf("vmhost failed: %s", r.Message)
}
