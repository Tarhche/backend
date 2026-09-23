package getMachines

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	machineMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/machine"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("an orchestrator sees its own machines and nobody else's", func(t *testing.T) {
		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

		var vmm machineMock.MockVMM
		vmm.On("List", mock.Anything).Once().Return([]machine.Machine{
			{ID: "0000000000000001", Owner: "runner-orchestrator-01", Running: true},
			{ID: "0000000000000002", Owner: "runner-orchestrator-02", Running: true},
			{ID: "0000000000000003", Owner: "runner-orchestrator-01", Running: false},
		}, nil)

		response, err := NewUseCase(&vmm, v).Execute(context.Background(), &Request{Owner: "runner-orchestrator-01"})

		require.NoError(t, err)
		require.Len(t, response.Machines, 2)
		assert.Equal(t, "0000000000000001", response.Machines[0].ID)
		assert.Equal(t, "0000000000000003", response.Machines[1].ID)
	})
}

func TestRequest_Validate(t *testing.T) {
	assert.Empty(t, (&Request{Owner: "runner-orchestrator-01"}).Validate())
	assert.Equal(t, "required_field", (&Request{}).Validate()["owner"])
	assert.Equal(t, "invalid_value", (&Request{Owner: "../etc"}).Validate()["owner"])
}
