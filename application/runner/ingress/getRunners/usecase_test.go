package getRunners

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("every connected runner comes back", func(t *testing.T) {
		at := time.Now()

		var registry infraIngress.MockRegistry
		registry.On("All", mock.Anything).Once().Return([]ingress.Runner{
			{ID: "runner-worker-01", Connections: 2, ConnectedAt: at},
			{ID: "runner-worker-02", Connections: 1, ConnectedAt: at},
		}, nil)
		defer registry.AssertExpectations(t)

		response, err := NewUseCase(&registry).Execute(t.Context())

		assert.NoError(t, err)
		assert.Equal(t, &Response{
			Items: []RunnerResponse{
				{ID: "runner-worker-01", Connections: 2, ConnectedAt: at},
				{ID: "runner-worker-02", Connections: 1, ConnectedAt: at},
			},
		}, response)
	})

	t.Run("no runners at all is an empty answer, not an error", func(t *testing.T) {
		var registry infraIngress.MockRegistry
		registry.On("All", mock.Anything).Once().Return([]ingress.Runner{}, nil)
		defer registry.AssertExpectations(t)

		response, err := NewUseCase(&registry).Execute(t.Context())

		assert.NoError(t, err)
		assert.Equal(t, &Response{Items: []RunnerResponse{}}, response)
	})

	t.Run("a registry that fails fails the use case", func(t *testing.T) {
		expected := errors.New("registry failure")

		var registry infraIngress.MockRegistry
		registry.On("All", mock.Anything).Once().Return(nil, expected)
		defer registry.AssertExpectations(t)

		response, err := NewUseCase(&registry).Execute(t.Context())

		assert.ErrorIs(t, err, expected)
		assert.Nil(t, response)
	})
}
