package startVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	host := vmhosttest.New(t, nil)

	id := host.Create(t, vmhosttest.Spec("web"))
	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{ID: id})
	require.NoError(t, err)
	assert.Empty(t, response.ValidationErrors)

	running := host.VM(t, id)
	assert.Equal(t, vm.StateRunning, running.State)
	assert.NotEmpty(t, running.Interfaces)
	assert.Len(t, host.Hypervisor.Booted(), 1)

	_, err = useCase.Execute(ctx, &Request{ID: id})
	require.NoError(t, err, "starting a vm that runs is starting nothing")
	assert.Len(t, host.Hypervisor.Booted(), 1)

	_, err = useCase.Execute(ctx, &Request{ID: "0123456789abcdef"})
	assert.ErrorIs(t, err, vm.ErrNotFound)
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "invalid_value"}, (&Request{ID: "web"}).Validate())
}
