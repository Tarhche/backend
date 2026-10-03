package killVM

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

	spec := vmhosttest.Spec("web")
	spec.RestartPolicy = "unless-stopped"

	id := host.Run(t, spec)
	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{ID: id})
	require.NoError(t, err)
	assert.Empty(t, response.ValidationErrors)

	killed := host.VM(t, id)
	assert.Equal(t, vm.StateExited, killed.State)
	assert.Equal(t, 137, killed.ExitCode)
	assert.True(t, killed.Stopped)

	_, err = useCase.Execute(ctx, &Request{ID: id})
	assert.ErrorIs(t, err, vm.ErrNotRunning, "a vm that does not run cannot be killed")
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field"}, (&Request{}).Validate())
}
