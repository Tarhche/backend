package createStack

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/dockerVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmEvents "github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

const compose = `
name: ignored
services:
  web:
    image: nginx:1.27
    ports: ["80:80"]
`

func useCaseOf(w *vmtest.Workload) *UseCase {
	tasks := &tasksMock.MockTasksRepository{}
	tasks.On("GetOneBySlug", mock.Anything, mock.Anything).Return(task.Task{}, domain.ErrNotExists)

	create := createVM.NewUseCase(w.VMs, tasks, w.Snapshots, w.Quota, w.Lifecycle, validator.New(translator.Codes{}), createVM.Images{Machine: "ubuntu:24.04", Docker: "docker:29-dind"})
	chooser := dockerVM.NewChooser(w.VMs, create, w.Lifecycle, dockerVM.Defaults{
		Resources: vm.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB},
		Ports:     []port.Port{80},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
	})

	return NewUseCase(w.Stacks, chooser, dispatch.New(w.Stacks, w.Producer, slog.New(slog.DiscardHandler)), validator.New(translator.Codes{}))
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a stack in a running docker vm is deployed at once", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

		response, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "owner", Name: "Web Site", Compose: compose})
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		assert.Equal(t, "01", response.VM.UUID)
		assert.False(t, response.VM.Created)

		stored, ok := w.Stacks.Stored(response.Stack.UUID)
		require.True(t, ok)
		assert.True(t, strings.HasPrefix(stored.Slug, "web-site-"), stored.Slug)
		assert.Equal(t, stack.Deploying, stored.State)
		assert.Equal(t, stack.Running, stored.ExpectedState)
		assert.Empty(t, stored.Reason)
		assert.Equal(t, compose, stored.Compose, "kept as it was written")

		var asked events.StackRequested
		require.True(t, w.Producer.Last(events.StackRequestedName, &asked))
		assert.Equal(t, events.StackRequested{
			StackUUID: stored.UUID,
			VMUUID:    "01",
			NodeName:  vmtest.Node,
			Action:    stack.ActionUp,
			Project:   stored.Slug,
			Compose:   compose,
		}, asked)
	})

	t.Run("one in a docker vm made for it waits for the vm to come up", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		response, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "owner", Name: "web", Compose: compose})
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)
		assert.True(t, response.VM.Created)

		stored, _ := w.Stacks.Stored(response.Stack.UUID)
		assert.Equal(t, stack.Deploying, stored.State)
		assert.Equal(t, dispatch.ReasonWaitingForVM, stored.Reason)
		assert.Equal(t, []string{vmEvents.VMScheduledName}, w.Producer.Subjects(), "the vm was asked for, the stack not yet")
	})

	t.Run("one in a docker vm that is not coming up is refused", func(t *testing.T) {
		t.Parallel()

		stopped := vmtest.Docker("01", "owner")
		stopped.CurrentState = vm.Stopped

		w := vmtest.New(vmtest.WithVMs(stopped))

		response, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "owner", Name: "web", Compose: compose, VM: dockerVM.Choice{UUID: "01"}})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "vm_not_running"}, response.ValidationErrors)
		assert.Zero(t, w.Stacks.Len())
	})

	for name, tt := range map[string]struct {
		compose string
		want    string
	}{
		"no compose at all":      {compose: "  ", want: "required_field"},
		"yaml that is not":       {compose: "services: [unclosed", want: "invalid_value"},
		"a project with nothing": {compose: "volumes:\n  data: {}\n", want: "invalid_value"},
		"more than a stack keeps": {
			compose: "services:\n  web:\n    image: nginx\n#" + strings.Repeat("x", MaxCompose),
			want:    "too_large",
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

			response, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "owner", Name: "web", Compose: tt.compose})
			require.NoError(t, err)
			assert.Equal(t, domain.ValidationErrors{"compose": tt.want}, response.ValidationErrors)
		})
	}
}
