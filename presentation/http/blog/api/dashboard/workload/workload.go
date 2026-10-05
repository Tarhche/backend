// Package workload is what the dashboard's workload routes share: whose things
// a route acts on, and how what the workload answered is said.
//
// The workload's routes come in two sets. Those under /api/dashboard/workload
// act on anybody's VMs, containers and stacks, under the workload's own
// permissions; those under /api/dashboard/my/workload act on the caller's own,
// under the self ones. One handler serves a route in both, told which set it
// is in by the Owner it is given, so the two cannot answer differently.
package workload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Owner is whose things a route acts on, as the workload is asked: one
// person's uuid narrows what it finds to theirs, and empty is anybody's.
type Owner func(r *http.Request) string

// Anybody is the owner on the workload's routes: whoever's it is.
func Anybody(*http.Request) string {
	return ""
}

// Caller is the owner on the my routes: whoever is asking, so that what is
// somebody else's is not there as far as they are concerned.
func Caller(r *http.Request) string {
	return auth.UUIDFromContext(r.Context())
}

// Refusal is what a request the workload or the dashboard would not take is
// answered with: what was wrong, field by field, in the reader's language.
type Refusal struct {
	Errors domain.ValidationErrors `json:"errors"`
}

// Failed answers a request that could not be answered, and reports whether
// there was one: nothing is written for a nil error.
//
// Something that is not there is not found. A request the workload took too
// long over is a gateway timeout rather than a failure, since what was asked
// for may still be under way: a pull carries on after whoever asked has
// stopped waiting. Anything else failed.
func Failed(rw http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case errors.Is(err, context.DeadlineExceeded):
		rw.WriteHeader(http.StatusGatewayTimeout)
	default:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	}

	return true
}

// Refused answers a request that was refused, and reports whether it was.
func Refused(rw http.ResponseWriter, refused domain.ValidationErrors) bool {
	if len(refused) == 0 {
		return false
	}

	JSON(rw, http.StatusBadRequest, Refusal{Errors: refused})

	return true
}

// JSON answers with body, as json.
func JSON(rw http.ResponseWriter, status int, body any) {
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}

// Decode reads the json a request carries into into, and answers the request
// itself when it cannot: a body that is not json is not a request at all.
func Decode(rw http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		JSON(rw, http.StatusBadRequest, Refusal{Errors: domain.ValidationErrors{"body": "invalid_value"}})

		return false
	}

	return true
}

// Page is the page of a listing asked for, the first when none is.
func Page(r *http.Request) uint {
	if page, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, 32); err == nil && page > 0 {
		return uint(page)
	}

	return 1
}

// Flag is a query parameter that says yes or no, and is no unless it says
// yes.
func Flag(r *http.Request, name string) bool {
	yes, err := strconv.ParseBool(r.URL.Query().Get(name))

	return err == nil && yes
}

// Since is the moment a log is read from, as an RFC 3339 timestamp, and the
// start of it when none is given.
func Since(r *http.Request) time.Time {
	since, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("since"))
	if err != nil {
		return time.Time{}
	}

	return since
}

// Tail is how many of the last lines of a log are asked for, and all of them
// when no number is.
func Tail(r *http.Request) uint {
	tail, err := strconv.ParseUint(r.URL.Query().Get("tail"), 10, 32)
	if err != nil {
		return 0
	}

	return uint(tail)
}
