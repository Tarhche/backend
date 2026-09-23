package checkOrchestratorExists

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	infraIngress "github.com/khanzadimahdi/testproject/infrastructure/runner/ingress"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("a connected orchestrator is there", func(t *testing.T) {
		var registry infraIngress.MockRegistry
		registry.On("Exists", mock.Anything, "runner-orchestrator-01").Once().Return(true, nil)
		defer registry.AssertExpectations(t)

		exists, err := NewUseCase(&registry).Execute(t.Context(), &Request{Name: "runner-orchestrator-01"})

		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("an orchestrator that is not connected is not there", func(t *testing.T) {
		var registry infraIngress.MockRegistry
		registry.On("Exists", mock.Anything, "runner-orchestrator-01").Once().Return(false, nil)
		defer registry.AssertExpectations(t)

		exists, err := NewUseCase(&registry).Execute(t.Context(), &Request{Name: "runner-orchestrator-01"})

		assert.NoError(t, err, "an orchestrator that is not there is not a failure")
		assert.False(t, exists)
	})

	t.Run("a registry that cannot answer is not an orchestrator that is missing", func(t *testing.T) {
		expected := errors.New("the registry is unreachable")

		var registry infraIngress.MockRegistry
		registry.On("Exists", mock.Anything, "runner-orchestrator-01").Once().Return(false, expected)
		defer registry.AssertExpectations(t)

		_, err := NewUseCase(&registry).Execute(t.Context(), &Request{Name: "runner-orchestrator-01"})

		assert.ErrorIs(t, err, expected)
	})
}
