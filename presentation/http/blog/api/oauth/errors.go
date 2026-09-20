// Package oauth serves this estate's authorization server: where an
// application registers, where somebody is asked whether it may act for them,
// and where it collects the session they gave it.
//
// The people reading these answers are applications, so what comes back is
// OAuth's own shape — an error code and a description — rather than the
// validation errors the rest of the API answers with.
package oauth

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/application/oauth"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// errorResponse is how OAuth says what was wrong.
type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// fail answers with what the protocol calls this failure, or with a 500 when
// it is not a failure the protocol has a name for — those are ours, and are
// recorded rather than described.
func fail(rw http.ResponseWriter, r *http.Request, err error) {
	oauthError, ok := oauth.AsError(err)
	if !ok {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	status := http.StatusBadRequest
	if oauthError.Code == oauth.ErrorInvalidClient {
		status = http.StatusUnauthorized
	}

	write(rw, status, errorResponse{
		Error:            oauthError.Code,
		ErrorDescription: oauthError.Description,
	})
}

func write(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")

	// nothing here may be held by a cache: a registration, a description of a
	// pending request and a session are each answered once, to one asker.
	rw.Header().Set("Cache-Control", "no-store")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(body)
}
