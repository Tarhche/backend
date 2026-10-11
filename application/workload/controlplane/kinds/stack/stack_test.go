package stack_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// asked is a stack somebody asks for, of shop's file, as admission is handed
// one.
func asked(name string, choice stackKind.VMChoice) stackKind.Stack {
	return stackKind.Stack{
		Kind:     stackKind.Name,
		Metadata: kind.Metadata{Name: name, OwnerUUID: "owner"},
		Spec:     stackKind.Spec{VM: choice, Compose: shop},
	}
}

// holding has slugs that start with prefix held.
func holding(prefix string) slugs.Taken {
	return func(_ context.Context, slug string) (bool, error) {
		return strings.HasPrefix(slug, prefix), nil
	}
}

func TestStacks_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("into the docker vm it names, which it lives in, keeping what to make it with again, deploying", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("01", "owner")))

		admitted, invalid, err := g.strategy.Admit(ctx, asked("  Web Shop ", stackKind.VMChoice{UUID: "01"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		v, _ := g.Stored("01")

		assert.Equal(t, "Web Shop", admitted.Metadata.Name)
		assert.True(t, strings.HasPrefix(admitted.Metadata.Slug, "web-shop-"), admitted.Metadata.Slug)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "01"}}, admitted.Metadata.Refs, "it lives in its vm")
		assert.Empty(t, admitted.Metadata.Owners, "which does not own it: it is a root")
		assert.Empty(t, admitted.Metadata.Node, "and no node keeps any of it")
		assert.Equal(t, vmKind.ChoiceOf(v), admitted.Spec.VM, "its vm as its record has it, its slug among it")
		assert.Equal(t, shop, admitted.Spec.Compose, "its file kept as it was written")
		assert.Equal(t, stackKind.Deploying, admitted.Status.State, "its graph makes it")
		assert.Equal(t, stackKind.Running, admitted.Status.Expected)
		assert.Empty(t, admitted.Status.Reason)
		assert.Zero(t, admitted.Metadata.Lifetime, "a stack is kept until it is deleted")
	})

	t.Run("into one its graph is to make when it names none, planned now and kept nowhere, though its owner has a docker vm already", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("01", "owner")))

		admitted, invalid, err := g.strategy.Admit(ctx, asked("web", stackKind.VMChoice{Name: "builds", Resources: &stackKind.Resources{Memory: 4 * vmtest.GiB}, Ports: []port.Port{}}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		planned := admitted.Spec.VM.UUID
		require.NotEmpty(t, planned)
		assert.NotEqual(t, "01", planned)

		_, kept := g.Stored(planned)
		assert.False(t, kept, "its graph makes it, once the stack is kept")

		assert.Equal(t, stackKind.VMChoice{
			UUID:      planned,
			Name:      "builds",
			Resources: &stackKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB},
			Ports:     []port.Port{},
			Network:   &stackKind.Network{Ingress: "allow", Egress: "allow"},
		}, admitted.Spec.VM, "as it was asked for, the defaults filled in, and no slug before it is made")
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: planned}}, admitted.Metadata.Refs)
		assert.Equal(t, stackKind.Deploying, admitted.Status.State)
		assert.Equal(t, "its vm is being made", admitted.Status.Reason)
	})

	t.Run("into one coming up, saying so", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Starting })))

		admitted, invalid, err := g.strategy.Admit(ctx, asked("web", stackKind.VMChoice{UUID: "01"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, "its vm is starting", admitted.Status.Reason)
	})

	t.Run("into the vm it names as its parent, among its refs or, as a manifest written before refs does, among its owners", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner")))

		refs := asked("web", stackKind.VMChoice{})
		refs.Metadata.Refs = []kind.Reference{{Kind: "vm", UUID: "02"}}

		admitted, invalid, err := g.strategy.Admit(ctx, refs)
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, "02", admitted.Spec.VM.UUID)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "02"}}, admitted.Metadata.Refs)

		owners := asked("web", stackKind.VMChoice{})
		owners.Metadata.Owners = []kind.Reference{{Kind: "vm", UUID: "02"}}

		admitted, invalid, err = g.strategy.Admit(ctx, owners)
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, "02", admitted.Spec.VM.UUID)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "02"}}, admitted.Metadata.Owners, "kept as they are given")
	})

	t.Run("given a slug nothing else holds", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("01", "owner")))

		_, _, err := g.strategyHolding(holding("web-")).Admit(ctx, asked("web", stackKind.VMChoice{UUID: "01"}))
		assert.ErrorIs(t, err, slugs.ErrExhausted, "every one it could be given is held")

		_, _, err = g.strategyHolding(holding("elsewhere-"), holding("web-")).Admit(ctx, asked("web", stackKind.VMChoice{UUID: "01"}))
		assert.ErrorIs(t, err, slugs.ErrExhausted, "by a stack or by anything else")
	})

	t.Run("as a dry run, its vm planned and nothing kept", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t)

		admitted, invalid, err := g.strategy.Admit(kind.DryRun(ctx), asked("web", stackKind.VMChoice{}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.NotEmpty(t, admitted.Spec.VM.UUID)
		assert.Zero(t, g.Memory.Len(vmKind.Name), "no vm is kept")
	})

	t.Run("one whose vm its graph is to make is held to the vms planned for its owner's stacks", func(t *testing.T) {
		t.Parallel()

		g := newStackGraph(t, vmtest.WithVMs(vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner")))

		planned := func(uuid string) kind.Raw {
			return aStack(uuid, uuid+"-vm", shop, func(s *stackKind.Stack) {
				s.Metadata.Slug = uuid
				s.Spec.VM.Resources = &stackKind.Resources{CPUs: 2}
			})
		}

		// two docker vms of two cpus, and a stack's planned of two, of the
		// eight its owner may be given.
		g.Keep(planned("s-1"))

		_, invalid, err := g.strategy.Admit(ctx, asked("web", stackKind.VMChoice{Resources: &stackKind.Resources{CPUs: 2}}))
		require.NoError(t, err)
		require.Empty(t, invalid, "two more fit")

		g.Keep(planned("s-2"))

		_, invalid, err = g.strategy.Admit(ctx, asked("web", stackKind.VMChoice{Resources: &stackKind.Resources{CPUs: 2}}))
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm.resources.cpus": "quota_exceeded"}, invalid, "and no more, counting what its graphs are to make")
	})

	for name, tt := range map[string]struct {
		vms    []vmKind.VM
		change func(*stackKind.Stack)
		want   domain.ValidationErrors
	}{
		"a stack with no name": {
			change: func(s *stackKind.Stack) { s.Metadata.Name = "  " },
			want:   domain.ValidationErrors{"name": "required_field"},
		},
		"one with a name nobody could show": {
			change: func(s *stackKind.Stack) { s.Metadata.Name = strings.Repeat("a", 101) },
			want:   domain.ValidationErrors{"name": "invalid_name"},
		},
		"one with no file": {
			change: func(s *stackKind.Stack) { s.Spec.Compose = " \n" },
			want:   domain.ValidationErrors{"compose": "required_field"},
		},
		"one whose file is not YAML": {
			change: func(s *stackKind.Stack) { s.Spec.Compose = "nope: [" },
			want:   domain.ValidationErrors{"compose": "invalid_value"},
		},
		"one whose file says what the platform does not carry out, each under its path": {
			change: func(s *stackKind.Stack) {
				s.Spec.Compose = "version: '3'\nservices:\n  web:\n    image: nginx\n    build: .\n    restart: always\n"
			},
			want: domain.ValidationErrors{"compose.version": "unsupported_key", "compose.services.web.build": "unsupported_key"},
		},
		"one whose file says an anchor and an alias of it, refused before anything else of it is read": {
			change: func(s *stackKind.Stack) {
				s.Spec.Compose = "services:\n  web: &web\n    image: nginx\n    build: .\n  api: *web\n"
			},
			want: domain.ValidationErrors{"compose.services.web": "unsupported_alias", "compose.services.api": "unsupported_alias"},
		},
		"one with more services than a stack may have": {
			change: func(s *stackKind.Stack) {
				s.Spec.Compose = "services:\n"

				for i := range stackKind.MaxServices + 1 {
					s.Spec.Compose += fmt.Sprintf("  s%d: {image: nginx}\n", i)
				}
			},
			want: domain.ValidationErrors{"compose.services": "too_large"},
		},
		"one whose file names what it does not have": {
			change: func(s *stackKind.Stack) {
				s.Spec.Compose = "services:\n  web:\n    image: nginx\n    depends_on: [db]\n    restart: always\n"
			},
			want: domain.ValidationErrors{"compose.services.web.depends_on.db": "undefined_service", "compose.services.web.restart": "invalid_restart"},
		},
		"one whose services depend on one another in a ring, each of them": {
			vms: []vmKind.VM{vmtest.Docker("01", "owner")},
			change: func(s *stackKind.Stack) {
				s.Spec.VM = stackKind.VMChoice{UUID: "01"}
				s.Spec.Compose = "services:\n  web:\n    image: nginx\n    depends_on: [api]\n  api:\n    image: api\n    depends_on: [db]\n  db:\n    image: postgres\n    depends_on: [web]\n  cache:\n    image: redis\n"
			},
			want: domain.ValidationErrors{
				"compose.services.web.depends_on": "depends_on_cycle",
				"compose.services.api.depends_on": "depends_on_cycle",
				"compose.services.db.depends_on":  "depends_on_cycle",
			},
		},
		"one depending on itself": {
			vms: []vmKind.VM{vmtest.Docker("01", "owner")},
			change: func(s *stackKind.Stack) {
				s.Spec.VM = stackKind.VMChoice{UUID: "01"}
				s.Spec.Compose = "services:\n  web:\n    image: nginx\n    depends_on: [web]\n"
			},
			want: domain.ValidationErrors{"compose.services.web.depends_on": "depends_on_cycle"},
		},
		"one naming a vm and asking for a new one": {
			change: func(s *stackKind.Stack) { s.Spec.VM = stackKind.VMChoice{UUID: "01", Name: "builds"} },
			want:   domain.ValidationErrors{"vm": "vm_or_new_vm"},
		},
		"one into a docker vm that is not coming up": {
			vms:    []vmKind.VM{vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })},
			change: func(s *stackKind.Stack) { s.Spec.VM = stackKind.VMChoice{UUID: "01"} },
			want:   domain.ValidationErrors{"vm": "vm_not_running"},
		},
		"one into a vm that is no docker vm": {
			vms:    []vmKind.VM{vmtest.Running("03", "owner")},
			change: func(s *stackKind.Stack) { s.Spec.VM = stackKind.VMChoice{UUID: "03"} },
			want:   domain.ValidationErrors{"vm.uuid": "not_docker"},
		},
		"one into somebody else's": {
			vms:    []vmKind.VM{vmtest.Docker("01", "somebody")},
			change: func(s *stackKind.Stack) { s.Spec.VM = stackKind.VMChoice{UUID: "01"} },
			want:   domain.ValidationErrors{"vm.uuid": "not_found"},
		},
		"one whose vm its graph is to make would be given more than one may": {
			change: func(s *stackKind.Stack) { s.Spec.VM = stackKind.VMChoice{Resources: &stackKind.Resources{CPUs: 64}} },
			want:   domain.ValidationErrors{"vm.resources.cpus": "too_large"},
		},
	} {
		t.Run("refused: "+name+", and makes no vm", func(t *testing.T) {
			t.Parallel()

			g := newStackGraph(t, vmtest.WithVMs(tt.vms...))

			s := asked("web", stackKind.VMChoice{})
			tt.change(&s)

			_, invalid, err := g.strategy.Admit(ctx, s)
			require.NoError(t, err)
			assert.Equal(t, tt.want, invalid)

			assert.Equal(t, len(tt.vms), g.Memory.Len(vmKind.Name))
		})
	}
}

func TestStacks_Reconcile(t *testing.T) {
	t.Parallel()

	g := newStackGraph(t)

	for _, state := range stackKind.Machine().States {
		t.Run("asks for nothing of one "+string(state)+": its graph carries it out", func(t *testing.T) {
			t.Parallel()

			s := stackKind.Stack{Kind: stackKind.Name, Status: stackKind.Status{Status: kind.Status{State: state, Expected: stackKind.Running}}}

			intents, err := g.strategy.Reconcile(context.Background(), s)
			require.NoError(t, err)
			assert.Empty(t, intents)
		})
	}
}

func TestStacks_Apply(t *testing.T) {
	t.Parallel()

	g := newStackGraph(t)

	for _, action := range []string{stackKind.ActionStart, stackKind.ActionStop, stackKind.ActionRestart, stackKind.ActionDelete} {
		t.Run(action+" is its graph's to carry out, and lets go of what its plan failed with", func(t *testing.T) {
			t.Parallel()

			s := stackKind.Stack{
				Kind:   stackKind.Name,
				Status: stackKind.Status{Status: kind.Status{State: stackKind.Starting, Reason: "its vm is stopped"}, Failure: &noderequest.Error{Code: "no_capacity", Message: "no room"}},
			}

			applied, invalid, err := g.strategy.Apply(context.Background(), s, action, nil)
			require.NoError(t, err)
			require.Empty(t, invalid)

			assert.Nil(t, applied.Status.Failure)
			assert.Equal(t, stackKind.Starting, applied.Status.State, "as its machine moved it")
		})
	}

	t.Run("an action it does not have is none of its", func(t *testing.T) {
		t.Parallel()

		_, _, err := g.strategy.Apply(context.Background(), stackKind.Stack{}, "apply", nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

func TestStacks_Outlive(t *testing.T) {
	t.Parallel()

	g := newStackGraph(t)

	went := vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) {
		v.Metadata.Name = "renamed"
		v.Metadata.Slug = "builds-abcde"
		v.Spec.Resources.CPUs = 3
	})

	parent, err := kind.Encode(went)
	require.NoError(t, err)

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](aStack("s-1", "01", shop))
	require.NoError(t, err)

	t.Run("a stack kept as gone with its vm keeps its vm as the vm's record had it as it went, and its slug", func(t *testing.T) {
		t.Parallel()

		kept, err := g.strategy.Outlive(s, parent)
		require.NoError(t, err)

		assert.Equal(t, vmKind.ChoiceOf(went), kept.Spec.VM)
		assert.Equal(t, "builds-abcde", kept.Spec.VM.Slug)
	})

	t.Run("but a parent that is not its vm says nothing of it", func(t *testing.T) {
		t.Parallel()

		other := parent
		other.Metadata.UUID = "02"

		kept, err := g.strategy.Outlive(s, other)
		require.NoError(t, err)

		assert.Equal(t, s.Spec.VM, kept.Spec.VM)
	})
}
