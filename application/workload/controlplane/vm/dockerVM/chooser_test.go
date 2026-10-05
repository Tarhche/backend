package dockerVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

var defaults = Defaults{
	Resources:      vm.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB},
	Ports:          []port.Port{80, 443, 8080},
	Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
	PersistentDisk: true,
}

func chooserOf(w *vmtest.Workload) *Chooser {
	tasks := &tasksMock.MockTasksRepository{}
	tasks.On("GetOneBySlug", mock.Anything, mock.Anything).Return(task.Task{}, domain.ErrNotExists)

	create := createVM.NewUseCase(
		w.VMs,
		tasks,
		w.Snapshots,
		w.Quota,
		w.Lifecycle,
		validator.New(translator.Codes{}),
		createVM.Images{Machine: "ubuntu:24.04", Docker: "docker:29-dind"},
	)

	return NewChooser(w.VMs, create, w.Lifecycle, defaults)
}

func TestChooser_Choose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	machine := vmtest.Running("m1", "owner")
	docker := vmtest.Docker("d1", "owner")
	another := vmtest.Docker("d2", "owner")
	theirs := vmtest.Docker("t1", "other")
	leaving := func() vm.VM { v := vmtest.Docker("d3", "owner"); v.CurrentState = vm.Deleting; return v }()

	for name, tt := range map[string]struct {
		vms     []vm.VM
		choice  Choice
		want    string
		refused domain.ValidationErrors
	}{
		"a docker vm the request names": {
			vms:    []vm.VM{docker, another},
			choice: Choice{UUID: "d2"},
			want:   "d2",
		},
		"not one that is somebody else's": {
			vms:     []vm.VM{theirs},
			choice:  Choice{UUID: "t1"},
			refused: domain.ValidationErrors{"vm.uuid": "not_found"},
		},
		"not one that is not a docker vm": {
			vms:     []vm.VM{machine},
			choice:  Choice{UUID: "m1"},
			refused: domain.ValidationErrors{"vm.uuid": "not_docker"},
		},
		"not one on its way out": {
			vms:     []vm.VM{leaving},
			choice:  Choice{UUID: "d3"},
			refused: domain.ValidationErrors{"vm.uuid": "not_found"},
		},
		"the only docker vm there is, when none is named": {
			vms:  []vm.VM{machine, docker, theirs, leaving},
			want: "d1",
		},
		"none, when there are several to choose from": {
			vms:     []vm.VM{docker, another},
			refused: domain.ValidationErrors{"vm": "vm_required"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(tt.vms...))

			chosen, refused, err := chooserOf(w).Choose(ctx, "owner", tt.choice)
			require.NoError(t, err)

			assert.Equal(t, tt.refused, refused)
			assert.Equal(t, tt.want, chosen.VM.UUID)
			assert.False(t, chosen.Created)
			assert.Empty(t, w.Producer.Messages(), "nothing was made")
		})
	}

	t.Run("one is made with the defaults for somebody who has none", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(machine))

		chosen, refused, err := chooserOf(w).Choose(ctx, "owner", Choice{})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.True(t, chosen.Created)
		assert.Equal(t, vm.KindDocker, chosen.VM.Kind)
		assert.Equal(t, "docker", chosen.VM.Name)
		assert.Equal(t, "docker:29-dind", chosen.VM.Image)
		assert.Equal(t, defaults.Resources, chosen.VM.Resources)
		assert.Equal(t, defaults.Ports, chosen.VM.Ports)
		assert.True(t, chosen.VM.PersistentDisk)
		assert.Equal(t, vm.Scheduled, chosen.VM.CurrentState)

		var scheduled events.VMScheduled
		require.True(t, w.Producer.Last(events.VMScheduledName, &scheduled))
		assert.Equal(t, chosen.VM.UUID, scheduled.VMUUID)
	})

	t.Run("one is made as asked, the defaults filling in the rest", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(docker))

		chosen, refused, err := chooserOf(w).Choose(ctx, "owner", Choice{New: &New{
			Name:    "builds",
			Ports:   []port.Port{3000},
			Network: &createVM.Network{Egress: vm.AccessDeny},
		}})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.True(t, chosen.Created)
		assert.Equal(t, "builds", chosen.VM.Name)
		assert.Equal(t, []port.Port{3000}, chosen.VM.Ports)
		assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, chosen.VM.Network)
		assert.Equal(t, defaults.Resources, chosen.VM.Resources)
	})

	t.Run("a size left at zero is the default size, each on its own", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			asked createVM.Resources
			want  vm.Resources
		}{
			"memory asked for, the rest left": {
				asked: createVM.Resources{Memory: 4 * vmtest.GiB},
				want:  vm.Resources{CPUs: defaults.Resources.CPUs, Memory: 4 * vmtest.GiB, Disk: defaults.Resources.Disk},
			},
			"cpus and disk asked for, memory left": {
				asked: createVM.Resources{CPUs: 3, Disk: 30 * vmtest.GiB},
				want:  vm.Resources{CPUs: 3, Memory: defaults.Resources.Memory, Disk: 30 * vmtest.GiB},
			},
			"nothing asked for at all": {
				want: defaults.Resources,
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				w := vmtest.New()

				chosen, refused, err := chooserOf(w).Choose(ctx, "owner", Choice{New: &New{Resources: &tt.asked}})
				require.NoError(t, err)
				require.Empty(t, refused)

				assert.True(t, chosen.Created)
				assert.Equal(t, tt.want, chosen.VM.Resources)
			})
		}
	})

	t.Run("ports and a network not given are the defaults, and ports given empty are none", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()
		chooser := chooserOf(w)

		left, refused, err := chooser.Choose(ctx, "owner", Choice{New: &New{Name: "left"}})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.Equal(t, defaults.Ports, left.VM.Ports)
		assert.Equal(t, defaults.Network, left.VM.Network)

		none, refused, err := chooser.Choose(ctx, "owner", Choice{New: &New{Name: "none", Ports: []port.Port{}}})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.Empty(t, none.VM.Ports)
	})

	t.Run("what one made for it is refused for is reported where it was asked", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		_, refused, err := chooserOf(w).Choose(ctx, "owner", Choice{New: &New{
			Resources: &createVM.Resources{CPUs: 64, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB},
		}})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm.new.resources.cpus": "too_large"}, refused)
	})

	t.Run("one no node has room for is refused, and not kept", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: full.Capacity.Memory}

		w := vmtest.New(vmtest.WithNodes([]node.Node{full}...))

		_, refused, err := chooserOf(w).Choose(ctx, "owner", Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "no_capacity"}, refused)
		assert.Zero(t, w.VMs.Len())
	})

	t.Run("a docker vm is somebody's", func(t *testing.T) {
		t.Parallel()

		_, refused, err := chooserOf(vmtest.New()).Choose(ctx, "", Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"owner_uuid": "required_field"}, refused)
	})
}
