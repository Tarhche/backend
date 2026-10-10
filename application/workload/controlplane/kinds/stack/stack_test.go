package stack_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/stack"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const compose = `
name: ignored
services:
  web:
    image: nginx:1.27
    ports: ["80:80"]
`

// strategyOf is the stack kind's control-plane strategy over w's VMs, with
// slugs nobody holds unless taken says.
func strategyOf(w *vmtest.Workload, taken ...string) *stack.Stacks {
	return strategyOver(w, w.Records, taken...)
}

// strategyOver is strategyOf reading the VMs from vms.
func strategyOver(w *vmtest.Workload, vms *records.Records, taken ...string) *stack.Stacks {
	held := func(_ context.Context, slug string) (bool, error) {
		for _, t := range taken {
			if strings.HasPrefix(slug, t) {
				return true, nil
			}
		}

		return false, nil
	}

	return stack.New(vms, w.Chooser, slugs.Taken(held))
}

// unreadable is a store of resources that cannot be read.
type unreadable struct {
	resource.Repository
}

func (unreadable) GetOne(context.Context, string, string) (resource.Record, error) {
	return resource.Record{}, errors.New("the database is gone")
}

// asked is a stack somebody asks for, as admission is handed one.
func asked(name string, choice stackKind.VMChoice) stackKind.Stack {
	return stackKind.Stack{
		Kind:     stackKind.Name,
		Metadata: kind.Metadata{Name: name, OwnerUUID: "owner"},
		Spec:     stackKind.Spec{VM: choice, Compose: compose},
	}
}

func TestStacks_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a stack in a running docker vm is admitted into it, placed where it is, waiting to be deployed", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

		admitted, invalid, err := strategyOf(w).Admit(ctx, asked("  Web Site ", stackKind.VMChoice{UUID: "01"}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, "Web Site", admitted.Metadata.Name)
		assert.True(t, strings.HasPrefix(admitted.Metadata.Slug, "web-site-"), admitted.Metadata.Slug)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "01"}}, admitted.Metadata.Owners, "it belongs to its vm")
		assert.Equal(t, vmtest.Node, admitted.Metadata.Node, "and is where its vm is")
		assert.Equal(t, stackKind.VMChoice{UUID: "01"}, admitted.Spec.VM, "the one it named")
		assert.Equal(t, compose, admitted.Spec.Compose, "kept as it was written")
		assert.Equal(t, stackKind.Waiting, admitted.Status.State)
		assert.Equal(t, stackKind.Running, admitted.Status.Expected)
		assert.Empty(t, admitted.Status.Reason)
		assert.Zero(t, admitted.Metadata.Lifetime, "a stack is kept until it is deleted")
	})

	t.Run("one into a docker vm made for it as it describes it keeps the vm's uuid alone, and says what the vm waits on", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		admitted, invalid, err := strategyOf(w).Admit(ctx, asked("web", stackKind.VMChoice{Name: "builds", Resources: &stackKind.Resources{Memory: 4 * vmtest.GiB}}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		made, kept := w.Stored(admitted.Spec.VM.UUID)
		require.True(t, kept)
		assert.Equal(t, "builds", made.Metadata.Name)
		assert.True(t, vmKind.DockerVM(made, vmtest.Images.Docker))
		assert.Equal(t, vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}, made.Spec.Resources)
		assert.Equal(t, stackKind.VMChoice{UUID: made.Metadata.UUID}, admitted.Spec.VM, "what the vm was made with is the vm's own")

		assert.Equal(t, stackKind.Waiting, admitted.Status.State)
		assert.Equal(t, "its vm is scheduled", admitted.Status.Reason)
	})

	t.Run("one asking for nothing goes into a vm made with the defaults, though its owner has a docker vm already", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

		admitted, invalid, err := strategyOf(w).Admit(ctx, asked("web", stackKind.VMChoice{}))
		require.NoError(t, err)
		require.Empty(t, invalid)

		made, kept := w.Stored(admitted.Spec.VM.UUID)
		require.True(t, kept)
		assert.NotEqual(t, "01", made.Metadata.UUID)
		assert.Equal(t, "docker", made.Metadata.Name)
		assert.Equal(t, vmtest.DockerDefaults.Resources, made.Spec.Resources)
		assert.Equal(t, stackKind.VMChoice{UUID: made.Metadata.UUID}, admitted.Spec.VM)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: made.Metadata.UUID}}, admitted.Metadata.Owners)
	})

	t.Run("one naming its vm as its parent goes into it", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner"), vmtest.Docker("02", "owner")))

		parented := asked("web", stackKind.VMChoice{})
		parented.Metadata.Owners = []kind.Reference{{Kind: "vm", UUID: "02"}}

		admitted, invalid, err := strategyOf(w).Admit(ctx, parented)
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, "02", admitted.Spec.VM.UUID)
	})

	t.Run("it is given a slug nothing else holds", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner")))

		_, _, err := strategyOf(w, "web-").Admit(ctx, asked("web", stackKind.VMChoice{UUID: "01"}))
		assert.ErrorIs(t, err, slugs.ErrExhausted, "every one it could be given is held")

		byStacks := func(context.Context, string) (bool, error) { return false, nil }
		byVMs := func(_ context.Context, slug string) (bool, error) { return strings.HasPrefix(slug, "web-"), nil }

		_, _, err = stack.New(w.Records, w.Chooser, byStacks, byVMs).Admit(ctx, asked("web", stackKind.VMChoice{UUID: "01"}))
		assert.ErrorIs(t, err, slugs.ErrExhausted, "by a stack or by anything else")
	})

	t.Run("one into a docker vm that is not coming up is refused", func(t *testing.T) {
		t.Parallel()

		stopped := vmtest.In(vmtest.Docker("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })

		w := vmtest.New(vmtest.WithVMs(stopped))

		_, invalid, err := strategyOf(w).Admit(ctx, asked("web", stackKind.VMChoice{UUID: "01"}))
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": "vm_not_running"}, invalid)
	})

	t.Run("what the chooser refuses is said where it was asked", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Docker("01", "owner"), vmtest.Running("03", "owner")))

		for choice, want := range map[*stackKind.VMChoice]domain.ValidationErrors{
			{UUID: "03"}:     {"vm.uuid": "not_docker"},
			{UUID: "theirs"}: {"vm.uuid": "not_found"},
			{Resources: &stackKind.Resources{CPUs: 64}}: {"vm.resources.cpus": "too_large"},
		} {
			_, invalid, err := strategyOf(w).Admit(ctx, asked("web", *choice))
			require.NoError(t, err)
			assert.Equal(t, want, invalid)
		}
	})

	for name, tt := range map[string]struct {
		change func(*stackKind.Stack)
		want   domain.ValidationErrors
	}{
		"a stack with no name":              {change: func(s *stackKind.Stack) { s.Metadata.Name = "  " }, want: domain.ValidationErrors{"name": "required_field"}},
		"one with a name nobody could show": {change: func(s *stackKind.Stack) { s.Metadata.Name = strings.Repeat("a", 101) }, want: domain.ValidationErrors{"name": "invalid_name"}},
		"one with no compose file":          {change: func(s *stackKind.Stack) { s.Spec.Compose = " \n" }, want: domain.ValidationErrors{"compose": "required_field"}},
		"one too large":                     {change: func(s *stackKind.Stack) { s.Spec.Compose = strings.Repeat("#", stack.MaxCompose+1) }, want: domain.ValidationErrors{"compose": "too_large"}},
		"one that is not YAML":              {change: func(s *stackKind.Stack) { s.Spec.Compose = "nope: [" }, want: domain.ValidationErrors{"compose": "invalid_value"}},
		"one with no service":               {change: func(s *stackKind.Stack) { s.Spec.Compose = "volumes:\n  data: {}\n" }, want: domain.ValidationErrors{"compose": "invalid_value"}},
		"one naming a vm and asking for a new one": {
			change: func(s *stackKind.Stack) { s.Spec.VM = stackKind.VMChoice{UUID: "01", Name: "builds"} },
			want:   domain.ValidationErrors{"vm": "vm_or_new_vm"},
		},
	} {
		t.Run(name+" is refused, and makes no vm", func(t *testing.T) {
			t.Parallel()

			w := vmtest.New()

			s := asked("web", stackKind.VMChoice{})
			tt.change(&s)

			_, invalid, err := strategyOf(w).Admit(ctx, s)
			require.NoError(t, err)
			assert.Equal(t, tt.want, invalid)

			assert.Zero(t, w.Memory.Len(vmKind.Name))
		})
	}

	t.Run("256 KiB of compose file is not too large", func(t *testing.T) {
		t.Parallel()

		code, ok := stack.ValidateCompose(compose + strings.Repeat("#", stack.MaxCompose-len(compose)))
		assert.True(t, ok, code)
	})
}

func TestStacks_Reconcile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stopped := vmtest.In(vmtest.Docker("stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })

	w := vmtest.New(vmtest.WithVMs(vmtest.Docker("running", "owner"), stopped))
	stacks := strategyOf(w)

	in := func(vmUUID string, state kind.State, expected kind.State) stackKind.Stack {
		return stackKind.Stack{
			Kind:     stackKind.Name,
			Metadata: kind.Metadata{UUID: "stack-uuid", Owners: []kind.Reference{{Kind: "vm", UUID: vmUUID}}},
			Status:   stackKind.Status{Status: kind.Status{State: state, Expected: expected}},
		}
	}

	for name, tt := range map[string]struct {
		stack  stackKind.Stack
		action string
	}{
		"one waiting in a vm that runs is made in it":               {stack: in("running", stackKind.Waiting, stackKind.Running), action: stackKind.ActionCreate},
		"one waiting on a vm that does not run is left to the vm":   {stack: in("stopped", stackKind.Waiting, stackKind.Running)},
		"and so is one whose vm is gone, which goes with it":        {stack: in("gone", stackKind.Waiting, stackKind.Running)},
		"one that lost some of its services is applied again":       {stack: in("running", stackKind.Degraded, stackKind.Running), action: stackKind.ActionApply},
		"one whose last command failed is applied again":            {stack: in("running", kind.Failed, stackKind.Running), action: stackKind.ActionApply},
		"one that stopped while it was expected running is started": {stack: in("running", stackKind.Stopped, stackKind.Running), action: stackKind.ActionStart},
		"one that runs while it was expected stopped is stopped":    {stack: in("running", stackKind.Running, stackKind.Stopped), action: stackKind.ActionStop},
		"as is one that runs some of its services":                  {stack: in("running", stackKind.Degraded, stackKind.Stopped), action: stackKind.ActionStop},
		"and one whose stop failed":                                 {stack: in("running", kind.Failed, stackKind.Stopped), action: stackKind.ActionStop},
		"one doing what it is expected to is asked nothing":         {stack: in("running", stackKind.Running, stackKind.Running)},
		"nor is one stopped as it was asked":                        {stack: in("running", stackKind.Stopped, stackKind.Stopped)},
		"nor one waiting that is to stay stopped":                   {stack: in("running", stackKind.Waiting, stackKind.Stopped)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			intents, err := stacks.Reconcile(ctx, tt.stack)
			require.NoError(t, err)

			if len(tt.action) == 0 {
				assert.Empty(t, intents)

				return
			}

			require.Len(t, intents, 1)
			assert.Equal(t, tt.action, intents[0].Action)
			assert.NotEmpty(t, intents[0].Reason)
			assert.True(t, stackKind.Descriptor().Allows(tt.action, tt.stack.Status.State), "what it asks for is allowed where it asks for it")
		})
	}

	t.Run("a vm that cannot be read is said", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		_, err := strategyOver(w, records.New(unreadable{w.Resources})).Reconcile(ctx, in("running", stackKind.Waiting, stackKind.Running))
		assert.Error(t, err)
	})
}

func TestStacks_Apply(t *testing.T) {
	t.Parallel()

	_, _, err := strategyOf(vmtest.New()).Apply(context.Background(), stackKind.Stack{}, "rename", nil)
	assert.ErrorIs(t, err, kind.ErrUnknownAction, "a stack has nothing done to it in the control plane")
}
