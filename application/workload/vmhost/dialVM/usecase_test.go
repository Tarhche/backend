package dialVM

import (
	"bufio"
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

	response, err := useCase.Execute(ctx, &Request{ID: id, Port: 80})
	require.NoError(t, err)
	require.NotNil(t, response.Conn)
	defer response.Conn.Close()

	_, err = response.Conn.Write([]byte("ping\n"))
	require.NoError(t, err)

	echoed, err := bufio.NewReader(response.Conn).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "ping\n", echoed)

	_, err = useCase.Execute(ctx, &Request{ID: id, Port: 22})
	assert.ErrorIs(t, err, vm.ErrInvalid, "a port the task is not reached on")
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef", Port: 80}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field", "port": "required_field"}, (&Request{}).Validate())
}
