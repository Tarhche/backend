package getVM

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

	id := host.Run(t, vmhosttest.Spec("web"))
	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{ID: id})
	require.NoError(t, err)

	assert.Equal(t, id, response.VM.ID)
	assert.Equal(t, vm.StateRunning, response.VM.State)
	assert.Equal(t, map[string]string{"node.name": "orchestrator-01", "task.slug": "web"}, response.VM.Spec.Labels)
	assert.Equal(t, []uint16{80}, response.VM.Endpoints())

	_, err = useCase.Execute(ctx, &Request{ID: "0123456789abcdef"})
	assert.ErrorIs(t, err, vm.ErrNotFound)
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "invalid_value"}, (&Request{ID: "../../etc/passwd"}).Validate())
}
