package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRequestIDMiddleware(t *testing.T) {
	t.Run("creates middleware", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		m := NewRequestIDMiddleware(next)

		assert.NotNil(t, m)
		assert.NotNil(t, m.next)
	})
}

func TestRequestIDGeneratesUUID(t *testing.T) {
	t.Run("generates UUID v7 when no header present", func(t *testing.T) {
		var capturedID string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.NotEmpty(t, capturedID)
		assert.Len(t, capturedID, 36) // UUID v7 format with hyphens
		assert.Equal(t, capturedID, res.Header().Get(RequestIDHeaderKey))
	})

	t.Run("generates unique UUIDs for different requests", func(t *testing.T) {
		var id1, id2 string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id1 == "" {
				id1 = GetRequestID(r)
			} else {
				id2 = GetRequestID(r)
			}
		})
		m := NewRequestIDMiddleware(next)

		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		res1 := httptest.NewRecorder()
		m.ServeHTTP(res1, req1)

		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		res2 := httptest.NewRecorder()
		m.ServeHTTP(res2, req2)

		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
		assert.NotEqual(t, id1, id2)
	})
}

func TestRequestIDReusesExisting(t *testing.T) {
	t.Run("reuses X-Request-Id header when present", func(t *testing.T) {
		expectedID := "550e8400-e29b-41d4-a716-446655440000"
		var capturedID string

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeaderKey, expectedID)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, expectedID, capturedID)
		assert.Equal(t, expectedID, res.Header().Get(RequestIDHeaderKey))
	})

	t.Run("does not overwrite existing header in request", func(t *testing.T) {
		expectedID := "custom-request-id-123"
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, expectedID, r.Header.Get(RequestIDHeaderKey))
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeaderKey, expectedID)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, expectedID, res.Header().Get(RequestIDHeaderKey))
	})
}

func TestRequestIDResponseHeader(t *testing.T) {
	t.Run("echoes request ID in response header", func(t *testing.T) {
		expectedID := "test-id-456"
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeaderKey, expectedID)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, expectedID, res.Header().Get(RequestIDHeaderKey))
	})

	t.Run("includes generated UUID in response header", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		responseID := res.Header().Get(RequestIDHeaderKey)
		assert.NotEmpty(t, responseID)
		assert.Len(t, responseID, 36)
	})
}

func TestRequestIDContext(t *testing.T) {
	t.Run("stores request ID in context", func(t *testing.T) {
		expectedID := "context-test-id"
		var contextID string

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contextID = GetRequestID(r)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeaderKey, expectedID)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, expectedID, contextID)
	})

	t.Run("generated UUID is stored in context", func(t *testing.T) {
		var contextID string

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contextID = GetRequestID(r)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.NotEmpty(t, contextID)
		assert.Len(t, contextID, 36)
	})
}

func TestGetRequestID(t *testing.T) {
	t.Run("retrieves request ID from request", func(t *testing.T) {
		expectedID := "test-id-789"
		ctx := context.WithValue(context.Background(), requestIDCtxKey, expectedID)
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

		id := GetRequestID(req)

		assert.Equal(t, expectedID, id)
	})

	t.Run("returns empty string when no ID in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		id := GetRequestID(req)

		assert.Empty(t, id)
	})

	t.Run("returns empty string when ID is wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), requestIDCtxKey, 12345)
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

		id := GetRequestID(req)

		assert.Empty(t, id)
	})
}

func TestGetRequestIDFromContext(t *testing.T) {
	t.Run("retrieves request ID from context", func(t *testing.T) {
		expectedID := "direct-context-id"
		ctx := context.WithValue(context.Background(), requestIDCtxKey, expectedID)

		id := GetRequestIDFromContext(ctx)

		assert.Equal(t, expectedID, id)
	})

	t.Run("returns empty string when no context", func(t *testing.T) {
		ctx := context.Background()

		id := GetRequestIDFromContext(ctx)

		assert.Empty(t, id)
	})

	t.Run("returns empty string when wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), requestIDCtxKey, []byte("bytes"))

		id := GetRequestIDFromContext(ctx)

		assert.Empty(t, id)
	})

	t.Run("handles nil value", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), requestIDCtxKey, nil)

		id := GetRequestIDFromContext(ctx)

		assert.Empty(t, id)
	})
}

func TestRequestIDMiddlewareChaining(t *testing.T) {
	t.Run("passes request to next handler", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
		assert.Equal(t, http.StatusOK, res.Code)
	})

	t.Run("next handler can write response", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Custom-Header", "custom-value")
			w.WriteHeader(http.StatusCreated)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, http.StatusCreated, res.Code)
		assert.Equal(t, "custom-value", res.Header().Get("Custom-Header"))
		assert.NotEmpty(t, res.Header().Get(RequestIDHeaderKey))
	})
}

func TestRequestIDHTTPMethods(t *testing.T) {
	t.Run("works with different HTTP methods", func(t *testing.T) {
		methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
				m := NewRequestIDMiddleware(next)

				req := httptest.NewRequest(method, "/", nil)
				res := httptest.NewRecorder()

				m.ServeHTTP(res, req)

				assert.NotEmpty(t, res.Header().Get(RequestIDHeaderKey))
			})
		}
	})
}

func TestRequestIDEdgeCases(t *testing.T) {
	t.Run("handles empty string header value", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := GetRequestID(r)
			assert.NotEmpty(t, id)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeaderKey, "")
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.NotEmpty(t, res.Header().Get(RequestIDHeaderKey))
	})

	t.Run("preserves whitespace in provided ID", func(t *testing.T) {
		idWithSpaces := "id-with spaces-123"
		var capturedID string

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r)
		})
		m := NewRequestIDMiddleware(next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeaderKey, idWithSpaces)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, idWithSpaces, capturedID)
	})
}

func TestRequestIDImplementsHandler(t *testing.T) {
	t.Run("RequestID implements http.Handler", func(t *testing.T) {
		m := NewRequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		var _ http.Handler = m
	})
}
