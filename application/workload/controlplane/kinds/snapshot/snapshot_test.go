package snapshot_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/resourceResult"
	controlPlaneSnapshots "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/snapshot"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

// bucket stands in for the snapshots bucket: what it holds, what Delete is
// asked, and an archive that is not there is domain.ErrNotExists, as the
// MinIO adapter says it.
type bucket struct {
	lock    sync.Mutex
	objects map[string]bool
	deleted []string

	// fail, when set, is what Delete answers instead.
	fail error
}

var _ snapshotKind.Store = &bucket{}

func newBucket(objectNames ...string) *bucket {
	b := &bucket{objects: make(map[string]bool)}
	for _, name := range objectNames {
		b.objects[name] = true
	}

	return b
}

func (b *bucket) Store(context.Context, string, io.Reader, int64) error { return nil }

func (b *bucket) Read(context.Context, string) (io.ReadSeekCloser, error) {
	return nil, domain.ErrNotExists
}

func (b *bucket) Delete(_ context.Context, objectName string) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.deleted = append(b.deleted, objectName)

	if b.fail != nil {
		return b.fail
	}

	if !b.objects[objectName] {
		return domain.ErrNotExists
	}

	delete(b.objects, objectName)

	return nil
}

func (b *bucket) has(objectName string) bool {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.objects[objectName]
}

// put stores an archive, as a node does once it has taken a snapshot.
func (b *bucket) put(objectName string) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.objects[objectName] = true
}

// strategyOf is the snapshot kind's control-plane strategy over w's VMs and
// resources, archives kept in archives, with room for userMax snapshots each.
func strategyOf(w *vmtest.Workload, archives snapshotKind.Store, userMax uint) *controlPlaneSnapshots.Snapshots {
	return controlPlaneSnapshots.New(controlPlaneSnapshots.Dependencies{
		VMs:       w.Records,
		Nodes:     w.Placement,
		Resources: w.Resources,
		Archives:  archives,
		UserMax:   userMax,
		Logger:    slog.New(slog.DiscardHandler),
	})
}

// asked is a snapshot somebody asks for, as admission is handed one.
func asked(name string, vmUUID string) snapshotKind.Snapshot {
	return snapshotKind.Snapshot{
		Kind:     snapshotKind.Name,
		Metadata: kind.Metadata{Name: name, OwnerUUID: "owner"},
		Spec:     snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: vmUUID}},
	}
}

// kept is a snapshot as the control plane keeps it: of owner's VM 01, a
// machine, ready, with a 10 GiB disk written by microsandbox, as changes say
// otherwise.
func kept(uuid string, changes ...func(s *snapshotKind.Snapshot)) resource.Record {
	s := snapshotKind.Snapshot{
		Kind: snapshotKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "before",
			OwnerUUID: "owner",
			Owners:    []kind.Reference{{Kind: "vm", UUID: "01"}},
			Node:      vmtest.Node,
			CreatedAt: time.Now().Add(-time.Hour),
		},
		Spec: snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: "01", Name: "box"}},
		Status: snapshotKind.Status{
			Status: kind.Status{State: snapshotKind.Ready, Expected: snapshotKind.Ready},
			Flavor: vm.KindMachine,
			Image:  "ubuntu:24.04",
			Disk:   10 * vmtest.GiB,
			Engine: "microsandbox/0.7.6",
			Size:   vmtest.GiB,
		},
	}

	for _, change := range changes {
		change(&s)
	}

	raw, err := kind.Encode(s)
	if err != nil {
		panic(err)
	}

	return resource.Record{Raw: raw}
}

// keeping is a workload whose control plane keeps these snapshots beside
// what opts say.
func keeping(t *testing.T, snapshots []resource.Record, opts ...vmtest.Option) *vmtest.Workload {
	t.Helper()

	w := vmtest.New(opts...)

	for _, r := range snapshots {
		_, err := w.Memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	return w
}

func TestSnapshots_Admit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a snapshot of one of its owner's vms is taken where the vm is, of what the vm is", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Stopped("01", "owner")))

		admitted, invalid, err := strategyOf(w, nil, 10).Admit(ctx, asked(" before the upgrade ", "01"))
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, snapshotKind.Name, admitted.Kind)
		assert.Equal(t, "before the upgrade", admitted.Metadata.Name)
		assert.Equal(t, "owner", admitted.Metadata.OwnerUUID)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "01"}}, admitted.Metadata.Owners, "it is taken of its vm")
		assert.Equal(t, vmtest.Node, admitted.Metadata.Node, "by the node holding the vm")
		assert.Zero(t, admitted.Metadata.Lifetime, "it is kept until it is deleted")
		assert.Equal(t, snapshotKind.VMRef{UUID: "01", Name: "box"}, admitted.Spec.VM, "and says what the vm was called")

		assert.Equal(t, snapshotKind.Creating, admitted.Status.State)
		assert.Equal(t, snapshotKind.Ready, admitted.Status.Expected)
		assert.Equal(t, vm.KindMachine, admitted.Status.Flavor)
		assert.Equal(t, "ubuntu:24.04", admitted.Status.Image)
		assert.Equal(t, uint64(10*vmtest.GiB), admitted.Status.Disk)
	})

	t.Run("and of one that runs", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		_, invalid, err := strategyOf(w, nil, 10).Admit(ctx, asked("now", "01"))
		require.NoError(t, err)
		assert.Empty(t, invalid)
	})

	for name, tt := range map[string]struct {
		vm    vmKind.VM
		nodes []node.Node
		kept  []resource.Record
		asked snapshotKind.Snapshot
		want  domain.ValidationErrors
	}{
		"one with no name": {
			vm:    vmtest.Running("01", "owner"),
			asked: asked("  ", "01"),
			want:  domain.ValidationErrors{"name": "required_field"},
		},
		"one with a name no listing could show": {
			vm:    vmtest.Running("01", "owner"),
			asked: asked(strings.Repeat("a", snapshotKind.MaxNameLength+1), "01"),
			want:  domain.ValidationErrors{"name": "invalid_name"},
		},
		"one of no vm": {
			vm:    vmtest.Running("01", "owner"),
			asked: asked("now", ""),
			want:  domain.ValidationErrors{"vm_uuid": "required_field"},
		},
		"one of a vm on its way somewhere": {
			vm:    vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Starting }),
			asked: asked("now", "01"),
			want:  domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"one of a vm that failed": {
			vm:    vmtest.In(vmtest.Running("01", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Failed }),
			asked: asked("now", "01"),
			want:  domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"one of a vm whose node has gone quiet": {
			vm:    vmtest.Stopped("01", "owner"),
			nodes: []node.Node{vmtest.Gone(vmtest.Node)},
			asked: asked("now", "01"),
			want:  domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"more than one person may keep": {
			vm:    vmtest.Running("01", "owner"),
			kept:  []resource.Record{kept("a"), kept("b")},
			asked: asked("one too many", "01"),
			want:  domain.ValidationErrors{"snapshots": "quota_exceeded"},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithVMs(tt.vm)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			_, invalid, err := strategyOf(keeping(t, tt.kept, opts...), nil, 2).Admit(ctx, tt.asked)
			require.NoError(t, err)
			assert.Equal(t, tt.want, invalid)
		})
	}

	t.Run("somebody else's snapshots are theirs to count", func(t *testing.T) {
		t.Parallel()

		theirs := func(s *snapshotKind.Snapshot) { s.Metadata.OwnerUUID = "other" }
		w := keeping(t, []resource.Record{kept("a", theirs), kept("b", theirs)}, vmtest.WithVMs(vmtest.Running("01", "owner")))

		_, invalid, err := strategyOf(w, nil, 2).Admit(ctx, asked("mine", "01"))
		require.NoError(t, err)
		assert.Empty(t, invalid)
	})

	t.Run("somebody else's vm is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "other")))

		_, _, err := strategyOf(w, nil, 10).Admit(ctx, asked("mine", "01"))
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestSnapshots_Reconcile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	strategy := strategyOf(vmtest.New(), nil, 10)

	for name, tt := range map[string]struct {
		state, expected kind.State
		want            []string
	}{
		"one admitted is taken":                      {state: snapshotKind.Creating, expected: snapshotKind.Ready, want: []string{snapshotKind.ActionCreate}},
		"one stored is asked nothing":                {state: snapshotKind.Ready, expected: snapshotKind.Ready},
		"nor is one that failed, which is not taken": {state: snapshotKind.Failed, expected: snapshotKind.Ready},
		"nor one being taken to be deleted":          {state: snapshotKind.Creating, expected: snapshotKind.Deleted},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s, err := kind.Decode[snapshotKind.Spec, snapshotKind.Status](kept("s1", func(s *snapshotKind.Snapshot) {
				s.Status.State, s.Status.Expected = tt.state, tt.expected
			}).Raw)
			require.NoError(t, err)

			intents, err := strategy.Reconcile(ctx, s)
			require.NoError(t, err)

			var actions []string
			for _, intent := range intents {
				actions = append(actions, intent.Action)
			}

			assert.Equal(t, tt.want, actions)
		})
	}
}

func TestSnapshots_Apply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	snapshotOf := func(t *testing.T, r resource.Record) snapshotKind.Snapshot {
		t.Helper()

		s, err := kind.Decode[snapshotKind.Spec, snapshotKind.Status](r.Raw)
		require.NoError(t, err)

		return s
	}

	t.Run("renamed, it is called what it was given", func(t *testing.T) {
		t.Parallel()

		renamed, invalid, err := strategyOf(vmtest.New(), nil, 10).Apply(ctx, snapshotOf(t, kept("s1")), snapshotKind.ActionRename, snapshotKind.RenamePayload{Name: " after "})
		require.NoError(t, err)
		require.Empty(t, invalid)
		assert.Equal(t, "after", renamed.Metadata.Name)
	})

	t.Run("deleted, its archive is taken away", func(t *testing.T) {
		t.Parallel()

		archives := newBucket(snapshotKind.ObjectKey("s1"))

		deleted, invalid, err := strategyOf(vmtest.New(), archives, 10).Apply(ctx, snapshotOf(t, kept("s1")), snapshotKind.ActionDelete, nil)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Equal(t, kind.Deleted, deleted.Status.State)
		assert.False(t, archives.has(snapshotKind.ObjectKey("s1")))
	})

	t.Run("one whose archive is not there is deleted all the same", func(t *testing.T) {
		t.Parallel()

		archives := newBucket()

		deleted, _, err := strategyOf(vmtest.New(), archives, 10).Apply(ctx, snapshotOf(t, kept("s1")), snapshotKind.ActionDelete, nil)
		require.NoError(t, err)
		assert.Equal(t, kind.Deleted, deleted.Status.State)
		assert.Equal(t, []string{snapshotKind.ObjectKey("s1")}, archives.deleted)
	})

	t.Run("one whose archive cannot be taken away is not deleted", func(t *testing.T) {
		t.Parallel()

		archives := newBucket(snapshotKind.ObjectKey("s1"))
		archives.fail = errors.New("connection refused")

		_, _, err := strategyOf(vmtest.New(), archives, 10).Apply(ctx, snapshotOf(t, kept("s1")), snapshotKind.ActionDelete, nil)
		assert.ErrorContains(t, err, "connection refused")
	})

	t.Run("with no bucket there is nothing to take away", func(t *testing.T) {
		t.Parallel()

		deleted, _, err := strategyOf(vmtest.New(), nil, 10).Apply(ctx, snapshotOf(t, kept("s1")), snapshotKind.ActionDelete, nil)
		require.NoError(t, err)
		assert.Equal(t, kind.Deleted, deleted.Status.State)
	})

	t.Run("nothing else is done to a snapshot in the control plane", func(t *testing.T) {
		t.Parallel()

		_, _, err := strategyOf(vmtest.New(), nil, 10).Apply(ctx, snapshotOf(t, kept("s1")), snapshotKind.ActionCreate, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

func TestSnapshots_Restorable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	onto := snapshotKind.Target{OwnerUUID: "owner", Flavor: vm.KindMachine, Disk: 20 * vmtest.GiB, Engine: "microsandbox"}

	t.Run("one that can be restored is what it took of its vm", func(t *testing.T) {
		t.Parallel()

		taken, refused, err := strategyOf(keeping(t, []resource.Record{kept("s1")}), nil, 10).Restorable(ctx, "s1", onto)
		require.NoError(t, err)
		assert.Empty(t, refused)
		assert.Equal(t, snapshotKind.Taken{Flavor: vm.KindMachine, Image: "ubuntu:24.04", Disk: 10 * vmtest.GiB}, taken)
	})

	for name, tt := range map[string]struct {
		snapshot resource.Record
		uuid     string
		onto     snapshotKind.Target
		want     string
	}{
		"one nobody has": {
			snapshot: kept("s1"), uuid: "another", onto: onto,
			want: snapshotKind.RefusedNotFound,
		},
		"none at all": {
			snapshot: kept("s1"), uuid: " ", onto: onto,
			want: snapshotKind.RefusedNotFound,
		},
		"somebody else's": {
			snapshot: kept("s1", func(s *snapshotKind.Snapshot) { s.Metadata.OwnerUUID = "other" }), uuid: "s1", onto: onto,
			want: snapshotKind.RefusedNotFound,
		},
		"one still being taken": {
			snapshot: kept("s1", func(s *snapshotKind.Snapshot) { s.Status.State = snapshotKind.Creating }), uuid: "s1", onto: onto,
			want: snapshotKind.RefusedNotReady,
		},
		"one of another flavor": {
			snapshot: kept("s1", func(s *snapshotKind.Snapshot) { s.Status.Flavor = vm.KindDocker }), uuid: "s1", onto: onto,
			want: snapshotKind.RefusedFlavor,
		},
		"one larger than its disk": {
			snapshot: kept("s1", func(s *snapshotKind.Snapshot) { s.Status.Disk = 21 * vmtest.GiB }), uuid: "s1", onto: onto,
			want: snapshotKind.RefusedDisk,
		},
		"one written by another engine": {
			snapshot: kept("s1", func(s *snapshotKind.Snapshot) { s.Status.Engine = "firecracker/1.9" }), uuid: "s1", onto: onto,
			want: snapshotKind.RefusedEngine,
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			taken, refused, err := strategyOf(keeping(t, []resource.Record{tt.snapshot}), nil, 10).Restorable(ctx, tt.uuid, tt.onto)
			require.NoError(t, err)
			assert.Equal(t, tt.want, refused)
			assert.Zero(t, taken, "nothing is taken from one that cannot be restored")
		})
	}

	t.Run("records that cannot be read are an error", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()
		strategy := controlPlaneSnapshots.New(controlPlaneSnapshots.Dependencies{VMs: w.Records, Nodes: w.Placement, Resources: unreadable{w.Resources}, UserMax: 10})

		_, _, err := strategy.Restorable(ctx, "s1", onto)
		assert.ErrorContains(t, err, "the database is gone")
	})
}

// unreadable is a store of resources that cannot be read.
type unreadable struct {
	resource.Repository
}

func (unreadable) GetOneByOwner(context.Context, string, string, string) (resource.Record, error) {
	return resource.Record{}, errors.New("the database is gone")
}

// admitRequest asks for a snapshot of vmUUID named name, for owner, as the
// resource API does.
func admitRequest(name string, vmUUID string) *admitResource.Request {
	return &admitResource.Request{
		Kind:      snapshotKind.Name,
		OwnerUUID: "owner",
		Manifest: kind.Raw{
			Kind:     snapshotKind.Name,
			Metadata: kind.Metadata{Name: name},
			Spec:     []byte(`{"vm":{"uuid":"` + vmUUID + `"}}`),
		},
	}
}

// TestSnapshots_bound holds the strategy, bound and registered as the
// control plane registers it, to what the generic plumbing makes of it: a
// snapshot admitted is sent its create, addressed to its vm's node, and what
// runs in the control plane is carried out in place.
func TestSnapshots_bound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	archives := newBucket(snapshotKind.ObjectKey("s1"))
	w := keeping(t, []resource.Record{kept("s1")}, vmtest.WithVMs(vmtest.Stopped("01", "owner")))

	require.NoError(t, w.Registry.Register(kind.BindControlPlane[snapshotKind.Spec, snapshotKind.Status](snapshotKind.Descriptor(), strategyOf(w, archives, 10))))

	t.Run("one admitted is asked of its vm's node to be taken", func(t *testing.T) {
		admitted, err := w.Admit.Execute(ctx, admitRequest("before", "01"))
		require.NoError(t, err)
		require.Empty(t, admitted.ValidationErrors)
		require.NotNil(t, admitted.Command)

		assert.Equal(t, snapshotKind.ActionCreate, admitted.Command.Action)
		assert.Equal(t, vmtest.Node, admitted.Command.Node)

		commands, err := messagingMock.Produced[kind.Command](w.Producer, kind.CommandName)
		require.NoError(t, err)
		require.Len(t, commands, 1)
		assert.Equal(t, admitted.Resource.Metadata.UUID, commands[0].UUID)
	})

	t.Run("renamed and deleted in place", func(t *testing.T) {
		acting := actOnResource.NewUseCase(w.Registry, w.Resources, w.Dispatcher, logger)

		renamed, err := acting.Execute(ctx, &actOnResource.Request{Kind: snapshotKind.Name, OwnerUUID: "owner", UUID: "s1", Action: snapshotKind.ActionRename, Payload: []byte(`{"name":"after"}`)})
		require.NoError(t, err)
		require.Empty(t, renamed.ValidationErrors)
		assert.Equal(t, "after", renamed.Resource.Metadata.Name)
		assert.Nil(t, renamed.Command, "no node is asked anything")

		deleted, err := deleteResource.NewUseCase(w.Registry, w.Resources, w.Dispatcher).Execute(ctx, &deleteResource.Request{Kind: snapshotKind.Name, OwnerUUID: "owner", UUID: "s1"})
		require.NoError(t, err)
		assert.True(t, deleted.Gone)
		assert.False(t, archives.has(snapshotKind.ObjectKey("s1")))

		_, err = w.Resources.GetOne(ctx, snapshotKind.Name, "s1")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("one deleted while it is taken goes once its node has taken it, archive and all", func(t *testing.T) {
		admitted, err := w.Admit.Execute(ctx, admitRequest("doomed", "01"))
		require.NoError(t, err)
		require.NotNil(t, admitted.Command)

		uuid := admitted.Resource.Metadata.UUID

		deleting, err := deleteResource.NewUseCase(w.Registry, w.Resources, w.Dispatcher).Execute(ctx, &deleteResource.Request{Kind: snapshotKind.Name, OwnerUUID: "owner", UUID: uuid})
		require.NoError(t, err)
		assert.False(t, deleting.Gone, "not from under the node taking it")

		waiting, err := w.Resources.GetOne(ctx, snapshotKind.Name, uuid)
		require.NoError(t, err)

		common, err := waiting.Common()
		require.NoError(t, err)
		assert.Equal(t, snapshotKind.Creating, common.State)
		assert.Equal(t, kind.Deleted, common.Expected, "it is to go once it can")
		require.NotNil(t, waiting.Pending)
		assert.Equal(t, snapshotKind.ActionCreate, waiting.Pending.Action)

		// its node takes it, stores its archive, and says so.
		archives.put(snapshotKind.ObjectKey(uuid))

		status, err := json.Marshal(snapshotKind.Status{Status: kind.Status{State: snapshotKind.Ready}, Engine: "memory/1", Size: 7})
		require.NoError(t, err)

		answer, err := json.Marshal(kind.Result{ID: admitted.Command.ID, Kind: snapshotKind.Name, UUID: uuid, Action: snapshotKind.ActionCreate, Node: vmtest.Node, OK: true, Status: status})
		require.NoError(t, err)

		require.NoError(t, resourceResult.NewResult(w.Registry, w.Resources, nil, logger, nil).Handle(ctx, answer))

		require.NoError(t, reconcile.NewUseCase(w.Registry, w.Resources, w.Nodes, w.Dispatcher, logger, reconcile.DefaultConfig()).Execute(ctx))

		_, err = w.Resources.GetOne(ctx, snapshotKind.Name, uuid)
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.False(t, archives.has(snapshotKind.ObjectKey(uuid)), "nothing its node stored is left behind")
	})
}
