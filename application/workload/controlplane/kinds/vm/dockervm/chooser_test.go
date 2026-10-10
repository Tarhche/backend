package dockervm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
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
		assert.Equal(t, "02", chosen.Metadata.UUID)
		assert.Equal(t, 2, w.Memory.Len(vmKind.Name), "and none is made")
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

	t.Run("one is made when none is named, whichever docker vms its owner has already", func(t *testing.T) {
		t.Parallel()

		for name, vms := range map[string][]vmKind.VM{
			"none":    {vmtest.Running("01", "owner")},
			"one":     {vmtest.Docker("01", "owner")},
			"several": {vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner")},
		} {
			w := vmtest.New(vmtest.WithVMs(vms...))

			chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{})
			require.NoError(t, err, name)
			require.Empty(t, refused, name)

			_, kept := w.Stored(chosen.Metadata.UUID)
			assert.True(t, kept, name)
			assert.Equal(t, len(vms)+1, w.Memory.Len(vmKind.Name), "with %s of its own already", name)
		}
	})

	t.Run("one made with the defaults is admitted as any vm is, and is asked of its node at once", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("02", "owner")))

		chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{})
		require.NoError(t, err)
		require.Empty(t, refused)

		made, kept := w.Stored(chosen.Metadata.UUID)
		require.True(t, kept)

		assert.Equal(t, "docker", made.Metadata.Name)
		assert.Equal(t, "owner", made.Metadata.OwnerUUID)
		assert.Equal(t, "docker:29-dind", made.Spec.Image, "the docker image, which makes it a docker vm")
		assert.Equal(t, vmtest.DockerDefaults.Resources, made.Spec.Resources)
		assert.Equal(t, []port.Port{80}, made.Spec.Ports)
		assert.Equal(t, vmtest.DockerDefaults.Network, made.Spec.Network)
		assert.Equal(t, vmtest.Node, made.Metadata.Node)
		assert.Equal(t, vmKind.Scheduled, made.Status.State, "its create was sent")
		assert.Equal(t, vmKind.Scheduled, chosen.Status.State, "and what was chosen says so")

		var command kind.ActOnResource
		require.True(t, w.Producer.Last(kind.ActOnResourceName, &command))
		assert.Equal(t, vmKind.ActionCreate, command.Action)
		assert.Equal(t, made.Metadata.UUID, command.UUID)
	})

	t.Run("one asked for is the defaults with what it was given in place of each", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{
			Name:      "builds",
			Resources: &vmKind.Resources{Memory: 4 * vmtest.GiB},
			Ports:     []port.Port{},
			Network:   &vmKind.Network{Egress: vm.AccessDeny},
		})
		require.NoError(t, err)
		require.Empty(t, refused)

		made, _ := w.Stored(chosen.Metadata.UUID)
		assert.Equal(t, "builds", made.Metadata.Name)
		assert.Equal(t, vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}, made.Spec.Resources, "a size of zero is the default")
		assert.Equal(t, []port.Port{}, made.Spec.Ports, "ports given empty are none")
		assert.Equal(t, vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, made.Spec.Network, "the way not named is the default")
	})

	t.Run("what one asked for is refused for is said where it was asked", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		_, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{Resources: &vmKind.Resources{CPUs: 64}})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm.resources.cpus": "too_large"}, refused)
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

	t.Run("in a dry run, one is admitted as a dry run, and nothing is kept or sent", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		chosen, refused, err := w.Chooser.Choose(kind.DryRun(ctx), "owner", dockervm.Choice{Name: "builds"})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.NotEmpty(t, chosen.Metadata.UUID, "under a uuid nothing holds")
		assert.Equal(t, "builds", chosen.Metadata.Name)
		assert.NotEmpty(t, chosen.Metadata.Slug)
		assert.Equal(t, vmtest.Node, chosen.Metadata.Node, "where it would be placed")
		assert.Equal(t, vmKind.Scheduled, chosen.Status.State, "as its create would leave it")
		assert.True(t, vmKind.Up(chosen), "on its way up, as one made for what goes into it always is")

		assert.Zero(t, w.Memory.Len(vmKind.Name), "nothing is kept")

		var command kind.ActOnResource
		assert.False(t, w.Producer.Last(kind.ActOnResourceName, &command), "nor sent")
	})

	t.Run("and one no node has room for is refused, with nothing kept to take away", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: 64 * vmtest.GiB}

		w := vmtest.New(vmtest.WithNodes(full))

		_, refused, err := w.Chooser.Choose(kind.DryRun(ctx), "owner", dockervm.Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": dockervm.ReasonNoCapacity}, refused)
		assert.Zero(t, w.Memory.Len(vmKind.Name))
	})

	t.Run("somebody has to be asking", func(t *testing.T) {
		t.Parallel()

		_, refused, err := vmtest.New().Chooser.Choose(ctx, "", dockervm.Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"owner_uuid": "required_field"}, refused)
	})
}

func TestChooser_Make(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("one is made under the uuid it is given, as one made for what names none is", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		made, refused, err := w.Chooser.Make(ctx, "owner", "vm-held", dockervm.Choice{UUID: "another", Name: "builds", Ports: []port.Port{}})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.Equal(t, "vm-held", made.Metadata.UUID, "and not under any it names")

		kept, there := w.Stored("vm-held")
		require.True(t, there)
		assert.Equal(t, "builds", kept.Metadata.Name)
		assert.Equal(t, "owner", kept.Metadata.OwnerUUID)
		assert.True(t, vmKind.DockerVM(kept, vmtest.Images.Docker))
		assert.Equal(t, []port.Port{}, kept.Spec.Ports)
		assert.Equal(t, vmtest.DockerDefaults.Resources, kept.Spec.Resources)

		var command kind.ActOnResource
		require.True(t, w.Producer.Last(kind.ActOnResourceName, &command))
		assert.Equal(t, vmKind.ActionCreate, command.Action, "it is asked of its node at once")
		assert.Equal(t, "vm-held", command.UUID)
	})

	t.Run("one made again asks for the slug it had, and is reached where it was", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		made, refused, err := w.Chooser.Make(ctx, "owner", "vm-held", dockervm.Choice{Name: "builds", Slug: "builds-abcde"})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.Equal(t, "builds-abcde", made.Metadata.Slug)

		kept, there := w.Stored("vm-held")
		require.True(t, there)
		assert.Equal(t, "builds-abcde", kept.Metadata.Slug)
	})

	t.Run("but one asked for asks for none, whatever it says", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		chosen, refused, err := w.Chooser.Choose(ctx, "owner", dockervm.Choice{Name: "builds", Slug: "builds-abcde"})
		require.NoError(t, err)
		require.Empty(t, refused)

		assert.NotEqual(t, "builds-abcde", chosen.Metadata.Slug)
		assert.True(t, strings.HasPrefix(chosen.Metadata.Slug, "builds-"), chosen.Metadata.Slug)
	})

	t.Run("but not under one another is kept under already", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("vm-held", "owner")))

		_, _, err := w.Chooser.Make(ctx, "owner", "vm-held", dockervm.Choice{})

		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.Equal(t, 1, w.Memory.Len(vmKind.Name))
	})

	t.Run("one no node has room for is not kept", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: 64 * vmtest.GiB}

		w := vmtest.New(vmtest.WithNodes(full))

		_, refused, err := w.Chooser.Make(ctx, "owner", "vm-held", dockervm.Choice{})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"vm": dockervm.ReasonNoCapacity}, refused)
		assert.Zero(t, w.Memory.Len(vmKind.Name))
	})

	for name, tt := range map[string]struct {
		owner string
		uuid  string
		want  domain.ValidationErrors
	}{
		"somebody has to be asking":  {uuid: "vm-held", want: domain.ValidationErrors{"owner_uuid": "required_field"}},
		"and a uuid has to be given": {owner: "owner", want: domain.ValidationErrors{"vm.uuid": "required_field"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New()

			_, refused, err := w.Chooser.Make(ctx, tt.owner, tt.uuid, dockervm.Choice{})
			require.NoError(t, err)

			assert.Equal(t, tt.want, refused)
			assert.Zero(t, w.Memory.Len(vmKind.Name))
		})
	}
}

// containerIn is a container of ownerUUID's whose spec keeps the vm it goes
// into as vm, as changes say otherwise.
func containerIn(uuid string, ownerUUID string, vm stackKind.VMChoice, changes ...func(c *containerKind.Container)) resource.Record {
	c := containerKind.Container{
		Kind:     containerKind.Name,
		Metadata: kind.Metadata{UUID: uuid, OwnerUUID: ownerUUID, Refs: []kind.Reference{{Kind: vmKind.Name, UUID: vm.UUID}}},
		Spec:     containerKind.Spec{VM: vm, Image: "nginx"},
		Status:   containerKind.Status{Status: kind.Status{State: containerKind.Pending, Expected: containerKind.Running}},
	}

	for _, change := range changes {
		change(&c)
	}

	return vmtest.Record(c)
}

func TestChooser_Plan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("one is planned as one made for what names none is, under a uuid of its own, and kept nowhere", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: 64 * vmtest.GiB}

		w := vmtest.New(vmtest.WithNodes(full))

		planned, refused, err := w.Chooser.Plan(ctx, "owner", dockervm.Choice{UUID: "another", Name: "builds", Resources: &vmKind.Resources{Memory: 4 * vmtest.GiB}})
		require.NoError(t, err)
		require.Empty(t, refused, "no node's room is looked at")

		parsed, err := uuid.FromString(planned.Metadata.UUID)
		require.NoError(t, err, "and not under any it names")
		assert.Equal(t, byte(7), parsed.Version())

		assert.Equal(t, "builds", planned.Metadata.Name)
		assert.Equal(t, "owner", planned.Metadata.OwnerUUID)
		assert.Equal(t, vmtest.Images.Docker, planned.Spec.Image)
		assert.Equal(t, vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}, planned.Spec.Resources, "the defaults where it says nothing")
		assert.Equal(t, []port.Port{80}, planned.Spec.Ports)
		assert.Equal(t, vmtest.DockerDefaults.Network, planned.Spec.Network)
		assert.Empty(t, planned.Metadata.Node)

		assert.Zero(t, w.Memory.Len(vmKind.Name), "nothing is kept")
		assert.Empty(t, w.Producer.Messages(), "nor asked of a node")

		another, _, err := w.Chooser.Plan(ctx, "owner", dockervm.Choice{})
		require.NoError(t, err)
		assert.NotEqual(t, planned.Metadata.UUID, another.Metadata.UUID)
		assert.Equal(t, "docker", another.Metadata.Name)
	})

	for name, tt := range map[string]struct {
		vms        []vmKind.VM
		containers []resource.Record
		choice     dockervm.Choice
		want       domain.ValidationErrors
	}{
		"one past what one vm may be given is refused where it was asked for": {
			choice: dockervm.Choice{Resources: &vmKind.Resources{CPUs: 64}},
			want:   domain.ValidationErrors{"vm.resources.cpus": "too_large"},
		},
		"and one past its owner's quota": {
			vms:    []vmKind.VM{vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner"), vmtest.Docker("03", "owner")},
			choice: dockervm.Choice{Resources: &vmKind.Resources{CPUs: 3}},
			want:   domain.ValidationErrors{"vm.resources.cpus": "quota_exceeded"},
		},
		"the vms planned for them, and not made, counted as theirs": {
			vms: []vmKind.VM{vmtest.Docker("01", "owner")},
			containers: []resource.Record{
				containerIn("c-1", "owner", stackKind.VMChoice{UUID: "planned-1", Resources: &stackKind.Resources{CPUs: 3}}),
				containerIn("c-2", "owner", stackKind.VMChoice{UUID: "planned-1", Resources: &stackKind.Resources{CPUs: 3}}),
			},
			choice: dockervm.Choice{Resources: &vmKind.Resources{CPUs: 4}},
			want:   domain.ValidationErrors{"vm.resources.cpus": "quota_exceeded"},
		},
		"the defaults where what plans one says nothing": {
			vms: []vmKind.VM{vmtest.Docker("01", "owner")},
			containers: []resource.Record{
				containerIn("c-1", "owner", stackKind.VMChoice{UUID: "planned-1"}),
				containerIn("c-2", "owner", stackKind.VMChoice{UUID: "planned-2"}),
			},
			choice: dockervm.Choice{Resources: &vmKind.Resources{CPUs: 3}},
			want:   domain.ValidationErrors{"vm.resources.cpus": "quota_exceeded"},
		},
		"and each once, however many plan it": {
			vms: []vmKind.VM{vmtest.Docker("01", "owner")},
			containers: []resource.Record{
				containerIn("c-1", "owner", stackKind.VMChoice{UUID: "planned-1"}),
				containerIn("c-2", "owner", stackKind.VMChoice{UUID: "planned-1"}),
				containerIn("c-3", "owner", stackKind.VMChoice{UUID: "planned-2"}),
			},
			choice: dockervm.Choice{Resources: &vmKind.Resources{CPUs: 2}},
			want:   nil,
		},
		"but not what is made already, what was asked to go, or what is somebody else's": {
			vms: []vmKind.VM{vmtest.Docker("01", "owner")},
			containers: []resource.Record{
				containerIn("c-1", "owner", stackKind.VMChoice{UUID: "01", Resources: &stackKind.Resources{CPUs: 4}}),
				containerIn("c-2", "owner", stackKind.VMChoice{UUID: "planned-1", Resources: &stackKind.Resources{CPUs: 4}}, func(c *containerKind.Container) {
					c.Metadata.DeletionRequestedAt = time.Now()
					c.Status.Expected = kind.Deleted
				}),
				containerIn("c-3", "other", stackKind.VMChoice{UUID: "planned-2", Resources: &stackKind.Resources{CPUs: 4}}),
			},
			choice: dockervm.Choice{Resources: &vmKind.Resources{CPUs: 4}},
			want:   nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(tt.vms...))

			for _, c := range tt.containers {
				_, err := w.Memory.Create(ctx, c)
				require.NoError(t, err)
			}

			_, refused, err := w.Chooser.Plan(ctx, "owner", tt.choice)
			require.NoError(t, err)

			assert.Equal(t, tt.want, refused)
			assert.Equal(t, len(tt.vms), w.Memory.Len(vmKind.Name), "and nothing is kept")
		})
	}

	t.Run("somebody has to be asking", func(t *testing.T) {
		t.Parallel()

		_, refused, err := vmtest.New().Chooser.Plan(ctx, "", dockervm.Choice{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"owner_uuid": "required_field"}, refused)
	})
}

func TestChoice_JSON(t *testing.T) {
	t.Parallel()

	var choice dockervm.Choice
	require.NoError(t, json.Unmarshal([]byte(`{"name":"builds","resources":{"cpus":2,"memory":1,"disk":2},"ports":[80],"network":{"ingress":"deny","egress":"allow"}}`), &choice))

	assert.Equal(t, dockervm.Choice{
		Name:      "builds",
		Resources: &vmKind.Resources{CPUs: 2, Memory: 1, Disk: 2},
		Ports:     []port.Port{80},
		Network:   &vmKind.Network{Ingress: vm.AccessDeny, Egress: vm.AccessAllow},
	}, choice, "as a container's or a stack's spec names one")
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
