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

	createcontainer "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/createContainer"
	getcontainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/getContainers"
	requestdocker "github.com/khanzadimahdi/testproject/application/workload/controlplane/docker/requestDocker"
	actonresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	admitresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	deleteresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	kindsdispatch "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	getkinds "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	getresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	getresources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	queryresource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	controlplanestacks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/stack"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/archive"
	createsnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/createSnapshot"
	deletesnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/deleteSnapshot"
	getsnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshot"
	getsnapshots "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshots"
	renamesnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/renameSnapshot"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	createvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	deletevm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/dockerVM"
	getvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVM"
	getvmlogs "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVMLogs"
	getvms "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVMs"
	restartvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/restartVM"
	restorevm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/restoreVM"
	startvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/startVM"
	stopvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/stopVM"
	updatevm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/updateVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
	containerAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/container"
	dockerAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/docker"
	kindsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/kinds"
	snapshotAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/snapshot"
	vmAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/vm"
)

// node answers what the control plane asks of it the way a node does: every
// Docker VM holds one container, and every VM's log one line.
func node() *messagingMock.Requester {
	return &messagingMock.Requester{Answer: func(_ context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
		var result any

		switch request.Op {
		case noderequest.OpContainersList:
			result = []noderequest.Container{{ID: "c1", Name: "web", Image: "nginx:1.27", State: "running", Stack: "web-abcde", Service: "web"}}
		case noderequest.OpContainersCreate:
			result = noderequest.Container{ID: "c2", Name: "api", Image: "nginx:1.27", State: "running"}
		case noderequest.OpVMLogs:
			result = []noderequest.VMLogLine{{At: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), Source: vm.LogSourceKernel, Line: "booted"}}
		case noderequest.OpContainersInspect:
			return noderequest.Failed(domain.ErrNotExists), nil
		default:
			return noderequest.Reply{OK: true}, nil
		}

		encoded, _ := json.Marshal(result)

		return noderequest.Reply{OK: true, Result: encoded}, nil
	}}
}

// controlPlane is the control plane's API, wired the way its provider wires
// it, over memory repositories and a node that answers.
func controlPlane(t *testing.T, w *vmtest.Workload) *client.Client {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)
	codes := validator.New(translator.Codes{})
	requester := node()

	createVM := createvm.NewUseCase(w.VMs, w.Tasks, w.Snapshots, w.Quota, w.Lifecycle, codes, createvm.Images{Machine: "ubuntu:24.04", Docker: "docker:29-dind"})
	chooser := dockerVM.NewChooser(w.VMs, createVM, w.Lifecycle, dockerVM.Defaults{
		Resources: vm.Resources{CPUs: 2, Memory: 2 * vmtest.GiB, Disk: 20 * vmtest.GiB},
		Ports:     []port.Port{80},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
	})
	remover := archive.NewRemover(nil, logger)

	mux := http.NewServeMux()
	mux.Handle("GET /api/vms", vmAPI.NewIndexHandler(getvms.NewUseCase(w.VMs, w.Runs)))
	mux.Handle("POST /api/vms", vmAPI.NewCreateHandler(createVM))
	mux.Handle("GET /api/vms/{uuid}", vmAPI.NewShowHandler(getvm.NewUseCase(w.VMs, w.Runs)))
	mux.Handle("PATCH /api/vms/{uuid}", vmAPI.NewUpdateHandler(updatevm.NewUseCase(w.VMs, w.Runs, w.Quota, w.Placement, w.Lifecycle, codes)))
	mux.Handle("DELETE /api/vms/{uuid}", vmAPI.NewDeleteHandler(deletevm.NewUseCase(w.VMs, w.Runs, w.Lifecycle, codes)))
	mux.Handle("POST /api/vms/{uuid}/start", vmAPI.NewStartHandler(startvm.NewUseCase(w.VMs, w.Runs, w.Lifecycle, codes)))
	mux.Handle("POST /api/vms/{uuid}/stop", vmAPI.NewStopHandler(stopvm.NewUseCase(w.VMs, w.Runs, w.Lifecycle, codes)))
	mux.Handle("POST /api/vms/{uuid}/restart", vmAPI.NewRestartHandler(restartvm.NewUseCase(w.VMs, w.Runs, w.Lifecycle, codes)))
	mux.Handle("POST /api/vms/{uuid}/restore", vmAPI.NewRestoreHandler(restorevm.NewUseCase(w.VMs, w.Runs, w.Snapshots, w.Nodes, w.Lifecycle, w.Commander, codes)))
	mux.Handle("GET /api/vms/{uuid}/logs", vmAPI.NewLogsHandler(getvmlogs.NewUseCase(w.VMs, w.Runs, requester, codes)))
	mux.Handle("GET /api/snapshots", snapshotAPI.NewIndexHandler(getsnapshots.NewUseCase(w.Snapshots)))
	mux.Handle("POST /api/vms/{uuid}/snapshots", snapshotAPI.NewCreateHandler(createsnapshot.NewUseCase(w.VMs, w.Runs, w.Snapshots, w.Lifecycle, w.Producer, codes, 10)))
	mux.Handle("GET /api/snapshots/{uuid}", snapshotAPI.NewShowHandler(getsnapshot.NewUseCase(w.Snapshots)))
	mux.Handle("PATCH /api/snapshots/{uuid}", snapshotAPI.NewRenameHandler(renamesnapshot.NewUseCase(w.Snapshots, codes)))
	mux.Handle("DELETE /api/snapshots/{uuid}", snapshotAPI.NewDeleteHandler(deletesnapshot.NewUseCase(w.Snapshots, w.VMs, w.Lifecycle, remover, codes)))
	mux.Handle("POST /api/vms/{uuid}/docker/{op}", dockerAPI.NewRequestHandler(requestdocker.NewUseCase(w.VMs, requester, codes)))
	mux.Handle("GET /api/containers", containerAPI.NewIndexHandler(getcontainers.NewUseCase(w.VMs, requester, logger)))
	mux.Handle("POST /api/containers", containerAPI.NewCreateHandler(createcontainer.NewUseCase(w.VMs, chooser, requester, codes)))

	// stacks are a kind, served by the resource API every kind is.
	resources := resourcesMemory.NewRepository()

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[stackKind.Spec, stackKind.Status](
		stackKind.Descriptor(),
		controlplanestacks.New(w.VMs, chooser, slugs.By(func(ctx context.Context, slug string) (resource.Record, error) {
			return resources.GetOneBySlug(ctx, stackKind.Name, slug)
		})),
	)))

	dispatcher := kindsdispatch.New(resources, w.Producer, waiters.New(), nil)

	require.NoError(t, kindsAPI.Route(mux, registry.Descriptors(), kindsAPI.UseCases{
		Admit:  admitresource.NewUseCase(registry, resources, dispatcher, logger),
		Act:    actonresource.NewUseCase(registry, resources, dispatcher),
		Delete: deleteresource.NewUseCase(registry, resources, dispatcher),
		Get:    getresource.NewUseCase(registry, resources),
		List:   getresources.NewUseCase(registry, resources),
		Query:  queryresource.NewUseCase(registry, resources, requester, nil, nil),
		Kinds:  getkinds.NewUseCase(registry),
	}))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c, err := client.New(server.URL)
	require.NoError(t, err)

	return c
}

func TestContract_VMs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := vmtest.New(vmtest.WithVMs(vmtest.Running("theirs", "other")))
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

	// its node lists it, and has a log of it from then on.
	listed, err := w.VMs.GetOne(ctx, created.UUID)
	require.NoError(t, err)

	listed.LastHeartbeatAt = time.Now()
	_, err = w.VMs.Save(ctx, &listed)
	require.NoError(t, err)

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
	run.ExecutionLogs = []byte("hello\nbye\n")

	w := vmtest.New(vmtest.WithVMs(vmtest.Running("theirs", "other")), vmtest.WithTasks(run))
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
	assert.Equal(t, domain.ValidationErrors{"vm": coderunner.CodeRefused}, refused.ValidationErrors)

	_, err = c.UpdateVM(ctx, "", "run", workloadControlPlane.VMUpdate{Name: new("renamed")})
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"vm": coderunner.CodeRefused}, refused.ValidationErrors)

	require.NoError(t, c.StopVM(ctx, "", "run"))

	stopping, err := c.VM(ctx, "", "run")
	require.NoError(t, err)
	assert.Equal(t, vm.Stopping, stopping.CurrentState)
	assert.Equal(t, vm.Stopped, stopping.ExpectedState)

	require.NoError(t, c.DeleteVM(ctx, "", "run"))

	_, err = c.VM(ctx, "", "run")
	assert.ErrorIs(t, err, domain.ErrNotExists, "a run taken away is gone")
}

func TestContract_Snapshots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := vmtest.New(vmtest.WithVMs(vmtest.Stopped("01", "owner")))
	c := controlPlane(t, w)

	taken, err := c.CreateSnapshot(ctx, "owner", "01", "before")
	require.NoError(t, err)
	assert.Equal(t, snapshot.Creating, taken.State)
	assert.Equal(t, "01", taken.VMUUID)

	read, err := c.Snapshot(ctx, "owner", taken.UUID)
	require.NoError(t, err)
	assert.Equal(t, "before", read.Name)

	page, err := c.Snapshots(ctx, "owner", "01", 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)

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
}

func TestContract_Docker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := vmtest.New(vmtest.WithVMs(vmtest.Docker("d1", "owner"), vmtest.Running("m1", "owner")))
	c := controlPlane(t, w)

	containers, err := c.Docker("owner", "d1").Containers(ctx, docker.ContainerFilter{All: true})
	require.NoError(t, err)
	require.Len(t, containers, 1)
	assert.Equal(t, "web", containers[0].Name)
	assert.Equal(t, "web-abcde", containers[0].Stack)

	_, err = c.Docker("owner", "d1").Container(ctx, "missing")
	assert.ErrorIs(t, err, domain.ErrNotExists, "a container that is not there")

	err = c.Docker("owner", "m1").Ping(ctx)
	assert.ErrorIs(t, err, vm.ErrNotDocker)

	var refused *client.ValidationError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"vm": "not_docker"}, refused.ValidationErrors)

	err = c.Docker("other", "d1").Ping(ctx)
	assert.ErrorIs(t, err, domain.ErrNotExists, "somebody else's vm is not there")

	created, err := c.CreateContainer(ctx, "owner", workloadControlPlane.ContainerRequest{
		Container: docker.ContainerSpec{Name: "api", Image: "nginx:1.27"},
	})
	require.NoError(t, err)
	assert.Equal(t, workloadControlPlane.ChosenVM{UUID: "d1", Name: "box", Created: false}, created.VM)
	assert.Equal(t, "c2", created.Container.ID)

	everywhere, err := c.Containers(ctx, "owner", "")
	require.NoError(t, err)
	require.Len(t, everywhere, 1)
	assert.Equal(t, "d1", everywhere[0].VMUUID)
	assert.Equal(t, "web", everywhere[0].Name)
}

func TestContract_Stacks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := vmtest.New(vmtest.WithVMs(vmtest.Docker("d1", "owner")))
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
	w := vmtest.New()
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
