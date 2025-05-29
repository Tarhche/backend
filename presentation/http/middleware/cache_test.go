package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCacheMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("should not cache response if the request is not eligible", func(t *testing.T) {
		t.Parallel()

		mockCache := new(cache.MockCache)
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test response"))
		})

		middleware := NewCacheMiddleware(nextHandler, mockCache)
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		resp := httptest.NewRecorder()

		middleware.ServeHTTP(resp, req)

		mockCache.AssertNotCalled(t, "Set")
		assert.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("should cache response if it is not already cached", func(t *testing.T) {
		t.Parallel()

		mockCache := new(cache.MockCache)
		mockCache.On("Get", mock.Anything).Return([]byte{}, domain.ErrNotExists)
		mockCache.On("Set", mock.Anything, mock.Anything).Return(nil)
		defer mockCache.AssertExpectations(t)

		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test response"))
		})

		middleware := NewCacheMiddleware(nextHandler, mockCache)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp := httptest.NewRecorder()

		middleware.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

	})

	t.Run("should use cache if it is already cached", func(t *testing.T) {
		t.Parallel()

		mockCache := new(cache.MockCache)
		cachedResponse := response{
			Headers: http.Header{"Content-Type": []string{"application/json"}},
			Body:    []byte("cached response"),
			Status:  http.StatusOK,
		}

		responseBytes, _ := json.Marshal(cachedResponse)
		mockCache.On("Get", mock.Anything).Return(responseBytes, nil)

		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called when cache exists")
		})

		middleware := NewCacheMiddleware(nextHandler, mockCache)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp := httptest.NewRecorder()

		middleware.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "cached response", resp.Body.String())
		mockCache.AssertExpectations(t)
	})

	t.Run("should call the next handler if the cache is invalid", func(t *testing.T) {
		t.Parallel()

		mockCache := new(cache.MockCache)
		mockCache.On("Get", mock.Anything).Return([]byte("invalid json"), nil)

		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("fresh response"))
		})

		middleware := NewCacheMiddleware(nextHandler, mockCache)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp := httptest.NewRecorder()

		middleware.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "fresh response", resp.Body.String())
		mockCache.AssertExpectations(t)
	})
}
