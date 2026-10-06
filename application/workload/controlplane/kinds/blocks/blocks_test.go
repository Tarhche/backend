package blocks_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// seen is a container as a node reports it in vm-1: the resource uuid names,
// when the platform made it, or nobody's when uuid is empty.
func seen(t *testing.T, uuid string, docker containerKind.Docker, owners ...kind.Reference) kind.Observation {
	t.Helper()

	status, err := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.StateOf(docker.State, docker.RestartPolicy)}, Docker: &docker})
	require.NoError(t, err)

	return kind.Observation{
		Kind:   containerKind.Name,
		UUID:   uuid,
		Owners: append([]kind.Reference{{Kind: "vm", UUID: "vm-1"}}, owners...),
		Status: status,
	}
}

func report(read []string, unseen []string, instances ...kind.Observation) kind.Report[json.RawMessage] {
	return kind.Report[json.RawMessage]{Instances: instances, Read: read, Unseen: unseen}
}

func TestBlocks_Witnessed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	at := time.Now()

	t.Run("what nobody keeps a record of is shown as a stack's or nobody's, its vm's owner's", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("kept", "vm-1", containerKind.Running, containerKind.Running))

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil,
			seen(t, "kept", containerKind.Docker{ID: "c-kept", Name: "web", State: "running"}),
			seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "running", CreatedAt: at.Add(-time.Hour)}),
			seen(t, "", containerKind.Docker{ID: "c-shop", Name: "shop-web-1", State: "running", Labels: map[string]string{docker.LabelComposeProject: "shop"}, CreatedAt: at}, kind.Reference{Kind: "stack", UUID: "stack-uuid"}),
		), at))

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 2, "what a record speaks for is the record's")

		shop, db := extras[0], extras[1]

		assert.Equal(t, "shop-web-1", shop.Metadata.Name, "newest first")
		assert.Equal(t, blockKinds.ManagedByStack, shop.Metadata.Labels[blockKinds.LabelManagedBy])
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}, {Kind: "stack", UUID: "stack-uuid"}}, shop.Metadata.Owners)

		assert.Equal(t, blockKinds.ManagedByNobody, db.Metadata.Labels[blockKinds.LabelManagedBy], "made from the vm's terminal: unmanaged")
		assert.Equal(t, "owner", db.Metadata.OwnerUUID, "its vm's owner's")
		assert.Equal(t, vmtest.Node, db.Metadata.Node)
		assert.Equal(t, blockKinds.Derived(containerKind.Name, "vm-1", "c-db"), db.Metadata.UUID, "known by its docker id")

		one, err := w.Containers.One(ctx, db.Metadata.UUID)
		require.NoError(t, err)
		assert.Equal(t, db, one)

		_, err = w.Containers.One(ctx, "kept")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a record is not an extra")
	})

	t.Run("what a vm that could not be read held is shown as it was last seen, and a vm not looked into is let go of", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))

		inVM2 := seen(t, "", containerKind.Docker{ID: "c-2", Name: "cache", State: "running"})
		inVM2.Owners = []kind.Reference{{Kind: "vm", UUID: "vm-2"}}

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1", "vm-2"}, nil,
			seen(t, "", containerKind.Docker{ID: "c-1", Name: "db", State: "running"}),
			inVM2,
		), at))

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report(nil, []string{"vm-1"}), at.Add(time.Second)))

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 1)
		assert.Equal(t, "db", extras[0].Metadata.Name, "vm-1 is shown as it was last seen; vm-2, not looked into, is let go of")

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil), at.Add(2*time.Second)))

		extras, err = w.Containers.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, extras, "and once vm-1 is read again, what is gone from it is gone")
	})

	t.Run("what is labelled as the platform's with no record is nobody's, rather than taken in", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		labels := labelled(t, "deleted-a-moment-ago")

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil,
			seen(t, "deleted-a-moment-ago", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labels}),
		), at))

		_, kept := w.Kept(containerKind.Name, "deleted-a-moment-ago")
		assert.False(t, kept, "what a node saw before it removed it does not come back")

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 1)
		assert.Equal(t, "deleted-a-moment-ago", extras[0].Metadata.UUID)
	})

	t.Run("but in the first look into a vm whose disk was just restored, it is taken in again, as it was asked for", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Minute) })
		w := blockstest.New(vmtest.WithVMs(restored))

		labels := labelled(t, "from-the-snapshot")

		look := report([]string{"vm-1"}, nil, seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labels}))

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, look, at))

		adopted, kept := w.Kept(containerKind.Name, "from-the-snapshot")
		require.True(t, kept)

		c, err := kind.Decode[containerKind.Spec, containerKind.Status](adopted.Raw)
		require.NoError(t, err)

		assert.Equal(t, "owner", c.Metadata.OwnerUUID)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, c.Metadata.Owners)
		assert.Equal(t, vmtest.Node, c.Metadata.Node)
		assert.Equal(t, "nginx:1.27", c.Spec.Image, "as its label says it was asked for")
		assert.Equal(t, containerKind.Running, c.Status.State)
		assert.Equal(t, containerKind.Running, c.Status.Expected)

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, extras, "and it is not shown as nobody's")

		require.NoError(t, w.Resources.Delete(ctx, containerKind.Name, "from-the-snapshot"))
		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, look, at.Add(time.Second)))

		_, kept = w.Kept(containerKind.Name, "from-the-snapshot")
		assert.False(t, kept, "only the first look after the restore takes anything in")
	})

	t.Run("nor long after the restore", func(t *testing.T) {
		t.Parallel()

		restored := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.RestoredAt = at.Add(-time.Hour) })
		w := blockstest.New(vmtest.WithVMs(restored))

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil,
			seen(t, "from-the-snapshot", containerKind.Docker{ID: "c1", Name: "web", State: "running", Labels: labelled(t, "from-the-snapshot")}),
		), at))

		_, kept := w.Kept(containerKind.Name, "from-the-snapshot")
		assert.False(t, kept)
	})

	t.Run("what a vm nobody keeps a record of holds is not shown", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New()

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil,
			seen(t, "", containerKind.Docker{ID: "c1", Name: "db", State: "running"}),
		), at))

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, extras)
	})
}

// labelled are the labels a container the platform made as uuid has, its
// spec among them.
func labelled(t *testing.T, uuid string) map[string]string {
	t.Helper()

	spec, err := containerKind.SpecLabel(containerKind.Spec{Name: "web", Image: "nginx:1.27"})
	require.NoError(t, err)

	labels := blockKinds.Labels(containerKind.Name, uuid)
	labels[containerKind.LabelSpec] = spec

	return labels
}

func TestBlocks_Act(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	witnessed := func(t *testing.T, w *blockstest.Workload, docker containerKind.Docker) kind.Raw {
		t.Helper()

		require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil, seen(t, "", docker)), time.Now()))

		extras, err := w.Containers.All(ctx)
		require.NoError(t, err)
		require.Len(t, extras, 1)

		return extras[0]
	}

	t.Run("it is asked of its node at once, and is what the command left it as", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := witnessed(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		w.Answer(func(query kind.Query) kind.Result {
			status, _ := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Completed}, Docker: &containerKind.Docker{ID: "c-db", Name: "db", State: "exited"}})

			return kind.Result{OK: true, Kind: query.Kind, UUID: query.UUID, Action: query.Action, Status: status}
		}, nil)

		after, gone, refused, err := w.Containers.Act(ctx, db, containerKind.ActionStop, nil)
		require.NoError(t, err)
		require.Empty(t, refused)
		assert.False(t, gone)

		assert.Equal(t, "exited", blocks.ObservedOf(after.Status).Docker.State)

		asked := w.Requester.Asked()
		require.Len(t, asked, 1)
		assert.Equal(t, vmtest.Node, asked[0].NodeName)
		assert.Equal(t, kind.Op(containerKind.Name, containerKind.ActionStop), asked[0].Request.Op)

		shown, err := w.Containers.One(ctx, db.Metadata.UUID)
		require.NoError(t, err)
		assert.Equal(t, after, shown, "and is shown so until its node says otherwise")
	})

	t.Run("removed, it is gone", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := witnessed(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "exited"})

		w.Answer(func(query kind.Query) kind.Result { return kind.Result{OK: true} }, nil)

		_, gone, _, err := w.Containers.Act(ctx, db, containerKind.ActionDelete, nil)
		require.NoError(t, err)
		assert.True(t, gone)

		_, err = w.Containers.One(ctx, db.Metadata.UUID)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("one that runs is not removed but by force, and its node is not asked", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := witnessed(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		_, _, _, err := w.Containers.Act(ctx, db, containerKind.ActionDelete, nil)

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, noderequest.CodeInvalid, refused.Code)
		assert.Empty(t, w.Requester.Asked())
	})

	t.Run("what its node refused is the node's refusal, in the codes every side knows", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := witnessed(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "exited"})

		w.Answer(func(query kind.Query) kind.Result {
			status, _ := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Failed}, Failure: &noderequest.Error{Code: noderequest.CodeInvalid, Message: "port is already allocated"}})

			return kind.Result{OK: false, Reason: "port is already allocated", Status: status}
		}, nil)

		_, _, _, err := w.Containers.Act(ctx, db, containerKind.ActionStart, nil)

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, &noderequest.Error{Code: noderequest.CodeInvalid, Message: "port is already allocated"}, refused)
	})

	t.Run("what its state does not allow is refused as a record's is", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := witnessed(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		_, _, refused, err := w.Containers.Act(ctx, db, containerKind.ActionStart, nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"action": "invalid_state_transition"}, refused)
	})

	t.Run("and the workload's own commands are nobody's to ask", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		db := witnessed(t, w, containerKind.Docker{ID: "c-db", Name: "db", State: "running"})

		_, _, _, err := w.Containers.Act(ctx, db, containerKind.ActionCreate, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

func TestBlocks_Query(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

	require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil,
		seen(t, "", containerKind.Docker{ID: "c-db", Name: "db", State: "running"}),
		seen(t, "", containerKind.Docker{ID: "c-old", Name: "old", State: "exited"}),
	), time.Now()))

	w.Answer(nil, json.RawMessage(`{"lines":[{"line":"ready"}]}`))

	db, err := w.Containers.One(ctx, blockKinds.Derived(containerKind.Name, "vm-1", "c-db"))
	require.NoError(t, err)

	answer, refused, err := w.Containers.Query(ctx, db, containerKind.ActionLogs, []byte(`{"tail": 1}`))
	require.NoError(t, err)
	require.Empty(t, refused)
	assert.JSONEq(t, `{"lines":[{"line":"ready"}]}`, string(answer))

	asked := w.Requester.Asked()
	require.Len(t, asked, 1)

	query, err := kind.QueryOf(asked[0].Request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tail":1}`, string(query.Payload))
	assert.Equal(t, db.Metadata.UUID, query.Resource.Metadata.UUID, "asked about what it is, as it was seen")

	old, err := w.Containers.One(ctx, blockKinds.Derived(containerKind.Name, "vm-1", "c-old"))
	require.NoError(t, err)

	_, _, err = w.Containers.Query(ctx, old, containerKind.ActionStats, nil)

	var notRunning *noderequest.Error
	require.ErrorAs(t, err, &notRunning)
	assert.Equal(t, noderequest.CodeNotRunning, notRunning.Code, "what one that is not running uses is not sampled")
}

func TestBlocks_Resolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))
	w.Keep(blockstest.AContainer("kept", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Status.Docker.ID = "0123456789abcdef"
	}))
	w.Keep(blockstest.AContainer("pending", "vm-1", containerKind.Pending, containerKind.Running, func(c *containerKind.Container) {
		c.Spec.Name = "api"
		c.Status.Docker = nil
	}))
	w.Keep(blockstest.AContainer("elsewhere", "vm-2", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Spec.Name = "cache"
		c.Status.Docker.ID = "fedcba"
		c.Status.Docker.Name = "cache"
	}))

	require.NoError(t, w.Containers.Witnessed(ctx, vmtest.Node, report([]string{"vm-1"}, nil,
		seen(t, "", containerKind.Docker{ID: "0123ffff", Name: "db", State: "running"}),
	), time.Now()))

	unmanaged := blockKinds.Derived(containerKind.Name, "vm-1", "0123ffff")
	vm1 := kind.Reference{Kind: "vm", UUID: "vm-1"}

	for name, tt := range map[string]struct {
		name string
		want string
	}{
		"a record by its uuid": {name: "kept", want: "kept"},
		"by its docker id":     {name: "0123456789abcdef", want: "kept"},
		"by its name":          {name: "web", want: "kept"},
		"by the name it was asked for with, not made yet": {name: "api", want: "pending"},
		"by the one docker id beginning so":               {name: "012345", want: "kept"},
		"what nobody keeps a record of, by its name":      {name: "db", want: unmanaged},
		"and by its docker id":                            {name: "0123ffff", want: unmanaged},
		"and by its uuid":                                 {name: unmanaged, want: unmanaged},
		"but not by a beginning several ids have":         {name: "0123"},
		"nor by one too short to name any":                {name: "01"},
		"nor what lives in another vm":                    {name: "cache"},
		"nor nothing":                                     {name: " "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			uuid, err := w.Containers.Resolve(ctx, vm1, tt.name)

			if len(tt.want) == 0 {
				assert.ErrorIs(t, err, domain.ErrNotExists)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, uuid)
		})
	}
}

func TestBlocks_Into(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stopped := vmtest.In(vmtest.Docker("vm-stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })
	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), stopped, vmtest.Docker("theirs", "somebody-else")))

	v, refused, err := w.Images.Into(ctx, "owner", "vm-1")
	require.NoError(t, err)
	require.Empty(t, refused)
	assert.Equal(t, "vm-1", v.Metadata.UUID)

	for parent, want := range map[string]domain.ValidationErrors{
		"":           {"vm": "required_field"},
		"vm-stopped": {"vm": "not_running"},
		"theirs":     {"vm.uuid": "not_found"},
	} {
		_, refused, err := w.Images.Into(ctx, "owner", parent)
		require.NoError(t, err)
		assert.Equal(t, want, refused, parent)
	}
}

func TestBlocks_Present(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stopped := vmtest.In(vmtest.Docker("vm-stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })
	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), stopped))

	create := []kind.Intent{{Action: blockKinds.ActionCreate, Reason: "it is not in its vm, which runs"}}

	for name, tt := range map[string]struct {
		state    kind.State
		expected kind.State
		vm       string
		want     []kind.Intent
	}{
		"one not made yet is made":          {state: networkKind.Pending, expected: networkKind.Present, vm: "vm-1", want: create},
		"and one its vm lost":               {state: networkKind.Missing, expected: networkKind.Present, vm: "vm-1", want: create},
		"but not while its vm does not run": {state: networkKind.Missing, expected: networkKind.Present, vm: "vm-stopped"},
		"one that is there is left":         {state: networkKind.Present, expected: networkKind.Present, vm: "vm-1"},
		"one waiting on its vm waits":       {state: networkKind.Waiting, expected: networkKind.Present, vm: "vm-1"},
		"one to be deleted is the loop's":   {state: networkKind.Missing, expected: kind.Deleted, vm: "vm-1"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			intents, err := w.Networks.Present(ctx, tt.state, tt.expected, tt.vm)
			require.NoError(t, err)
			assert.Equal(t, tt.want, intents)
		})
	}
}

func TestRefusal(t *testing.T) {
	t.Parallel()

	status, err := json.Marshal(imageKind.Status{Failure: &noderequest.Error{Code: noderequest.CodeDockerUnavailable, Message: "its dockerd did not answer"}})
	require.NoError(t, err)

	assert.Equal(t, &noderequest.Error{Code: noderequest.CodeDockerUnavailable, Message: "its dockerd did not answer"}, blocks.Refusal(status, "anything"))
	assert.Equal(t, &noderequest.Error{Code: noderequest.CodeInternal, Message: "it broke"}, blocks.Refusal(nil, "it broke"), "and when it says nothing, its reason")
}
