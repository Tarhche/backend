package stopVM

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

	spec := vmhosttest.Spec("web")
	spec.RestartPolicy = "always"

	id := host.Run(t, spec)
	agent := host.Agent(t, id)
	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{ID: id, Timeout: vm.DefaultStopTimeout})
	require.NoError(t, err)
	assert.Empty(t, response.ValidationErrors)

	stopped := host.VM(t, id)
	assert.Equal(t, vm.StateExited, stopped.State)
	assert.Equal(t, 143, stopped.ExitCode, "the task ended on the term it was sent")
	assert.True(t, stopped.Stopped, "no policy starts it again")
	assert.True(t, agent.PoweredOff(), "its machine turned itself off")

	_, err = useCase.Execute(ctx, &Request{ID: id, Timeout: time.Second})
	assert.NoError(t, err, "stopping a vm that does not run is stopping nothing")
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef"}).Validate())
	assert.Empty(t, (&Request{ID: "0123456789abcdef", Timeout: 0}).Validate(), "no time at all is a stop that kills at once")
	assert.Equal(t, domain.ValidationErrors{"timeout": "invalid_value"}, (&Request{ID: "0123456789abcdef", Timeout: -time.Second}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field"}, (&Request{}).Validate())
}
