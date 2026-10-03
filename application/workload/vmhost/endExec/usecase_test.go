package endExec

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
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

	execID, conn, err := host.Engine.Exec(ctx, id, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}, TTY: true})
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	useCase := NewUseCase(host.Engine, accepts())

	response, err := useCase.Execute(ctx, &Request{ID: id, Exec: execID, End: guest.EndExec{Grace: time.Second, KillGrace: time.Second}})
	require.NoError(t, err)
	assert.True(t, response.Ended.Signalled, "the command was there to end")

	response, err = useCase.Execute(ctx, &Request{ID: id, Exec: execID})
	require.NoError(t, err)
	assert.False(t, response.Ended.Signalled, "nothing was left to end")

	require.NoError(t, host.Engine.Stop(ctx, id, 0))

	response, err = useCase.Execute(ctx, &Request{ID: id, Exec: execID})
	require.NoError(t, err, "a vm that does not run took its commands with it")
	assert.False(t, response.Ended.Signalled)
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef", Exec: "3f2a9c"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field", "exec": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"exec": "invalid_value"}, (&Request{ID: "0123456789abcdef", Exec: "../poweroff"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"end": "invalid_value"}, (&Request{ID: "0123456789abcdef", Exec: "x", End: guest.EndExec{Grace: -1}}).Validate())
}
