// Package api serves vmhost's API (domain/workload/vm/api.go) on its unix
// socket: HTTP and JSON shaped like the docker engine's, so the orchestrator's
// microvm driver reads like the container driver does, and streams — a
// command's terminal, a connection to a task's port — carried on a connection
// that switches protocols once the VM's agent has agreed, as docker's attach
// is.
//
// Only an orchestrator on the same host reaches the socket, so an answer that
// is a failure says why: a refusal travels as the error vmhost had, under its
// code (vm.Describe), and the driver reads the same error back
// (vm.ErrorResponse.Err), so errors.Is(err, vm.ErrCapacity) holds on either
// side of the socket. A request that is not valid is answered as vm.ErrInvalid
// is, with the fields that are not.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// maxBody bounds what vmhost reads of a request: the largest is a VM's spec.
const maxBody = 1 << 20

// reply answers what was asked with v, as JSON.
func reply(rw http.ResponseWriter, status int, v any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(v)
}

// done answers that what was asked was done, with nothing to say about it.
func done(rw http.ResponseWriter) {
	rw.WriteHeader(http.StatusNoContent)
}

// failed answers an error the way vmhost's errors travel: the status and the
// code it is read back from. A failure of vmhost's own is recorded on the
// request's span.
func failed(rw http.ResponseWriter, r *http.Request, err error) {
	status, answer := vm.Describe(err)

	if status >= http.StatusInternalServerError {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
	}

	reply(rw, status, answer)
}

// refused answers a request that is not valid, naming what is not.
func refused(rw http.ResponseWriter, r *http.Request, validationErrors domain.ValidationErrors) {
	fields := make([]string, 0, len(validationErrors))
	for field := range validationErrors {
		fields = append(fields, field)
	}

	slices.Sort(fields)

	reasons := make([]string, 0, len(fields))
	for _, field := range fields {
		reasons = append(reasons, field+": "+validationErrors[field])
	}

	failed(rw, r, fmt.Errorf("%w: %s", vm.ErrInvalid, strings.Join(reasons, "; ")))
}

// decode reads a request's JSON body into v. A body that is not there leaves v
// as it was, which is how a request that may say nothing says nothing; one
// that cannot be read is refused.
func decode(rw http.ResponseWriter, r *http.Request, v any) bool {
	// fields this vmhost does not know are let be: an orchestrator newer
	// than it may send them while the two are deployed one after the other.
	decoder := json.NewDecoder(http.MaxBytesReader(rw, r.Body, maxBody))

	if err := decoder.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		failed(rw, r, fmt.Errorf("%w: the body is not one: %w", vm.ErrInvalid, err))

		return false
	}

	return true
}
