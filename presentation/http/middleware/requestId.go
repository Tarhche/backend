package middleware

import (
	"context"
	"net/http"

	"github.com/gofrs/uuid/v5"
)

// RequestIDHeaderKey is the HTTP header that carries the request identifier.
// It is formatted with http.CanonicalHeaderKey.
const RequestIDHeaderKey = "X-Request-Id"

type requestIDCtxKeyType struct{}

var requestIDCtxKey = requestIDCtxKeyType{}

// RequestID ensures every request carries a unique identifier. It reuses an
// incoming X-Request-Id header when present, otherwise it generates a UUID v7.
// The identifier is stored in the request context and echoed back in the
// response header so clients can correlate requests with logs.
type RequestID struct {
	next http.Handler
}

// Ensure RequestID implements the http.Handler interface.
var _ http.Handler = &RequestID{}

func NewRequestIDMiddleware(next http.Handler) *RequestID {
	return &RequestID{
		next: next,
	}
}

func (m *RequestID) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get(RequestIDHeaderKey)
	if requestID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		requestID = id.String()
		r.Header.Set(RequestIDHeaderKey, requestID)
	}

	rw.Header().Set(RequestIDHeaderKey, requestID)

	ctx := context.WithValue(r.Context(), requestIDCtxKey, requestID)

	m.next.ServeHTTP(rw, r.WithContext(ctx))
}

// GetRequestID returns the request identifier stored in the request.
func GetRequestID(r *http.Request) string {
	return GetRequestIDFromContext(r.Context())
}

// GetRequestIDFromContext returns the request identifier stored in the context.
func GetRequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDCtxKey).(string); ok {
		return id
	}

	return ""
}
