package getVMLogs

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
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
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

	t.Run("lines after the last one the reader has are handed over, once the vm is found", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, func(uint64, guest.Process) vmMock.FakeRun {
			return vmMock.FakeRun{Lines: []string{"one", "two", "three"}, Exits: true}
		})

		id := host.Run(t, vmhosttest.Spec("job"))
		require.Eventually(t, func() bool { return host.VM(t, id).State == vm.StateExited }, 5*time.Second, 5*time.Millisecond)

		begun := false

		var lines []vm.LogLine

		response, err := NewUseCase(host.Engine, accepts()).Execute(ctx, &Request{ID: id, After: 1}, func() { begun = true }, func(line vm.LogLine) error {
			assert.True(t, begun, "begun before the first line")

			lines = append(lines, line)

			return nil
		})
		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)

		require.Len(t, lines, 2)
		assert.Equal(t, uint64(2), lines[0].Seq)
		assert.Equal(t, "two", lines[0].Content)
		assert.Equal(t, "three", lines[1].Content)
	})

	t.Run("a vm that is not there is not begun", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)

		_, err := NewUseCase(host.Engine, accepts()).Execute(ctx, &Request{ID: "0123456789abcdef"}, func() {
			t.Error("begun for a vm that is not there")
		}, func(vm.LogLine) error { return nil })

		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("what is not valid is said so, and nothing is begun", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)

		refuses := &validator.MockValidator{}
		refuses.On("Validate", mock.Anything).Return(domain.ValidationErrors{"id": "invalid_value"})

		response, err := NewUseCase(host.Engine, refuses).Execute(ctx, &Request{ID: "x"}, func() {
			t.Error("begun for a request that is not valid")
		}, func(vm.LogLine) error { return nil })
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"id": "invalid_value"}, response.ValidationErrors)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{ID: "0123456789abcdef", After: 7, Follow: true}).Validate())
	assert.Equal(t, domain.ValidationErrors{"id": "required_field"}, (&Request{}).Validate())
}
