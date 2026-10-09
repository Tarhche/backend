package dockervm_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestChooser_Choose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("the docker vm named, of the person asking", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner")))

		chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{UUID: "02"})
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.Equal(t, "02", chosen.VM.Metadata.UUID)
		assert.False(t, chosen.Created)
	})

	for name, tt := range map[string]struct {
		vms  []vmKind.VM
		want domain.ValidationErrors
	}{
		"one that is not there": {
			want: domain.ValidationErrors{"vm.uuid": "not_found"},
		},
		"somebody else's": {
			vms:  []vmKind.VM{vmtest.Docker("01", "other")},
			want: domain.ValidationErrors{"vm.uuid": "not_found"},
		},
		"one on its way out": {
			vms:  []vmKind.VM{vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Deleting })},
			want: domain.ValidationErrors{"vm.uuid": "not_found"},
		},
		"a machine": {
			vms:  []vmKind.VM{vmtest.Running("01", "owner")},
			want: domain.ValidationErrors{"vm.uuid": "not_docker"},
		},
	} {
		t.Run("not "+name, func(t *testing.T) {
			t.Parallel()

			_, refused, err := vmtest.New(vmtest.WithVMs(tt.vms...)).Chooser.Choose(ctx, "owner", dockervm.Choice{UUID: "01"})
			require.NoError(t, err)
			assert.Equal(t, tt.want, refused)
		})
	}

	t.Run("their only docker vm, when none is named: machines and those on their way out are not", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(
			vmtest.Docker("01", "owner"),
			vmtest.Running("02", "owner"),
			vmtest.In(vmtest.Docker("03", "owner"), func(v *vmKind.VM) { v.Status.Expected = vmKind.Deleted }),
			vmtest.Docker("04", "other"),
		))

		chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{})
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.Equal(t, "01", chosen.VM.Metadata.UUID)
	})

	t.Run("one of several has to be named", func(t *testing.T) {
		t.Parallel()

		_, refused, err := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner"))).Chooser.Choose(ctx, "owner", dockervm.Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "vm_required"}, refused)
	})

	t.Run("one is made for somebody who has none, with the defaults, as any vm is admitted, and is asked of its node at once", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("02", "owner")))

		chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{})
		require.NoError(t, err)
		require.Empty(t, refused)
		require.True(t, chosen.Created)

		made, kept := w.Stored(chosen.VM.Metadata.UUID)
		require.True(t, kept)

		assert.Equal(t, "docker", made.Metadata.Name)
		assert.Equal(t, "owner", made.Metadata.OwnerUUID)
		assert.Equal(t, "docker:29-dind", made.Spec.Image, "the docker image, which makes it a docker vm")
		assert.Equal(t, vmtest.DockerDefaults.Resources, made.Spec.Resources)
		assert.Equal(t, []port.Port{80}, made.Spec.Ports)
		assert.Equal(t, vmtest.DockerDefaults.Network, made.Spec.Network)
		assert.Equal(t, vmtest.Node, made.Metadata.Node)
		assert.Equal(t, vmKind.Scheduled, made.Status.State, "its create was sent")
		assert.Equal(t, vmKind.Scheduled, chosen.VM.Status.State, "and what was chosen says so")

		var command kind.ActOnResource
		require.True(t, w.Producer.Last(kind.ActOnResourceName, &command))
		assert.Equal(t, vmKind.ActionCreate, command.Action)
		assert.Equal(t, made.Metadata.UUID, command.UUID)
	})

	t.Run("one asked for is the defaults with what it was given in place of each", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{New: &dockervm.New{
			Name:      "builds",
			Resources: &vmKind.Resources{Memory: 4 * vmtest.GiB},
			Ports:     []port.Port{},
			Network:   &vmKind.Network{Egress: vm.AccessDeny},
		}})
		require.NoError(t, err)
		require.Empty(t, refused)

		made, _ := w.Stored(chosen.VM.Metadata.UUID)
		assert.Equal(t, "builds", made.Metadata.Name)
		assert.Equal(t, vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}, made.Spec.Resources, "a size of zero is the default")
		assert.Equal(t, []port.Port{}, made.Spec.Ports, "ports given empty are none")
		assert.Equal(t, vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, made.Spec.Network, "the way not named is the default")
	})

	t.Run("what one asked for is refused for is said where it was asked", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		_, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{New: &dockervm.New{Resources: &vmKind.Resources{CPUs: 64}}})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm.new.resources.cpus": "too_large"}, refused)
		assert.Zero(t, w.Memory.Len(vmKind.Name), "nothing is made")
	})

	t.Run("one no node has room for is not made", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: 64 * vmtest.GiB}

		w := vmtest.New(vmtest.WithNodes(full))

		_, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": dockervm.ReasonNoCapacity}, refused)
		assert.Zero(t, w.Memory.Len(vmKind.Name), "nobody asked for it but what cannot go into it")
	})

	t.Run("somebody has to be asking", func(t *testing.T) {
		t.Parallel()

		_, refused, err := vmtest.New().Chooser.Choose(ctx, "", dockervm.Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"owner_uuid": "required_field"}, refused)
	})
}

func TestChoice_JSON(t *testing.T) {
	t.Parallel()

	var choice dockervm.Choice
	require.NoError(t, json.Unmarshal([]byte(`{"uuid":"01","new":{"name":"builds","resources":{"cpus":2,"memory":1,"disk":2},"ports":[80],"network":{"ingress":"deny","egress":"allow"}}}`), &choice))

	assert.Equal(t, dockervm.Choice{UUID: "01", New: &dockervm.New{
		Name:      "builds",
		Resources: &vmKind.Resources{CPUs: 2, Memory: 1, Disk: 2},
		Ports:     []port.Port{80},
		Network:   &vmKind.Network{Ingress: vm.AccessDeny, Egress: vm.AccessAllow},
	}}, choice, "as a request has always named one")
}

func TestNetworkOf(t *testing.T) {
	t.Parallel()

	network, valid := dockervm.NetworkOf("allow", "deny")
	assert.True(t, valid)
	assert.Equal(t, vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, network)

	_, valid = dockervm.NetworkOf("open", "deny")
	assert.False(t, valid)

	_, valid = dockervm.NetworkOf("allow", "")
	assert.False(t, valid)
}
