package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	"github.com/khanzadimahdi/testproject/infrastructure/health"
)

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	t.Run("every dependency answers", func(t *testing.T) {
		t.Parallel()

		var pinger health.MockPinger

		pinger.On("Ping", mock.Anything).Once().Return(nil)
		defer pinger.AssertExpectations(t)

		handler := NewHealthHandler(checkhealth.NewUseCase(
			checkhealth.Dependency{Name: "database", Pinger: &pinger},
		))

		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "text/plain; charset=utf-8", recorder.Header().Get("Content-Type"))
		assert.Equal(t, "ok", recorder.Body.String())
	})

	t.Run("a dependency doesn't answer", func(t *testing.T) {
		t.Parallel()

		var pinger health.MockPinger

		pinger.On("Ping", mock.Anything).Once().Return(errors.New("connection refused"))
		defer pinger.AssertExpectations(t)

		handler := NewHealthHandler(checkhealth.NewUseCase(
			checkhealth.Dependency{Name: "database", Pinger: &pinger},
		))

		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Equal(t, "database: connection refused", recorder.Body.String())
	})
}
