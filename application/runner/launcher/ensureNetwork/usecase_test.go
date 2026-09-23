package ensureNetwork

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
	t.Run("the network is made, or found, and said what it is", func(t *testing.T) {
		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

		ensured := machine.Network{Owner: "runner-orchestrator-01", Name: "runner-public", Subnet: "10.200.1.0/24", Gateway: "10.200.1.1", Masquerade: true}

		var network machineMock.MockHostNetwork
		network.On("EnsureNetwork", mock.Anything, "runner-orchestrator-01", "runner-public", true).Once().Return(ensured, nil)
		defer network.AssertExpectations(t)

		response, err := NewUseCase(&network, v).Execute(context.Background(), &Request{Owner: "runner-orchestrator-01", Name: "runner-public", Masquerade: true})

		require.NoError(t, err)
		assert.Equal(t, ensured, response.Network)
	})
}

func TestRequest_Validate(t *testing.T) {
	assert.Empty(t, (&Request{Owner: "runner-orchestrator-01", Name: "runner-stack-shop"}).Validate())

	validationErrors := (&Request{Owner: "runner-orchestrator-01", Name: "docker0"}).Validate()
	assert.Equal(t, "invalid_value", validationErrors["name"], "a network that is not the runner's own is out of reach")

	assert.Equal(t, domain.ValidationErrors{"owner": "required_field", "name": "required_field"}, (&Request{}).Validate())
}
