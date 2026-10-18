// Package respond writes the control plane's answers the one way its API
// answers: JSON, a refusal as 400 with the codes it was refused for, what is
// not there as 404, and what a node refused with the status its code stands
// for, so the blog's client can read every one of them back as the error it
// was.
package respond

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"unsafe"

	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// JSON writes body with status.
func JSON(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}

// Refused writes what a request was refused for, under the fields it names.
func Refused(rw http.ResponseWriter, validationErrors domain.ValidationErrors) {
	JSON(rw, http.StatusBadRequest, map[string]domain.ValidationErrors{"errors": validationErrors})
}

// NodeRefused writes why a node gave no answer, with the status its code
// stands for: not there is 404, a VM that cannot be asked is 409, what dockerd
// refused is 422, a node that took too long is 504, and anything else 502.
func NodeRefused(rw http.ResponseWriter, refused *noderequest.Error) {
	JSON(rw, StatusOf(refused.Code), map[string]*noderequest.Error{"error": refused})
}

// StatusOf is the status a node's code is answered with.
func StatusOf(code noderequest.Code) int {
	switch code {
	case noderequest.CodeNotFound:
		return http.StatusNotFound
	case noderequest.CodeNotRunning, noderequest.CodeNotDocker, noderequest.CodeDockerUnavailable:
		return http.StatusConflict
	case noderequest.CodeInvalid:
		return http.StatusUnprocessableEntity
	case noderequest.CodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusBadGateway
	}
}

// Failed writes an error a use case returned: what is not there is 404, and
// anything else is recorded on the request's span and answered with 500.
func Failed(rw http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrNotExists) {
		rw.WriteHeader(http.StatusNotFound)

		return
	}

	infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
	rw.WriteHeader(http.StatusInternalServerError)
}

// Decode reads a request's JSON body into out, and writes the refusal when
// it is not JSON that fits.
func Decode(rw http.ResponseWriter, r *http.Request, out any) bool {
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		Refused(rw, domain.ValidationErrors{"body": "invalid_value"})

		return false
	}

	return true
}

// Owner is the person a request narrows what it acts on to, or nobody.
func Owner(r *http.Request) string {
	return r.URL.Query().Get("owner")
}

// Page is the page a listing asks for; anything that is not a page is the
// first.
func Page(r *http.Request) uint {
	var page uint = 1

	if r.URL.Query().Has("page") {
		if parsed, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page))*8); err == nil {
			page = uint(parsed)
		}
	}

	return page
}
