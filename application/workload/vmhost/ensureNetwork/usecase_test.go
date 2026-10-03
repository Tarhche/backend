package ensureNetwork

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
	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{Name: "workload-stack-shop"})
	require.NoError(t, err)
	assert.Equal(t, "workload-stack-shop", response.Network.Name)
	assert.False(t, response.Network.Masquerade)
	assert.NotEmpty(t, response.Network.Subnet)

	again, err := useCase.Execute(ctx, &Request{Name: "workload-stack-shop"})
	require.NoError(t, err)
	assert.Equal(t, response.Network, again.Network, "asking twice is asking once")

	_, err = useCase.Execute(ctx, &Request{Name: "workload-stack-shop", Masquerade: true})
	assert.ErrorIs(t, err, vm.ErrConflict)
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{Name: "workload-isolated"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"name": "invalid_value"}, (&Request{Name: "docker0/../x"}).Validate())
}
