package getRunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("a connected runner comes back", func(t *testing.T) {
		var registry infraIngress.MockRegistry
		registry.On("Get", mock.Anything, "runner-worker-01").Once().Return(ingress.Runner{ID: "runner-worker-01"}, nil)
		defer registry.AssertExpectations(t)

		response, err := NewUseCase(&registry).Execute(t.Context(), &Request{ID: "runner-worker-01"})

		assert.NoError(t, err)
		assert.Equal(t, &Response{ID: "runner-worker-01"}, response)
	})

	t.Run("a runner that is not connected does not exist", func(t *testing.T) {
		var registry infraIngress.MockRegistry
		registry.On("Get", mock.Anything, "runner-worker-01").Once().Return(ingress.Runner{}, domain.ErrNotExists)
		defer registry.AssertExpectations(t)

		response, err := NewUseCase(&registry).Execute(t.Context(), &Request{ID: "runner-worker-01"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, response)
	})
}
