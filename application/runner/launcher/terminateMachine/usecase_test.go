package terminateMachine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	machineMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/machine"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func TestUseCase_Execute(t *testing.T) {
	t.Run("a machine's process ends before it is unplugged", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		killed := vmm.On("Kill", mock.Anything, "0123456789abcdef").Once().Return(nil)
		network.On("Unplug", mock.Anything, "0123456789abcdef").Once().Return(nil).NotBefore(killed)
		defer vmm.AssertExpectations(t)
		defer network.AssertExpectations(t)

		response, err := NewUseCase(&vmm, &network, accepts()).Execute(context.Background(), &Request{ID: "0123456789abcdef"})

		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)
	})

	t.Run("a process that cannot be ended leaves the machine plugged in", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		failed := errors.New("no such process")
		vmm.On("Kill", mock.Anything, mock.Anything).Once().Return(failed)

		_, err := NewUseCase(&vmm, &network, accepts()).Execute(context.Background(), &Request{ID: "0123456789abcdef"})

		assert.ErrorIs(t, err, failed)
		network.AssertNotCalled(t, "Unplug", mock.Anything, mock.Anything)
	})
}

func TestRequest_Validate(t *testing.T) {
	assert.Empty(t, (&Request{ID: "0123456789abcdef"}).Validate())
	assert.Equal(t, "required_field", (&Request{}).Validate()["id"])
	assert.Equal(t, "invalid_value", (&Request{ID: "../0123456789"}).Validate()["id"])
}
