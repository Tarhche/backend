package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	actonresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	admitresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	deleteresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	getkinds "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	getresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	getresources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	queryresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/recordResult"
	controlplanestacks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/stack"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/runs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
	kindsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/kinds"
)

// node answers what the control plane asks of it the way a node does: every
// VM's log is one line.
func node() *messagingMock.Requester {
	return &messagingMock.Requester{Answer: func(_ context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
		var result any

		switch request.Op {
		case kind.Op(vmKind.Name, vmKind.ActionLogs):
			result = vmKind.Logs{Lines: []vmKind.LogLine{{At: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), Source: vm.LogSourceKernel, Line: "booted"}}}
		default:
			return noderequest.Reply{OK: true}, nil
		}

		encoded, _ := json.Marshal(result)

		return noderequest.Reply{OK: true, Result: encoded}, nil
	}}
}

// containersNode is the node holding the building blocks: what it is sent
// of a container, a network or a volume it carries out at once, and says so,
// as a node does. Anything else it is sent goes to others, which keeps it,
// unanswered.
type containersNode struct {
	results domain.MessageHandler
	others  domain.Producer
}

func (n *containersNode) Produce(ctx context.Context, subject string, payload []byte) error {
	var command kind.ActOnResource
	if subject != kind.ActOnResourceName || json.Unmarshal(payload, &command) != nil {
		return n.others.Produce(ctx, subject, payload)
	}

	result := kind.ResourceActedOn{ID: command.ID, Kind: command.Kind, UUID: command.UUID, Action: command.Action, Node: command.Node, OK: true, At: time.Now()}

	var (
		status any
		err    error
	)

	switch command.Kind {
	case containerKind.Name:
		c, decoded := kind.Decode[containerKind.Spec, containerKind.Status](command.Resource)
		status, err = containerKind.Status{
			Status:  kind.Status{State: containerKind.Running},
			Docker:  &containerKind.Docker{ID: "c-" + c.Spec.Name, Name: c.Spec.Name, Image: c.Spec.Image, State: "running", Status: "Up Less than a second"},
			Failure: &noderequest.Error{},
		}, decoded
	case networkKind.Name:
		n, decoded := kind.Decode[networkKind.Spec, networkKind.Status](command.Resource)
		status, err = networkKind.Status{
			Status: kind.Status{State: networkKind.Present},
			Docker: &networkKind.Docker{ID: "n-" + n.Spec.Name, Name: n.Spec.Name, Driver: "bridge", Scope: "local", Containers: []string{}},
		}, decoded
	case volumeKind.Name:
		v, decoded := kind.Decode[volumeKind.Spec, volumeKind.Status](command.Resource)
		status, err = volumeKind.Status{
			Status: kind.Status{State: volumeKind.Present},
			Docker: &volumeKind.Docker{Name: v.Spec.Name, Driver: "local"},
		}, decoded
	default:
		return n.others.Produce(ctx, subject, payload)
	}

	if err != nil {
		return err
	}

	if command.Action != "delete" {
		result.Status, _ = json.Marshal(status)
	}

	answer, _ := json.Marshal(result)

	go func() { _ = n.results.Handle(context.Background(), answer) }()

	return nil
}

// controlPlane is the control plane's API, wired the way its provider wires
// it, over memory repositories and a node that answers: VMs, their snapshots,
// stacks, tasks and the building blocks of Docker VMs are kinds, served by the
// resource API every kind is.
func controlPlane(t *testing.T, w *blockstest.Workload) *client.Client {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)
	requester := node()

	// snapshots are taken of VMs, and stacks live in Docker VMs, registered
	// after them as the control plane registers them.
	require.NoError(t, w.Registry.Register(kind.BindControlPlane[snapshotKind.Spec, snapshotKind.Status](snapshotKind.Descriptor(), w.Snapshots)))
	require.NoError(t, w.Registry.Register(kind.BindControlPlane[stackKind.Spec, stackKind.Status](
		stackKind.Descriptor(),
		controlplanestacks.New(w.Records, w.Chooser, slugs.By(func(ctx context.Context, slug string) (resource.Record, error) {
			return w.Memory.GetOneBySlug(ctx, stackKind.Name, slug)
		})),
	)))

	// what is sent to a container's node comes back from it, and what is
	// sent to any other node is kept unanswered.
	waiting := waiters.New()
	containers := &containersNode{results: recordResult.NewResourceActedOnHandler(w.Registry, w.Resources, waiting, logger, nil), others: w.Producer}
	dispatcher := dispatch.New(w.Resources, containers, waiting, nil, dispatch.PollEvery(10*time.Millisecond))

	mux := http.NewServeMux()

	require.NoError(t, kindsAPI.Route(mux, w.Registry.Descriptors(), kindsAPI.UseCases{
		Admit:  admitresource.NewUseCase(w.Registry, w.Resources, dispatcher, logger),
		Act:    actonresource.NewUseCase(w.Registry, w.Resources, dispatcher, logger),
		Delete: deleteresource.NewUseCase(w.Registry, w.Resources, dispatcher),
		Get:    getresource.NewUseCase(w.Registry, w.Resources),
		List:   getresources.NewUseCase(w.Registry, w.Resources),
		Query:  queryresource.NewUseCase(w.Registry, w.Resources, requester, nil, nil),
		Kinds:  getkinds.NewUseCase(w.Registry),
	}))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL, vmtest.Images.Docker)
	require.NoError(t, err)

	return c
}

// witnessed has the containers' strategy hear that the node holds, in the
// Docker VM vmUUID names, what docker says of each container, a heartbeat for
// each.
func witnessed(t *testing.T, w *blockstest.Workload, vmUUID string, containers ...containerKind.Docker) {
	t.Helper()

	at := time.Now()

	for _, c := range containers {
		owners := []kind.Reference{{Kind: "vm", UUID: vmUUID}}
		if stack := c.Labels[stackKind.LabelStack]; len(stack) > 0 {
			owners = append(owners, kind.Reference{Kind: stackKind.Name, UUID: stack})
		}

		status, err := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.StateOf(c.State, "")}, Docker: &c})
		require.NoError(t, err)

		require.NoError(t, w.Containers.Witnessed(t.Context(), vmtest.Node, kind.Observation{Kind: containerKind.Name, Owners: owners, Status: status}, false, at))
	}
}

func TestContract_VMs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := blockstest.New(vmtest.WithVMs(vmtest.Running("theirs", "other")))
	c := controlPlane(t, w)

	created, err := c.CreateVM(ctx, "owner", workloadControlPlane.VMRequest{
		Name:      "box",
		Kind:      vm.KindMachine,
		Resources: vm.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB},
		Ports:     []port.Port{22},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		Lifetime:  time.Hour,
	})
	require.NoError(t, err)

	assert.Equal(t, "box", created.Name)
	assert.Equal(t, "owner", created.OwnerUUID)
	assert.Equal(t, vm.KindMachine, created.Kind)
	assert.Equal(t, vm.Scheduled, created.CurrentState)
	assert.Equal(t, vm.Running, created.ExpectedState)
	assert.Equal(t, vmtest.Node, created.NodeName)
	assert.Equal(t, time.Hour, created.Lifetime)
	assert.Equal(t, []port.Port{22}, created.Ports)
	assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, created.Network)

	read, err := c.VM(ctx, "owner", created.UUID)
	require.NoError(t, err)
	assert.Equal(t, created.Slug, read.Slug)

	_, err = c.VM(ctx, "owner", "theirs")
	assert.ErrorIs(t, err, domain.ErrNotExists)

	page, err := c.VMs(ctx, "owner", "", 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, uint(1), page.TotalPages)

	name := "renamed"
	updated, err := c.UpdateVM(ctx, "owner", created.UUID, workloadControlPlane.VMUpdate{Name: &name})
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Name)

	// a docker vm is asked for by the docker image, and is one as its image
	// says, which is how the control plane lists one too.
	docker, err := c.CreateVM(ctx, "owner", workloadControlPlane.VMRequest{
		Name:      "builds",
		Kind:      vm.KindDocker,
		Resources: vm.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB},
	})
	require.NoError(t, err)
	assert.Equal(t, vm.KindDocker, docker.Kind)
	assert.Equal(t, vmtest.Images.Docker, docker.Image)

	for which, want := range map[vm.Kind]string{vm.KindDocker: docker.UUID, vm.KindMachine: created.UUID} {
		listed, err := c.VMs(ctx, "owner", which, 1)
		require.NoError(t, err)
		require.Len(t, listed.Items, 1, which)
		assert.Equal(t, want, listed.Items[0].UUID, which)
		assert.Equal(t, which, listed.Items[0].Kind)
	}

	_, err = c.CreateVM(ctx, "owner", workloadControlPlane.VMRequest{
		Name:      "huge",
		Kind:      vm.KindMachine,
		Resources: vm.Resources{CPUs: 64, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB},
	})

	var refused *client.ValidationError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"resources.cpus": "too_large"}, refused.ValidationErrors)

	require.NoError(t, c.StopVM(ctx, "owner", created.UUID))
	require.NoError(t, c.StartVM(ctx, "owner", created.UUID))
	require.NoError(t, c.RestartVM(ctx, "owner", created.UUID))

	// its node has not made it yet, so there is no log to read: it is not
	// running, rather than not there.
	_, err = c.VMLogs(ctx, "owner", created.UUID, vm.LogOptions{Tail: 10})
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"vm": "not_running"}, refused.ValidationErrors)

	// its node made it, and has a log of it from then on.
	w.Change(created.UUID, func(v *vmKind.VM) {
		v.Status.State = vmKind.Running
		v.Status.ObservedAt = time.Now()
	})

	lines, err := c.VMLogs(ctx, "owner", created.UUID, vm.LogOptions{Tail: 10})
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "booted", lines[0].Line)

	require.NoError(t, c.DeleteVM(ctx, "owner", created.UUID))

	deleting, err := c.VM(ctx, "", created.UUID)
	require.NoError(t, err)
	assert.Equal(t, vm.Deleting, deleting.CurrentState)
}

// TestContract_Runs reads the code runner's runs back as the VMs they run in,
// among anybody's: what manages them travels with them, and so does what a run
// can be asked and what it is refused.
func TestContract_Runs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	run := vmtest.Run("run")
	run.Status.Run.Output = "hello\nbye\n"

	w := blockstest.New(vmtest.WithVMs(vmtest.Running("theirs", "other")), vmtest.WithTasks(run))
	c := controlPlane(t, w)

	page, err := c.VMs(ctx, "", "", 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, "run", page.Items[0].UUID, "the newest first")
	assert.Equal(t, vm.ManagedByCodeRunner, page.Items[0].ManagedBy)
	assert.Equal(t, task.GuestOwnerUUID, page.Items[0].OwnerUUID)
	assert.Equal(t, vm.Running, page.Items[0].CurrentState)
	assert.Empty(t, page.Items[1].ManagedBy)

	docker, err := c.VMs(ctx, "", vm.KindDocker, 1)
	require.NoError(t, err)
	assert.Empty(t, docker.Items, "a run is a machine")

	theirs, err := c.VMs(ctx, "other", "", 1)
	require.NoError(t, err)
	require.Len(t, theirs.Items, 1, "and nobody's own")

	read, err := c.VM(ctx, "", "run")
	require.NoError(t, err)
	assert.Equal(t, vm.ManagedByCodeRunner, read.ManagedBy)
	assert.Equal(t, vm.Resources{CPUs: 2, Memory: 200 * vmtest.MiB, Disk: 100 * vmtest.MiB}, read.Resources)

	_, err = c.VM(ctx, "other", "run")
	assert.ErrorIs(t, err, domain.ErrNotExists)

	lines, err := c.VMLogs(ctx, "", "run", vm.LogOptions{})
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "bye", lines[1].Line)

	var refused *client.ValidationError

	err = c.StartVM(ctx, "", "run")
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"vm": runs.CodeRefused}, refused.ValidationErrors)

	_, err = c.UpdateVM(ctx, "", "run", workloadControlPlane.VMUpdate{Name: new("renamed")})
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"vm": runs.CodeRefused}, refused.ValidationErrors)

	require.NoError(t, c.StopVM(ctx, "", "run"))

	stopping, err := c.VM(ctx, "", "run")
	require.NoError(t, err)
	assert.Equal(t, vm.Stopping, stopping.CurrentState)
	assert.Equal(t, vm.Stopped, stopping.ExpectedState)

	require.NoError(t, c.DeleteVM(ctx, "", "run"))

	deleting, err := c.VM(ctx, "", "run")
	require.NoError(t, err)
	assert.Equal(t, vm.Deleting, deleting.CurrentState, "a run taken away goes once its node has taken it away")
}

// TestContract_Tasks runs a snippet's task as the code runner does, reads it
// back and takes it away, through the resource API every kind is served by.
func TestContract_Tasks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New()
	c := controlPlane(t, w)

	none := 0

	run, err := c.RunTask(ctx, task.GuestOwnerUUID, workloadControlPlane.TaskRequest{
		Name: "0199b3c2-request",
		Spec: taskKind.Spec{
			Kind:        task.KindJob,
			Image:       "ghcr.io/tarhche/code-runner:nodejs-22.14-latest",
			Command:     []string{"--timeout", "120", "serve"},
			Ports:       []port.Port{3000},
			Interactive: true,
			TTL:         2 * time.Minute,
			Limits:      taskKind.Limits{CPU: 2, Memory: 200 * vmtest.MiB, Disk: 100 * vmtest.MiB},
			MaxRetries:  &none,
		},
	})
	require.NoError(t, err)

	assert.NotEmpty(t, run.Metadata.UUID)
	assert.Equal(t, "0199b3c2-request", run.Metadata.Name)
	assert.Equal(t, task.GuestOwnerUUID, run.Metadata.OwnerUUID)
	assert.Equal(t, vmtest.Node, run.Metadata.Node)
	assert.Equal(t, taskKind.Scheduled, run.Status.State, "asked of its node at once")

	sent, err := messagingMock.Produced[kind.ActOnResource](w.Producer, kind.ActOnResourceName)
	require.NoError(t, err)
	require.Len(t, sent, 1)
	assert.Equal(t, taskKind.ActionCreate, sent[0].Action)

	read, err := c.Task(ctx, run.Metadata.UUID)
	require.NoError(t, err)
	assert.Equal(t, run.Metadata.Slug, read.Metadata.Slug)
	assert.Equal(t, []port.Port{3000}, read.Spec.Ports)

	require.NoError(t, c.DeleteTask(ctx, run.Metadata.UUID))

	deleting, err := c.Task(ctx, run.Metadata.UUID)
	require.NoError(t, err)
	assert.Equal(t, taskKind.Deleting, deleting.Status.State)

	_, err = c.Task(ctx, "missing")
	assert.ErrorIs(t, err, domain.ErrNotExists)

	_, err = c.RunTask(ctx, task.GuestOwnerUUID, workloadControlPlane.TaskRequest{Name: "nothing to run"})

	var refused *client.ValidationError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "required_field", refused.Refused()["image"])
}

func TestContract_Snapshots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := blockstest.New(
		vmtest.WithVMs(vmtest.Stopped("01", "owner"), vmtest.Running("02", "owner")),
		vmtest.WithSnapshots(vmtest.Snapshot("stored", "owner", func(s *snapshotKind.Snapshot) {
			s.Metadata.Owners = []kind.Reference{{Kind: "vm", UUID: "02"}}
			s.Spec.VM = snapshotKind.VMRef{UUID: "02", Name: "box"}
		})),
	)
	c := controlPlane(t, w)

	taken, err := c.CreateSnapshot(ctx, "owner", "01", "before")
	require.NoError(t, err)
	assert.Equal(t, snapshot.Creating, taken.State)
	assert.Equal(t, "01", taken.VMUUID)
	assert.Equal(t, "box", taken.VMName)
	assert.Equal(t, vm.KindMachine, taken.Kind)
	assert.Equal(t, uint64(10*vmtest.GiB), taken.Disk)

	read, err := c.Snapshot(ctx, "owner", taken.UUID)
	require.NoError(t, err)
	assert.Equal(t, "before", read.Name)

	page, err := c.Snapshots(ctx, "owner", "01", 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "those taken of one vm")
	assert.Equal(t, taken.UUID, page.Items[0].UUID)

	everybody, err := c.Snapshots(ctx, "", "", 1)
	require.NoError(t, err)
	assert.Len(t, everybody.Items, 2, "and anybody's")

	_, err = c.CreateSnapshot(ctx, "other", "01", "theirs")
	assert.ErrorIs(t, err, domain.ErrNotExists, "somebody else's vm is not there")

	_, err = c.RenameSnapshot(ctx, "owner", taken.UUID, "after")

	var refused *client.ValidationError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"snapshot": "invalid_state_transition"}, refused.ValidationErrors)

	// still being taken on a node that is there: it goes once it is taken.
	require.NoError(t, c.DeleteSnapshot(ctx, "owner", taken.UUID))

	pending, err := c.Snapshot(ctx, "owner", taken.UUID)
	require.NoError(t, err)
	assert.Equal(t, snapshot.Deleting, pending.State)

	err = c.RestoreVM(ctx, "owner", "01", taken.UUID)
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"snapshot_uuid": "snapshot_not_ready"}, refused.ValidationErrors)

	renamed, err := c.RenameSnapshot(ctx, "owner", "stored", "after")
	require.NoError(t, err)
	assert.Equal(t, "after", renamed.Name)
	assert.Equal(t, snapshot.Ready, renamed.State)

	require.NoError(t, c.DeleteSnapshot(ctx, "owner", "stored"))

	_, err = c.Snapshot(ctx, "owner", "stored")
	assert.ErrorIs(t, err, domain.ErrNotExists, "a stored snapshot goes at once")
}

// TestContract_BuildingBlocks holds the blog's client to the containers of
// Docker VMs as the control plane keeps them, as the container kind: those
// kept, and what a VM's dockerd holds that a stack or its terminal made.
func TestContract_BuildingBlocks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("d1", "owner"), vmtest.Running("m1", "owner")))
	c := controlPlane(t, w)

	witnessed(t, w, "d1",
		containerKind.Docker{ID: "s1", Name: "web-abcde-web-1", Image: "nginx:1.27", State: "running", Labels: map[string]string{docker.LabelComposeProject: "web-abcde", docker.LabelComposeService: "web"}},
		containerKind.Docker{ID: "u1", Name: "db", Image: "postgres:17", State: "exited", Status: "Exited (0) 2 minutes ago"},
	)

	containers, err := c.Docker("owner", "d1").Containers(ctx, docker.ContainerFilter{All: true})
	require.NoError(t, err)
	require.Len(t, containers, 2)

	byName := map[string]docker.Container{}
	for _, container := range containers {
		byName[container.Name] = container
	}

	assert.Equal(t, "web-abcde", byName["web-abcde-web-1"].Stack, "a stack's, as it is")
	assert.False(t, byName["web-abcde-web-1"].Unmanaged)
	assert.True(t, byName["db"].Unmanaged, "and one made from its vm's terminal, marked so")

	running, err := c.Docker("owner", "d1").Containers(ctx, docker.ContainerFilter{})
	require.NoError(t, err)
	assert.Len(t, running, 1, "only those that run, unless all are asked for")

	created, err := c.CreateContainer(ctx, "owner", workloadControlPlane.ContainerRequest{
		Container: docker.ContainerSpec{Name: "api", Image: "nginx:1.27"},
	})
	require.NoError(t, err)
	assert.Equal(t, workloadControlPlane.ChosenVM{UUID: "d1", Name: "box", Created: false}, created.VM, "its owner's only docker vm")
	assert.Equal(t, "c-api", created.Container.ID, "as its node made it")
	assert.Equal(t, "running", created.Container.State)

	read, err := c.Docker("owner", "d1").Container(ctx, "api")
	require.NoError(t, err, "named by its name in its vm")
	assert.Equal(t, "c-api", read.ID)

	read, err = c.Docker("owner", "d1").Container(ctx, "u1")
	require.NoError(t, err, "and what nobody keeps, by its docker id")
	assert.Equal(t, "db", read.Name)

	_, err = c.Docker("owner", "d1").Container(ctx, "missing")
	assert.ErrorIs(t, err, domain.ErrNotExists, "a container that is not there")

	err = c.Docker("owner", "m1").Ping(ctx)
	assert.ErrorIs(t, err, vm.ErrNotDocker)

	var refused *client.ValidationError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"vm": "not_docker"}, refused.ValidationErrors)

	err = c.Docker("other", "d1").Ping(ctx)
	assert.ErrorIs(t, err, domain.ErrNotExists, "somebody else's vm is not there")

	everywhere, err := c.Containers(ctx, "owner", "")
	require.NoError(t, err)
	require.Len(t, everywhere, 3)

	for _, container := range everywhere {
		assert.Equal(t, "d1", container.VMUUID)
		assert.Equal(t, "box", container.VMName)
	}

	err = c.Docker("owner", "d1").RemoveContainer(ctx, "api", false)
	assert.ErrorIs(t, err, docker.ErrInvalid, "one that runs is not removed but by force, as docker says")

	require.NoError(t, c.Docker("owner", "d1").RemoveContainer(ctx, "api", true))

	_, err = c.Docker("owner", "d1").Container(ctx, "api")
	assert.ErrorIs(t, err, domain.ErrNotExists, "removed, it is gone")

	t.Run("what somebody who may ask anybody's asks for in a docker vm is its owner's", func(t *testing.T) {
		network, err := c.Docker("", "d1").CreateNetwork(ctx, docker.NetworkSpec{Name: "backend"})
		require.NoError(t, err)
		assert.Equal(t, "n-backend", network.ID, "as its node made it")

		volume, err := c.Docker("", "d1").CreateVolume(ctx, docker.VolumeSpec{Name: "data"})
		require.NoError(t, err)
		assert.Equal(t, "data", volume.Name)

		for _, plural := range []string{networkKind.Name, volumeKind.Name} {
			kept, _, err := w.Memory.GetAll(ctx, plural, resource.Filter{OwnerUUID: "owner"}, 0, 0)
			require.NoError(t, err)
			assert.Len(t, kept, 1, "a %s kept as the vm's owner's", plural)
		}

		networks, err := c.Docker("owner", "d1").Networks(ctx)
		require.NoError(t, err)
		require.Len(t, networks, 1, "listed at once among its owner's, from its record")
		assert.Equal(t, "backend", networks[0].Name)
		assert.False(t, networks[0].Unmanaged)

		volumes, err := c.Docker("owner", "d1").Volumes(ctx)
		require.NoError(t, err)
		require.Len(t, volumes, 1)
		assert.False(t, volumes[0].Unmanaged)
	})
}

func TestContract_Stacks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("d1", "owner")))
	c := controlPlane(t, w)

	created, err := c.CreateStack(ctx, "owner", workloadControlPlane.StackRequest{
		Name:    "web",
		Compose: "services:\n  web:\n    image: nginx:1.27\n",
		VM:      workloadControlPlane.DockerVMChoice{UUID: "d1"},
	})
	require.NoError(t, err)
	assert.Equal(t, "d1", created.VM.UUID)
	assert.Equal(t, stack.Deploying, created.Stack.State, "its vm runs, so it is deployed at once")
	assert.Equal(t, stack.Running, created.Stack.ExpectedState)
	assert.Equal(t, "box", created.Stack.VMName, "a stack names its vm")

	// what compose made for it, and a container of another project.
	witnessed(t, w, "d1",
		containerKind.Docker{ID: "s1", Name: created.Stack.Slug + "-web-1", State: "running", Labels: map[string]string{docker.LabelComposeProject: created.Stack.Slug, docker.LabelComposeService: "web", stackKind.LabelStack: created.Stack.UUID}},
		containerKind.Docker{ID: "o1", Name: "other-web-1", State: "running", Labels: map[string]string{docker.LabelComposeProject: "other", docker.LabelComposeService: "web"}},
	)

	detail, err := c.Stack(ctx, "owner", created.Stack.UUID)
	require.NoError(t, err)
	assert.False(t, detail.VMNotRunning)
	assert.Equal(t, "box", detail.VMName)
	require.Len(t, detail.Containers, 1)
	assert.Equal(t, "web", detail.Containers[0].Service)

	page, err := c.Stacks(ctx, "owner", "d1", 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "box", page.Items[0].VMName)

	err = c.StopStack(ctx, "owner", created.Stack.UUID)

	var refused *client.ValidationError
	require.ErrorAs(t, err, &refused, "a stack being deployed is busy")
	assert.Equal(t, domain.ValidationErrors{"stack": "invalid_state_transition"}, refused.ValidationErrors)

	require.NoError(t, c.DeleteStack(ctx, "owner", created.Stack.UUID, true))

	_, err = c.CreateStack(ctx, "owner", workloadControlPlane.StackRequest{Name: "web", Compose: "nope: ["})
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"compose": "invalid_value"}, refused.ValidationErrors)

	assert.False(t, errors.Is(err, domain.ErrNotExists))
}

// TestContract_NewDockerVM holds a Docker VM made for a stack or a container to
// what the dashboard asked for and nothing else: a form leaves a size it was
// not given at zero and ports nobody added as an empty list, and the VM is
// made with the default for each size it was not given, and no ports at all.
func TestContract_NewDockerVM(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := blockstest.New()
	c := controlPlane(t, w)

	created, err := c.CreateStack(ctx, "owner", workloadControlPlane.StackRequest{
		Name:    "web",
		Compose: "services:\n  web:\n    image: nginx:1.27\n",
		VM: workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{
			Resources: &vm.Resources{Memory: 4 * vmtest.GiB},
			Ports:     []port.Port{},
		}},
	})
	require.NoError(t, err)
	require.True(t, created.VM.Created)

	made, err := c.VM(ctx, "owner", created.VM.UUID)
	require.NoError(t, err)

	assert.Equal(t, vm.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}, made.Resources, "the sizes it was not given are the defaults")
	assert.Empty(t, made.Ports, "ports given empty are none, not the defaults")

	created, err = c.CreateStack(ctx, "owner", workloadControlPlane.StackRequest{
		Name:    "api",
		Compose: "services:\n  api:\n    image: nginx:1.27\n",
		VM:      workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{Name: "api"}},
	})
	require.NoError(t, err)

	made, err = c.VM(ctx, "owner", created.VM.UUID)
	require.NoError(t, err)

	assert.Equal(t, []port.Port{80}, made.Ports, "ports left out are the defaults")
}
