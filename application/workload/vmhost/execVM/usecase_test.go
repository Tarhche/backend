package execVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
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

	response, err := useCase.Execute(ctx, &Request{ID: id, Exec: guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}, TTY: true}})
	require.NoError(t, err)
	require.NotNil(t, response.Conn)
	defer response.Conn.Close()

	assert.NotEmpty(t, response.ExecID)

	require.NoError(t, guest.WriteFrame(response.Conn, guest.FrameStdin, []byte("ls\n")))

	frame, err := guest.ReadFrame(response.Conn)
	require.NoError(t, err)
	assert.Equal(t, guest.FrameStdout, frame.Type)
	assert.Equal(t, "ls\n", string(frame.Payload))

	require.NoError(t, host.Engine.Stop(ctx, id, 0))

	_, err = useCase.Execute(ctx, &Request{ID: id, Exec: guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}}})
	assert.ErrorIs(t, err, vm.ErrNotRunning)
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef", Exec: guest.Exec{Process: guest.Process{Args: []string{"sh"}, WorkingDir: "/tmp"}}}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field", "args": "required_field"}, (&Request{}).Validate())
	assert.Equal(t, domain.ValidationErrors{"working_dir": "invalid_value"}, (&Request{ID: "0123456789abcdef", Exec: guest.Exec{Process: guest.Process{Args: []string{"sh"}, WorkingDir: "tmp"}}}).Validate())
}
