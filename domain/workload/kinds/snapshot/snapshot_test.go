package snapshot_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := snapshotKind.Descriptor()

	t.Run("it keeps every rule a kind keeps", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, kind.Check(d))
	})

	t.Run("its state is its record's, and it outlives the vm it is taken of", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "snapshot", d.Name)
		assert.Equal(t, "snapshots", d.Plural)
		assert.Equal(t, kind.OnControlPlane, d.StateBy)
		assert.Equal(t, "vm", d.Parent)
		assert.Equal(t, kind.ParentRules{Delete: kind.CascadeKeep, Restore: kind.CascadeKeep}, d.OnParent, "a vm deleted or restored leaves its snapshots as they were")
		assert.False(t, d.Endpoints)
	})

	t.Run("its actions are asked under the permissions snapshots always had", func(t *testing.T) {
		t.Parallel()

		permissions := map[string]string{}
		for _, a := range d.Actions {
			if !a.Internal {
				admin, _ := d.Permissions(a.Permission)
				permissions[a.Name] = admin
			}
		}

		assert.Equal(t, map[string]string{
			"rename": "workload.snapshots.update",
			"delete": "workload.snapshots.delete",
			"state":  "workload.snapshots.show",
		}, permissions)

		create, _ := d.Action(snapshotKind.ActionCreate)
		assert.True(t, create.Internal, "taking it is the workload's own to ask, once it is admitted")
	})

	t.Run("only taking it runs on a node", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			runs kind.Executor
			mode kind.Mode
		}{
			snapshotKind.ActionCreate: {runs: kind.OnNode, mode: kind.ModeCommand},
			snapshotKind.ActionRename: {runs: kind.OnControlPlane, mode: kind.ModeCommand},
			snapshotKind.ActionDelete: {runs: kind.OnControlPlane, mode: kind.ModeCommand},
			snapshotKind.ActionState:  {runs: kind.OnControlPlane, mode: kind.ModeQuery},
		} {
			a, found := d.Action(name)
			require.True(t, found, name)
			assert.Equal(t, tt.runs, a.Runs, name)
			assert.Equal(t, tt.mode, a.Mode, name)
		}

		create, _ := d.Action(snapshotKind.ActionCreate)
		assert.Equal(t, snapshotKind.Ready, create.Desires)

		remove, _ := d.Action(snapshotKind.ActionDelete)
		assert.Equal(t, kind.Deleted, remove.Desires)
	})

	t.Run("what each action may be asked in", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			action string
			state  kind.State
			allows bool
		}{
			"taken once it is admitted":                    {action: snapshotKind.ActionCreate, state: snapshotKind.Creating, allows: true},
			"and never again":                              {action: snapshotKind.ActionCreate, state: snapshotKind.Ready},
			"nor once it failed":                           {action: snapshotKind.ActionCreate, state: snapshotKind.Failed},
			"renamed once it is stored":                    {action: snapshotKind.ActionRename, state: snapshotKind.Ready, allows: true},
			"or failed":                                    {action: snapshotKind.ActionRename, state: snapshotKind.Failed, allows: true},
			"not while it is taken":                        {action: snapshotKind.ActionRename, state: snapshotKind.Creating},
			"deleted once it is stored":                    {action: snapshotKind.ActionDelete, state: snapshotKind.Ready, allows: true},
			"or once it failed":                            {action: snapshotKind.ActionDelete, state: snapshotKind.Failed, allows: true},
			"not from under the node taking it":            {action: snapshotKind.ActionDelete, state: snapshotKind.Creating},
			"and asked what it is doing whatever it is":    {action: snapshotKind.ActionState, state: snapshotKind.Creating, allows: true},
			"nothing at all once it is gone":               {action: snapshotKind.ActionState, state: snapshotKind.Deleted},
			"nor anything it does not have, such as start": {action: "start", state: snapshotKind.Ready},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, tt.allows, d.Allows(tt.action, tt.state))
			})
		}
	})

	t.Run("it is renamed with a name", func(t *testing.T) {
		t.Parallel()

		rename, _ := d.Action(snapshotKind.ActionRename)

		value, invalid, err := rename.Payload.Decode([]byte(`{"name":"before the upgrade"}`))
		require.NoError(t, err)
		assert.Empty(t, invalid)
		assert.Equal(t, snapshotKind.RenamePayload{Name: "before the upgrade"}, value)

		_, invalid, err = rename.Payload.Decode(nil)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, invalid)
	})
}

func TestMachine(t *testing.T) {
	t.Parallel()

	m := snapshotKind.Machine()

	assert.Empty(t, m.Validate())
	assert.Equal(t, snapshotKind.Creating, m.Initial, "a snapshot is admitted being taken")

	for name, tt := range map[string]struct {
		from  kind.State
		on    kind.Trigger
		to    kind.State
		taken bool
	}{
		"its archive stored, it is ready":       {from: snapshotKind.Creating, on: kind.OnObserved(snapshotKind.Ready), to: snapshotKind.Ready, taken: true},
		"or failed, when it could not be":       {from: snapshotKind.Creating, on: kind.OnObserved(snapshotKind.Failed), to: snapshotKind.Failed, taken: true},
		"and nothing else ends its being taken": {from: snapshotKind.Creating, on: kind.OnObserved(kind.Missing), to: snapshotKind.Creating},
		"asking for its create moves nothing":   {from: snapshotKind.Creating, on: kind.OnAction(snapshotKind.ActionCreate), to: snapshotKind.Creating},
		"nor does renaming it":                  {from: snapshotKind.Ready, on: kind.OnAction(snapshotKind.ActionRename), to: snapshotKind.Ready},
		"deleted, it is on its way out":         {from: snapshotKind.Ready, on: kind.OnAction(snapshotKind.ActionDelete), to: snapshotKind.Deleting, taken: true},
		"failed ones too":                       {from: snapshotKind.Failed, on: kind.OnAction(snapshotKind.ActionDelete), to: snapshotKind.Deleting, taken: true},
		"and gone once it is":                   {from: snapshotKind.Deleting, on: kind.OnObserved(kind.Deleted), to: kind.Deleted, taken: true},
		"and nothing leaves deleted":            {from: kind.Deleted, on: kind.OnObserved(snapshotKind.Ready), to: kind.Deleted},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, taken := m.Next(tt.from, tt.on)

			assert.Equal(t, tt.to, to)
			assert.Equal(t, tt.taken, taken)
		})
	}

	assert.True(t, m.IsInFlight(snapshotKind.Creating), "it is waited on while it is taken")
	assert.True(t, m.IsTerminal(snapshotKind.Ready), "a ready snapshot rests")
	assert.True(t, m.IsTerminal(snapshotKind.Failed))
}

func TestValidateName(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		name string
		code string
	}{
		"a name":              {name: "before the upgrade"},
		"none":                {name: "   ", code: "required_field"},
		"one a listing cuts":  {name: strings.Repeat("a", snapshotKind.MaxNameLength+1), code: "invalid_name"},
		"one that just fits":  {name: strings.Repeat("a", snapshotKind.MaxNameLength)},
		"spaces do not count": {name: "  " + strings.Repeat("a", snapshotKind.MaxNameLength) + "  "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			code, ok := snapshotKind.ValidateName(tt.name)
			assert.Equal(t, tt.code, code)
			assert.Equal(t, len(tt.code) == 0, ok)
		})
	}
}

func TestObjectKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "snapshots/2f1c9a4e.msb", snapshotKind.ObjectKey("2f1c9a4e"))

	// every snapshot has an object of its own.
	assert.NotEqual(t, snapshotKind.ObjectKey("a"), snapshotKind.ObjectKey("b"))
}

// ready is a ready snapshot of a machine of owner's, its 5 GiB disk written
// by microsandbox, as changes say otherwise.
func ready(changes ...func(s *snapshotKind.Snapshot)) snapshotKind.Snapshot {
	s := snapshotKind.Snapshot{
		Kind:     snapshotKind.Name,
		Metadata: kind.Metadata{UUID: "snapshot-uuid", Name: "before", OwnerUUID: "owner"},
		Spec:     snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: "vm-uuid", Name: "box"}},
		Status: snapshotKind.Status{
			Status: kind.Status{State: snapshotKind.Ready, Expected: snapshotKind.Ready},
			Flavor: vm.KindMachine,
			Image:  "ubuntu:24.04",
			Disk:   5 << 30,
			Engine: "microsandbox/0.7.6",
			Size:   1 << 30,
		},
	}

	for _, change := range changes {
		change(&s)
	}

	return s
}

func TestRestorable(t *testing.T) {
	t.Parallel()

	onto := snapshotKind.Target{OwnerUUID: "owner", Flavor: vm.KindMachine, Disk: 10 << 30, Engine: "microsandbox"}

	for name, tt := range map[string]struct {
		snapshot snapshotKind.Snapshot
		onto     snapshotKind.Target
		want     string
	}{
		"one of its owner's, ready, of its flavor and engine, that fits its disk": {
			snapshot: ready(), onto: onto,
		},
		"one exactly as large as its disk": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Disk = 10 << 30 }), onto: onto,
		},
		"somebody else's is not there": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Metadata.OwnerUUID = "other" }), onto: onto,
			want: snapshotKind.RefusedNotFound,
		},
		"one still being taken": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.State = snapshotKind.Creating }), onto: onto,
			want: snapshotKind.RefusedNotReady,
		},
		"one that failed": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.State = snapshotKind.Failed }), onto: onto,
			want: snapshotKind.RefusedNotReady,
		},
		"one on its way out": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Expected = snapshotKind.Deleted }), onto: onto,
			want: snapshotKind.RefusedNotReady,
		},
		"one of another flavor": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Flavor = vm.KindDocker }), onto: onto,
			want: snapshotKind.RefusedFlavor,
		},
		"one larger than its disk": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Disk = 11 << 30 }), onto: onto,
			want: snapshotKind.RefusedDisk,
		},
		"one written by another engine": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Engine = "firecracker/1.9" }), onto: onto,
			want: snapshotKind.RefusedEngine,
		},
		"another version of the same engine is the engine's to refuse": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Engine = "microsandbox/0.8.0" }), onto: onto,
		},
		"no engine is held against a node that has not said its own": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Engine = "firecracker/1.9" }),
			onto:     snapshotKind.Target{OwnerUUID: "owner", Flavor: vm.KindMachine, Disk: 10 << 30},
		},
		"a vm made from it takes its flavor, and a disk it fits in": {
			snapshot: ready(func(s *snapshotKind.Snapshot) { s.Status.Disk = 40 << 30 }),
			onto:     snapshotKind.Target{OwnerUUID: "owner"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, snapshotKind.Restorable(tt.snapshot, tt.onto))
		})
	}
}

func TestEntity(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	taken := ready(func(s *snapshotKind.Snapshot) {
		s.Metadata.CreatedAt = created
		s.Metadata.Owners = []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}
		s.Status.CompletedAt = created.Add(time.Minute)
	})

	assert.Equal(t, snapshot.Snapshot{
		UUID:        "snapshot-uuid",
		Name:        "before",
		OwnerUUID:   "owner",
		VMUUID:      "vm-uuid",
		VMName:      "box",
		Kind:        vm.KindMachine,
		Image:       "ubuntu:24.04",
		Disk:        5 << 30,
		Engine:      "microsandbox/0.7.6",
		Size:        1 << 30,
		State:       snapshot.Ready,
		CreatedAt:   created,
		CompletedAt: created.Add(time.Minute),
	}, snapshotKind.Entity(taken))

	t.Run("a failed one was done with when it failed", func(t *testing.T) {
		t.Parallel()

		failed := snapshotKind.Entity(ready(func(s *snapshotKind.Snapshot) {
			s.Status.State = snapshotKind.Failed
			s.Status.Reason = "no space left on device"
			s.Status.Since = created.Add(time.Hour)
		}))

		assert.Equal(t, snapshot.Failed, failed.State)
		assert.Equal(t, "no space left on device", failed.Reason)
		assert.Equal(t, created.Add(time.Hour), failed.CompletedAt)
	})

	for name, tt := range map[string]struct {
		state, expected kind.State
		want            snapshot.State
	}{
		"being taken": {state: snapshotKind.Creating, expected: snapshotKind.Ready, want: snapshot.Creating},
		"stored":      {state: snapshotKind.Ready, expected: snapshotKind.Ready, want: snapshot.Ready},
		"failed":      {state: snapshotKind.Failed, expected: snapshotKind.Ready, want: snapshot.Failed},
		"deleted while it was taken, it is going":   {state: snapshotKind.Creating, expected: snapshotKind.Deleted, want: snapshot.Deleting},
		"and so is one whose archive is going":      {state: snapshotKind.Deleting, expected: snapshotKind.Deleted, want: snapshot.Deleting},
		"a state the dashboard has no word for":     {state: "exploded", want: 0},
		"or none at all, which says nothing either": {want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, snapshotKind.StateOf(kind.Status{State: tt.state, Expected: tt.expected}))
		})
	}

	t.Run("one asked for by its vm alone names it", func(t *testing.T) {
		t.Parallel()

		asked := snapshotKind.Snapshot{Spec: snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: "vm-uuid"}}}
		assert.Equal(t, "vm-uuid", snapshotKind.VMOf(asked))
	})
}
