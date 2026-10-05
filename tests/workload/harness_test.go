package workload_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	providers "github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	snapshotsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/snapshots"
	stacksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/stacks"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	storageMemory "github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"
	vmhostAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/vmhost"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

const (
	// nodeName is the one node the workload has.
	nodeName = "workload-orchestrator-01"

	// ownerUUID is whoever uses the dashboard.
	ownerUUID = "owner-uuid"

	// ingressDomain is what a VM's ports are served under.
	ingressDomain = "workload.example.com"

	// beat is how often the node says what it holds, and how often the
	// control plane looks over its VMs: far more often than either does when
	// it is served, so a test does not wait on them.
	beat = 50 * time.Millisecond

	// settle is how long anything a test waits for is given.
	settle = 15 * time.Second
)

// workload is a control plane and one node, as their serve commands wire them,
// with the blog's client in front of the control plane and what the dashboard
// needs to put the answers into words.
type workload struct {
	// the node's engine, its dockerds and the bucket snapshots are kept in.
	engine   *memory.Engine
	dockerd  *dockerd
	archives *storageMemory.Storage

	// what the control plane keeps.
	vms       *vmsMemory.Repository
	snapshots *snapshotsMemory.Repository
	stacks    *stacksMemory.Repository
	nodes     *nodesMemory.Repository

	// the blog's side of it.
	client     workloadControlPlane.Client
	translator translatorContract.Translator
	validator  domain.Validator
	owners     *presenter.Directory
}

// start runs a workload until the test ends.
func start(t *testing.T) *workload {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.DiscardHandler)

	natsURL := natsServer(t)

	english := translator.New(translation.Translations, translation.EN)

	w := &workload{
		dockerd:    newDockerd(t),
		archives:   storageMemory.New(),
		vms:        vmsMemory.NewRepository(),
		snapshots:  snapshotsMemory.NewRepository(),
		stacks:     stacksMemory.NewRepository(),
		nodes:      nodesMemory.NewRepository(),
		translator: english,
		validator:  validator.New(english),
		owners:     presenter.NewDirectory(people{}),
	}

	// docker is there only in a Docker VM, as it is on a node: a machine VM
	// has no such command.
	w.engine = memory.New(
		memory.WithCapacity(16, 64<<30, 1000<<30),
		memory.WithExec(func(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
			if spec, err := w.engine.Spec(id); err != nil || spec.Kind != vm.KindDocker {
				_, _ = io.WriteString(stderr, "sh: docker: not found\n")

				return 127
			}

			return w.dockerd.exec(ctx, id, options, stdin, stdout, stderr)
		}),
	)

	// the node: its commands come over JetStream as the control plane's
	// messages, and its requests over core NATS, on its own subject.
	nodeConfigs := configs.NewWorkloadOrchestrator()
	nodeConfigs.Name = nodeName
	nodeConfigs.DockerReadyTimeout = 5 * time.Second

	nodeConnection := connect(t, natsURL)

	nodeMessages, err := produceConsumer.NewProduceConsumer(nodeConnection, "workload-orchestrator-"+nodeName, logger,
		produceConsumer.WithConcurrency(4),
		produceConsumer.WithProgress(10*time.Second),
	)
	require.NoError(t, err)

	node, err := providers.NewOrchestratorVMs(providers.OrchestratorVMDependencies{
		NATS:      nodeConnection,
		Engine:    throughVMHost(t, w.engine, logger),
		Archives:  w.archives,
		Producer:  nodeMessages,
		Validator: validator.New(english),
		Configs:   nodeConfigs,
		NodeName:  nodeName,
		Logger:    logger,
	})
	require.NoError(t, err)

	// the control plane: its API is what the blog's client calls, and what
	// the nodes report comes over JetStream.
	controlPlaneConnection := connect(t, natsURL)

	controlPlaneMessages, err := produceConsumer.NewProduceConsumer(controlPlaneConnection, "workload-controlplane", logger)
	require.NoError(t, err)

	tasks := &tasksMock.MockTasksRepository{}
	tasks.On("GetOneBySlug", mock.Anything, mock.Anything).Return(task.Task{}, domain.ErrNotExists)

	controlPlane, err := providers.NewControlPlaneVMs(configs.NewWorkloadControlPlane(), providers.ControlPlaneVMStores{
		VMs:       w.vms,
		Snapshots: w.snapshots,
		Stacks:    w.stacks,
		Nodes:     w.nodes,
		Tasks:     tasks,
		Archives:  w.archives,
	}, controlPlaneConnection, controlPlaneMessages, logger)
	require.NoError(t, err)

	mux := http.NewServeMux()
	controlPlane.Route(mux)

	api := httptest.NewServer(mux)

	w.client, err = client.New(api.URL)
	require.NoError(t, err)

	// each side listens before anything is said: a JetStream subject keeps a
	// message only for those already listening, as it does when served.
	for subject, handler := range controlPlane.Subscribers {
		require.NoError(t, controlPlaneMessages.Consume(ctx, subject, handler))
	}

	for subject, handler := range node.Subscribers {
		require.NoError(t, nodeMessages.Consume(ctx, subject, handler))
	}

	require.NoError(t, node.Responder.Serve(ctx, noderequest.Subject(nodeName)))

	beats := every(ctx, beat, func() { _ = node.Heartbeat.Execute(ctx) })
	reconciles := every(ctx, 4*beat, func() { _ = controlPlane.Reconcile.Execute(ctx) })

	t.Cleanup(func() {
		cancel()
		<-beats
		<-reconciles

		controlPlaneMessages.Wait()
		nodeMessages.Wait()
		node.Responder.Wait()

		api.Close()
	})

	// the control plane places VMs on nodes that have said what they offer.
	require.Eventually(t, func() bool {
		_, err := w.nodes.GetOne(ctx, nodeName)

		return err == nil
	}, settle, beat, "the node never said what it offers")

	return w
}

// throughVMHost is engine as a node reaches its own: served by a vmhost on a
// unix socket, and asked through the vmhost's client, so every call, archive
// and exec session crosses the socket as it does when served.
func throughVMHost(t *testing.T, engine vm.Engine, logger *slog.Logger) vm.Engine {
	t.Helper()

	// not the test's own directory, which is named after the test: a unix
	// socket's path is about a hundred bytes at most.
	dir, err := os.MkdirTemp("", "vmhost")
	require.NoError(t, err)

	socket := filepath.Join(dir, "vmhost.sock")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := vmhostAPI.NewServer(engine, logger)
	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: settle}

	go func() {
		_ = httpServer.Serve(listener)
	}()

	t.Cleanup(func() {
		_ = server.Close()
		_ = httpServer.Close()
		_ = os.RemoveAll(dir)
	})

	return vmhost.NewClient(socket)
}

// natsServer is a NATS server with JetStream, of the test's own.
func natsServer(t *testing.T) string {
	t.Helper()

	natsServer, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)

	go natsServer.Start()
	t.Cleanup(natsServer.Shutdown)

	require.True(t, natsServer.ReadyForConnections(5*time.Second), "nats did not come up")

	return natsServer.ClientURL()
}

// connect is a connection of a service's own, as each service has one.
func connect(t *testing.T, url string) *nats.Conn {
	t.Helper()

	connection, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(connection.Close)

	return connection
}

// every runs do every interval until ctx ends, and closes what it returns once
// it has stopped.
func every(ctx context.Context, interval time.Duration, do func()) <-chan struct{} {
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				do()
			}
		}
	}()

	return stopped
}

// people is who uses the dashboard: one person, who owns everything here.
type people struct {
	user.Repository
}

func (people) GetByUUIDs(_ context.Context, uuids []string) ([]user.User, error) {
	var found []user.User

	for _, uuid := range uuids {
		if uuid == ownerUUID {
			found = append(found, user.User{UUID: ownerUUID, Name: "Mahdi", Username: "mahdi"})
		}
	}

	return found, nil
}
