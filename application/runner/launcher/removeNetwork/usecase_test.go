package removeNetwork

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	machineMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/machine"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("a network still in use says so", func(t *testing.T) {
		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

		var network machineMock.MockHostNetwork
		network.On("RemoveNetwork", mock.Anything, "runner-orchestrator-01", "runner-stack-shop").Once().Return(machine.ErrNetworkInUse)

		_, err := NewUseCase(&network, v).Execute(context.Background(), &Request{Owner: "runner-orchestrator-01", Name: "runner-stack-shop"})

		assert.ErrorIs(t, err, machine.ErrNetworkInUse)
	})
}

func TestRequest_Validate(t *testing.T) {
	assert.Empty(t, (&Request{Owner: "runner-orchestrator-01", Name: "runner-stack-shop"}).Validate())
	assert.Equal(t, "invalid_value", (&Request{Owner: "runner-orchestrator-01", Name: "eth0"}).Validate()["name"])
}
