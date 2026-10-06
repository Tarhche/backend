package createVM

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/quota"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

const (
	mib = 1 << 20
	gib = 1 << 30
)

var limits = quota.Limits{
	MinMemory:       128 * mib,
	MinDisk:         1 * gib,
	DockerMinMemory: 512 * mib,
	DockerMinDisk:   4 * gib,
	MaxCPUs:         4,
	MaxMemory:       8 * gib,
	MaxDisk:         50 * gib,
	UserMaxVMs:      3,
	UserCPUs:        8,
	UserMemory:      16 * gib,
	UserDisk:        200 * gib,
	MaxLifetime:     720 * time.Hour,
}

var images = Images{Machine: "ubuntu:24.04", Docker: "docker:29-dind"}

// roomy is a node that spoke a moment ago and has room for anything asked here.
func roomy(name string) node.Node {
	return node.Node{
		Name:            name,
		Role:            node.OrchestratorRole,
		Capacity:        vm.Info{Engine: "microsandbox", Version: "0.7.6", CPUs: 16, Memory: 64 * gib, Disk: 1000 * gib},
		LastHeartbeatAt: time.Now(),
	}
}

type fixture struct {
	vms       *vmsMemory.Repository
	snapshots *snapshotsMemory.Repository
	tasks     *tasksMock.MockTasksRepository
	producer  *messagingMock.Recorder
	useCase   *UseCase
}

func newFixture(t *testing.T, nodes []node.Node, vms []vm.VM, snapshots []snapshot.Snapshot) *fixture {
	t.Helper()

	f := &fixture{
		vms:       vmsMemory.NewRepository(vms...),
		snapshots: snapshotsMemory.NewRepository(snapshots...),
		tasks:     &tasksMock.MockTasksRepository{},
		producer:  &messagingMock.Recorder{},
	}

	f.tasks.On("GetOneBySlug", mock.Anything, mock.Anything).Return(task.Task{}, domain.ErrNotExists).Maybe()

	nodesRepository := nodesMemory.NewRepository(nodes...)

	f.useCase = NewUseCase(
		f.vms,
		f.tasks,
		f.snapshots,
		quota.New(f.vms, limits),
		lifecycle.New(f.vms, &vmtest.Children{}, nodesRepository, placement.New(nodesRepository, f.vms, 4), command.New(f.producer)),
		validator.New(translator.Codes{}),
		images,
	)

	return f
}

func machine() *Request {
	return &Request{
		OwnerUUID: "owner-uuid",
		Name:      "My Box",
		Kind:      vm.KindMachine,
		Resources: Resources{CPUs: 2, Memory: 2 * gib, Disk: 10 * gib},
		Ports:     []port.Port{8080, 22, 8080},
		Network:   Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm is placed where there is room and its node is asked to make it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, nil, nil)

		response, err := f.useCase.Execute(ctx, machine())
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)
		require.NotNil(t, response.VM)

		stored, ok := f.vms.Stored(response.VM.UUID)
		require.True(t, ok)

		assert.Equal(t, "My Box", stored.Name)
		assert.True(t, strings.HasPrefix(stored.Slug, "my-box-"), stored.Slug)
		assert.Equal(t, "ubuntu:24.04", stored.Image, "a machine that names no image boots the default one")
		assert.Equal(t, []port.Port{22, 8080}, stored.Ports, "sorted, each once")
		assert.Equal(t, vm.Scheduled, stored.CurrentState)
		assert.Equal(t, vm.Running, stored.ExpectedState)
		assert.Equal(t, "workload-orchestrator-01", stored.NodeName)
		assert.False(t, stored.UpdatedAt.IsZero())

		var scheduled events.VMScheduled
		require.True(t, f.producer.Last(events.VMScheduledName, &scheduled))

		assert.Equal(t, stored.UUID, scheduled.VMUUID)
		assert.Equal(t, "workload-orchestrator-01", scheduled.NodeName)
		assert.Empty(t, scheduled.SnapshotUUID)
		assert.Equal(t, stored.UUID, scheduled.Spec.ID)
		assert.Equal(t, events.Resources{CPUs: 2, Memory: 2 * gib, Disk: 10 * gib}, scheduled.Spec.Resources)
		assert.Equal(t, vm.AccessDeny, scheduled.Spec.Network.Egress)
		assert.Equal(t, map[string]string{
			vm.LabelOwner:   "owner-uuid",
			vm.LabelVM:      stored.UUID,
			vm.LabelSlug:    stored.Slug,
			vm.LabelPurpose: vm.PurposeVM,
		}, scheduled.Spec.Labels)
	})

	t.Run("a docker vm boots from the docker image and nothing else", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, nil, nil)

		request := machine()
		request.Kind = vm.KindDocker
		request.Resources = Resources{CPUs: 2, Memory: 2 * gib, Disk: 20 * gib}

		response, err := f.useCase.Execute(ctx, request)
		require.NoError(t, err)
		require.NotNil(t, response.VM)
		assert.Equal(t, "docker:29-dind", response.VM.Image)

		request.Image = "alpine:3"
		response, err = f.useCase.Execute(ctx, request)
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"image": "invalid_image"}, response.ValidationErrors)
	})

	t.Run("a network left out is open both ways", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, nil, nil)

		request := machine()
		request.Network = Network{}

		response, err := f.useCase.Execute(ctx, request)
		require.NoError(t, err)
		require.NotNil(t, response.VM)
		assert.Equal(t, "allow", response.VM.Network.Ingress)
		assert.Equal(t, "allow", response.VM.Network.Egress)
	})

	t.Run("a lifetime is counted from now", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, nil, nil)

		request := machine()
		request.LifetimeSeconds = 3600

		before := time.Now()
		response, err := f.useCase.Execute(ctx, request)
		require.NoError(t, err)
		require.NotNil(t, response.VM)

		assert.Equal(t, int64(3600), response.VM.LifetimeSeconds)
		assert.WithinDuration(t, before.Add(time.Hour), response.VM.ExpiresAt, 5*time.Second)
	})

	for name, tt := range map[string]struct {
		change func(*Request)
		owned  []vm.VM
		want   domain.ValidationErrors
	}{
		"what can be told from the request alone": {
			change: func(r *Request) {
				r.OwnerUUID = ""
				r.Name = "  "
				r.Kind = "firecore"
				r.Ports = []port.Port{0}
				r.Network = Network{Ingress: "open"}
				r.LifetimeSeconds = -1
			},
			want: domain.ValidationErrors{
				"owner_uuid":       "required_field",
				"name":             "required_field",
				"kind":             "invalid_kind",
				"ports":            "invalid_port",
				"network.ingress":  "invalid_access",
				"lifetime_seconds": "invalid_lifetime",
			},
		},
		"more ports than a vm may expose": {
			change: func(r *Request) {
				r.Ports = make([]port.Port, MaxPorts+1)
				for i := range r.Ports {
					r.Ports[i] = port.Port(1000 + i)
				}
			},
			want: domain.ValidationErrors{"ports": "too_many_ports"},
		},
		"a port there is not": {
			change: func(r *Request) { r.Ports = []port.Port{65536} },
			want:   domain.ValidationErrors{"ports": "invalid_port"},
		},
		"more than one vm may be given": {
			change: func(r *Request) { r.Resources = Resources{CPUs: 5, Memory: 9 * gib, Disk: 51 * gib} },
			want: domain.ValidationErrors{
				"resources.cpus":   "too_large",
				"resources.memory": "too_large",
				"resources.disk":   "too_large",
			},
		},
		"less than a docker vm needs": {
			change: func(r *Request) {
				r.Kind = vm.KindDocker
				r.Resources = Resources{CPUs: 1, Memory: 256 * mib, Disk: 2 * gib}
			},
			want: domain.ValidationErrors{
				"resources.memory": "too_small",
				"resources.disk":   "too_small",
			},
		},
		"longer than a vm may be kept": {
			change: func(r *Request) { r.LifetimeSeconds = int64((721 * time.Hour) / time.Second) },
			want:   domain.ValidationErrors{"lifetime_seconds": "too_large"},
		},
		"more vms than one person may have": {
			owned: []vm.VM{
				{UUID: "01", OwnerUUID: "owner-uuid", Resources: vm.Resources{CPUs: 1, Memory: gib, Disk: gib}},
				{UUID: "02", OwnerUUID: "owner-uuid", Resources: vm.Resources{CPUs: 1, Memory: gib, Disk: gib}},
				{UUID: "03", OwnerUUID: "owner-uuid", Resources: vm.Resources{CPUs: 1, Memory: gib, Disk: gib}},
			},
			want: domain.ValidationErrors{"vms": "quota_exceeded"},
		},
		"more than one person's vms may be given between them": {
			owned: []vm.VM{
				{UUID: "01", OwnerUUID: "owner-uuid", Resources: vm.Resources{CPUs: 4, Memory: 8 * gib, Disk: 50 * gib}},
				{UUID: "02", OwnerUUID: "owner-uuid", Resources: vm.Resources{CPUs: 4, Memory: 7 * gib, Disk: 50 * gib}},
			},
			want: domain.ValidationErrors{
				"resources.cpus":   "quota_exceeded",
				"resources.memory": "quota_exceeded",
			},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, tt.owned, nil)

			request := machine()
			if tt.change != nil {
				tt.change(request)
			}

			response, err := f.useCase.Execute(ctx, request)
			require.NoError(t, err)

			assert.Equal(t, tt.want, response.ValidationErrors)
			assert.Nil(t, response.VM)
			assert.Empty(t, f.producer.Messages(), "nothing is asked of a node")
			assert.Equal(t, len(tt.owned), f.vms.Len(), "nothing is written down")
		})
	}

	t.Run("a vm no node has room for is failed, not retried", func(t *testing.T) {
		t.Parallel()

		full := roomy("workload-orchestrator-01")
		full.Capacity.Allocated = vm.Resources{Memory: 63 * gib}

		f := newFixture(t, []node.Node{full}, nil, nil)

		response, err := f.useCase.Execute(ctx, machine())
		require.NoError(t, err)
		require.NotNil(t, response.VM)

		stored, ok := f.vms.Stored(response.VM.UUID)
		require.True(t, ok)

		assert.Equal(t, vm.Failed, stored.CurrentState)
		assert.Equal(t, vm.Failed, stored.ExpectedState, "nothing keeps asking for it")
		assert.Equal(t, lifecycle.ReasonNoCapacity, stored.Reason)
		assert.Empty(t, stored.NodeName)
		assert.Empty(t, f.producer.Messages())
	})

	t.Run("a vm made from a snapshot is the snapshot's kind and image, with room for its disk", func(t *testing.T) {
		t.Parallel()

		ready := snapshot.Snapshot{
			UUID:      "snapshot-uuid",
			OwnerUUID: "owner-uuid",
			Kind:      vm.KindDocker,
			Image:     "docker:28-dind",
			Disk:      30 * gib,
			State:     snapshot.Ready,
		}

		f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, nil, []snapshot.Snapshot{ready})

		request := machine()
		request.Kind = ""
		request.Resources = Resources{CPUs: 2, Memory: 2 * gib, Disk: 10 * gib}
		request.SnapshotUUID = "snapshot-uuid"

		response, err := f.useCase.Execute(ctx, request)
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		stored, ok := f.vms.Stored(response.VM.UUID)
		require.True(t, ok)

		assert.Equal(t, vm.KindDocker, stored.Kind)
		assert.Equal(t, "docker:28-dind", stored.Image)
		assert.Equal(t, uint64(30*gib), stored.Resources.Disk)
		assert.Equal(t, "snapshot-uuid", stored.RestoreFrom)

		var scheduled events.VMScheduled
		require.True(t, f.producer.Last(events.VMScheduledName, &scheduled))
		assert.Equal(t, "snapshot-uuid", scheduled.SnapshotUUID)
	})

	for name, tt := range map[string]struct {
		snapshot snapshot.Snapshot
		kind     vm.Kind
		want     domain.ValidationErrors
	}{
		"one that is not stored yet": {
			snapshot: snapshot.Snapshot{UUID: "snapshot-uuid", OwnerUUID: "owner-uuid", Kind: vm.KindMachine, State: snapshot.Creating},
			want:     domain.ValidationErrors{"snapshot_uuid": "snapshot_not_ready"},
		},
		"somebody else's": {
			snapshot: snapshot.Snapshot{UUID: "snapshot-uuid", OwnerUUID: "other", Kind: vm.KindMachine, State: snapshot.Ready},
			want:     domain.ValidationErrors{"snapshot_uuid": "not_found"},
		},
		"one of another kind": {
			snapshot: snapshot.Snapshot{UUID: "snapshot-uuid", OwnerUUID: "owner-uuid", Kind: vm.KindDocker, State: snapshot.Ready},
			kind:     vm.KindMachine,
			want:     domain.ValidationErrors{"kind": "kind_mismatch"},
		},
	} {
		t.Run("not made from a snapshot that is "+name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, nil, []snapshot.Snapshot{tt.snapshot})

			request := machine()
			request.Kind = tt.kind
			request.SnapshotUUID = "snapshot-uuid"

			response, err := f.useCase.Execute(ctx, request)
			require.NoError(t, err)
			assert.Equal(t, tt.want, response.ValidationErrors)
		})
	}

	t.Run("a slug a task already has is not given", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, []node.Node{roomy("workload-orchestrator-01")}, nil, nil)

		taken := ""
		f.tasks.ExpectedCalls = nil
		f.tasks.On("GetOneBySlug", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			taken = args.String(1)
		}).Return(task.Task{}, nil).Once()
		f.tasks.On("GetOneBySlug", mock.Anything, mock.Anything).Return(task.Task{}, domain.ErrNotExists)

		response, err := f.useCase.Execute(ctx, machine())
		require.NoError(t, err)
		require.NotNil(t, response.VM)

		assert.NotEmpty(t, taken)
		assert.NotEqual(t, taken, response.VM.Slug)
	})
}
