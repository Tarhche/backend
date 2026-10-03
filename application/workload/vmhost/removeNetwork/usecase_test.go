package removeNetwork

import (
	"context"
	"testing"
	"time"

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

	_, err := host.Engine.EnsureNetwork(ctx, "workload-stack-shop", false)
	require.NoError(t, err)

	spec := vmhosttest.Spec("shop-api")
	spec.Networks = []vm.Attachment{{Network: "workload-stack-shop", Aliases: []string{"api"}}}
	id := host.Run(t, spec)

	_, err = useCase.Execute(ctx, &Request{Name: "workload-stack-shop"})
	assert.ErrorIs(t, err, vm.ErrNetworkInUse)

	_, err = useCase.Execute(ctx, &Request{Name: vm.PublicNetwork})
	assert.ErrorIs(t, err, vm.ErrConflict)

	require.NoError(t, host.Engine.Stop(ctx, id, time.Second))

	response, err := useCase.Execute(ctx, &Request{Name: "workload-stack-shop"})
	require.NoError(t, err)
	assert.Empty(t, response.ValidationErrors)

	_, err = useCase.Execute(ctx, &Request{Name: "workload-stack-shop"})
	assert.NoError(t, err, "a network that is not there is the outcome asked for")
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{Name: "workload-stack-shop"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"name": "invalid_value"}, (&Request{Name: "Shop"}).Validate())
}
