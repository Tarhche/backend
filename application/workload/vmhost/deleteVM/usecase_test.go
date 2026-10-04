package deleteVM

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
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

	id := host.Run(t, vmhosttest.Spec("web"))
	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{ID: id})
	require.NoError(t, err)
	assert.Empty(t, response.ValidationErrors)

	_, err = host.Engine.VM(ctx, id)
	assert.ErrorIs(t, err, vm.ErrNotFound)

	_, err = host.Hypervisor.Machine(ctx, id)
	assert.ErrorIs(t, err, vm.ErrNotFound, "its machine was ended")
	assert.Empty(t, host.Fabric.Plugged(id), "its taps were taken away")

	_, err = os.Stat(layout.VM(host.DataDir, id))
	assert.ErrorIs(t, err, os.ErrNotExist, "its record, disk and output are gone")

	_, err = useCase.Execute(ctx, &Request{ID: id})
	assert.ErrorIs(t, err, vm.ErrNotFound)
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "invalid_value"}, (&Request{ID: "../0123456789abc"}).Validate())
}
