package getVMs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain"
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

	mine := host.Create(t, vmhosttest.Spec("mine"))

	theirs := vmhosttest.Spec("theirs")
	theirs.Labels = map[string]string{"node.name": "orchestrator-02", "task.slug": "theirs"}
	host.Create(t, theirs)

	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{Labels: []string{"node.name=orchestrator-01"}})
	require.NoError(t, err)
	require.Len(t, response.VMs, 1)
	assert.Equal(t, mine, response.VMs[0].ID)
	assert.Equal(t, "mine", response.VMs[0].Spec.Labels["task.slug"], "a vm is answered with its labels")

	response, err = useCase.Execute(ctx, &Request{Labels: []string{"node.name=orchestrator-01", "task.slug=theirs"}})
	require.NoError(t, err)
	assert.NotNil(t, response.VMs, "none is an empty list")
	assert.Empty(t, response.VMs)

	response, err = useCase.Execute(ctx, &Request{})
	require.NoError(t, err)
	assert.Len(t, response.VMs, 2, "no filter is every vm")
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{Labels: []string{"node.name=orchestrator-01", "task.stack="}}).Validate())
	assert.Equal(t, domain.ValidationErrors{"label": "invalid_value"}, (&Request{Labels: []string{"node.name"}}).Validate())
	assert.Equal(t, domain.ValidationErrors{"label": "invalid_value"}, (&Request{Labels: []string{"=x"}}).Validate())

	many := make([]string, maxLabels+1)
	for i := range many {
		many[i] = "k=v"
	}

	assert.Equal(t, domain.ValidationErrors{"label": "exceeds_limit"}, (&Request{Labels: many}).Validate())
}
