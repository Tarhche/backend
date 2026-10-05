package updateVM

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func useCaseOf(w *vmtest.Workload) *UseCase {
	return NewUseCase(w.VMs, w.Quota, w.Placement, w.Lifecycle, validator.New(translator.Codes{}))
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a new name and lifetime are the record's alone", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		name := "  renamed  "
		lifetime := int64(3600)

		response, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "01", Name: &name, LifetimeSeconds: &lifetime})
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, "renamed", stored.Name)
		assert.Equal(t, time.Hour, stored.Lifetime)
		assert.WithinDuration(t, time.Now().Add(time.Hour), stored.ExpiresAt, 5*time.Second)
		assert.Equal(t, vm.Running, stored.CurrentState, "nothing is restarted for them")
		assert.Empty(t, w.Producer.Messages())
	})

	t.Run("a lifetime of nothing keeps it until it is deleted", func(t *testing.T) {
		t.Parallel()

		expiring := vmtest.Running("01", "owner")
		expiring.Lifetime = time.Hour
		expiring.ExpiresAt = time.Now().Add(time.Hour)

		w := vmtest.New(vmtest.WithVMs(expiring))

		forever := int64(0)
		_, err := useCaseOf(w).Execute(ctx, &Request{UUID: "01", LifetimeSeconds: &forever})
		require.NoError(t, err)

		stored, _ := w.VMs.Stored("01")
		assert.Zero(t, stored.Lifetime)
		assert.True(t, stored.ExpiresAt.IsZero())
	})

	t.Run("new ports restart a running vm with them", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		ports := []port.Port{443, 80, 443}
		response, err := useCaseOf(w).Execute(ctx, &Request{UUID: "01", Ports: &ports, Network: &createVM.Network{Egress: vm.AccessDeny}})
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, []port.Port{80, 443}, stored.Ports)
		assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, stored.Network, "the way it named changed, the other stayed")
		assert.Equal(t, vm.Restarting, stored.CurrentState)

		var reconfigure events.VMReconfigureRequested
		require.True(t, w.Producer.Last(events.VMReconfigureRequestedName, &reconfigure))
		assert.Equal(t, []port.Port{80, 443}, reconfigure.Spec.Ports)
		assert.Equal(t, vm.AccessDeny, reconfigure.Spec.Network.Egress)
	})

	t.Run("a vm that grows is held to what it may be given", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		grown := createVM.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}
		response, err := useCaseOf(w).Execute(ctx, &Request{UUID: "01", Resources: &grown})
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, grown.VM(), stored.Resources)
	})

	for name, tt := range map[string]struct {
		vm        vm.VM
		nodes     []node.Node
		resources createVM.Resources
		ports     []port.Port
		want      domain.ValidationErrors
	}{
		"a disk does not shrink": {
			vm:        vmtest.Running("01", "owner"),
			resources: createVM.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: 5 * vmtest.GiB},
			want:      domain.ValidationErrors{"resources.disk": "disk_cannot_shrink"},
		},
		"more than one vm may be given": {
			vm:        vmtest.Running("01", "owner"),
			resources: createVM.Resources{CPUs: 8, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB},
			want:      domain.ValidationErrors{"resources.cpus": "too_large"},
		},
		"more than its node has room for": {
			vm: vmtest.Running("01", "owner"),
			nodes: []node.Node{func() node.Node {
				n := vmtest.Alive(vmtest.Node)
				n.Capacity.Memory = 2 * vmtest.GiB
				n.Capacity.Allocated = vm.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB}

				return n
			}()},
			resources: createVM.Resources{CPUs: 1, Memory: 4 * vmtest.GiB, Disk: 10 * vmtest.GiB},
			want:      domain.ValidationErrors{"resources": "no_capacity"},
		},
		"a vm on its way somewhere": {
			vm: func() vm.VM {
				v := vmtest.Running("01", "owner")
				v.CurrentState = vm.Starting

				return v
			}(),
			ports: []port.Port{80},
			want:  domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"a vm on its way out": {
			vm: func() vm.VM {
				v := vmtest.Running("01", "owner")
				v.CurrentState = vm.Deleting

				return v
			}(),
			ports: []port.Port{80},
			want:  domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithVMs(tt.vm)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			w := vmtest.New(opts...)

			request := &Request{UUID: "01"}
			if tt.resources != (createVM.Resources{}) {
				request.Resources = &tt.resources
			}

			if tt.ports != nil {
				request.Ports = &tt.ports
			}

			response, err := useCaseOf(w).Execute(ctx, request)
			require.NoError(t, err)
			assert.Equal(t, tt.want, response.ValidationErrors)

			stored, _ := w.VMs.Stored("01")
			assert.Equal(t, tt.vm.Resources, stored.Resources, "nothing is written down")
			assert.Empty(t, w.Producer.Messages())
		})
	}

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		name := "mine"
		_, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "other", UUID: "01", Name: &name})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
