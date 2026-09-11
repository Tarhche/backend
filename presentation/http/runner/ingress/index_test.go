package ingress

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	getRunners "github.com/khanzadimahdi/testproject/application/runner/ingress/getRunners"
	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
)

func TestIndexHandler(t *testing.T) {
	t.Run("the connected runners come back", func(t *testing.T) {
		at := time.Now().UTC().Truncate(time.Second)

		var registry infraIngress.MockRegistry
		registry.On("All", mock.Anything).Once().Return([]ingress.Runner{
			{ID: "runner-worker-01", Connections: 2, ConnectedAt: at},
		}, nil)
		defer registry.AssertExpectations(t)

		request := httptest.NewRequest(http.MethodGet, "/api/runners", nil)
		recorder := httptest.NewRecorder()

		NewIndexHandler(getRunners.NewUseCase(&registry)).ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

		var response getRunners.Response
		assert.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
		assert.Equal(t, getRunners.Response{
			Items: []getRunners.RunnerResponse{
				{ID: "runner-worker-01", Connections: 2, ConnectedAt: at},
			},
		}, response)
	})

	t.Run("a registry that fails is a server error", func(t *testing.T) {
		var registry infraIngress.MockRegistry
		registry.On("All", mock.Anything).Once().Return(nil, errors.New("registry failure"))
		defer registry.AssertExpectations(t)

		request := httptest.NewRequest(http.MethodGet, "/api/runners", nil)
		recorder := httptest.NewRecorder()

		NewIndexHandler(getRunners.NewUseCase(&registry)).ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	})
}
