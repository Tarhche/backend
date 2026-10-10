package workload_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	kindsReconcileResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	controlPlaneHeartbeatNode "github.com/khanzadimahdi/testproject/application/workload/controlplane/node/heartbeatNode"
	orchestratorHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/beatHeart"
	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	providers "github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/pubsub"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
	leasesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/leases"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
	storageMemory "github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/workload/node"
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

// patience is how patient the control plane's reconcile loop is with the
// resources of every kind: far less than when it is served, so a test does
// not wait on its backoff, nor long for what a node no longer holds to be
// taken to be gone, which nothing but time says.
var patience = kindsReconcileResources.Config{
	Batch:               20,
	NodeSilentAfter:     30 * time.Second,
	ResourceSilentAfter: 3 * time.Second,
	Patience:            10 * time.Second,
	Backoff:             500 * time.Millisecond,
	MaxBackoff:          2 * time.Second,
}

// workload is a control plane and one node, as their serve commands wire them,
// with the blog's client in front of the control plane and what the dashboard
// needs to put the answers into words.
type workload struct {
	// the node's engine, its dockerds and the bucket snapshots are kept in.
	engine   *memory.Engine
	dockerd  *dockerd
	archives *storageMemory.Storage

	// what the control plane keeps: the resources of every kind, VMs, their
	// snapshots, stacks and the code runner's tasks among them, the nodes,
	// and how each node of each root's plan went, the last time it was
	// carried out.
	resources  *resourcesMemory.Repository
	nodes      *nodesMemory.Repository
	executions *executionsMemory.Repository

	// programs are what a task's program does once its VM boots, as a test
	// says; kinds are the kinds the node runs; and natsURL is where the
	// workload's messages travel, which the blog listens to as well.
	programs programs
	kinds    *kind.Registry[kind.NodeBinding]
	natsURL  string

	// beating says whether the node says what it holds: a node that stops
	// is one the control plane hears nothing more from.
	beating atomic.Bool

	// boots, while it holds a channel, holds every VM made in its boot until
	// it is closed (holdBoots).
	boots atomic.Pointer[chan struct{}]

	// api is where the control plane's own API is served, which the blog's
	// client reaches it on, and what a test asks of it directly when the blog
	// has no word for it.
	api string

	// the blog's side of it.
	client     workloadControlPlane.Client
	translator translatorContract.Translator
	validator  domain.Validator
	owners     *presenter.Directory
}

// option is how a test's workload differs from every other's.
type option func(*settings)

// settings are what options change: how patient the control plane's
// reconcile loop is, and whether every root is queued as often as that loop
// runs, as a resync does.
type settings struct {
	reconcile kindsReconcileResources.Config
	resync    bool
}

// silentAfter has the control plane give up on a node that has said nothing
// for this long, rather than for as long as it does when served.
func silentAfter(d time.Duration) option {
	return func(s *settings) {
		s.reconcile.NodeSilentAfter = d
	}
}

// withoutResync has the control plane never queue every root: a root is
// carried out only when what it is told of reaches it.
func withoutResync() option {
	return func(s *settings) {
		s.resync = false
	}
}

// start runs a workload until the test ends.
func start(t *testing.T, options ...option) *workload {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.DiscardHandler)

	set := settings{reconcile: patience, resync: true}
	for _, option := range options {
		option(&set)
	}

	reconcileConfig := set.reconcile

	natsURL := natsServer(t)

	english := translator.New(translation.Translations, translation.EN)

	w := &workload{
		dockerd:    newDockerd(t),
		archives:   storageMemory.New(),
		resources:  resourcesMemory.NewRepository(),
		nodes:      nodesMemory.NewRepository(),
		executions: executionsMemory.NewRepository(),
		natsURL:    natsURL,
		translator: english,
		validator:  validator.New(english),
		owners:     presenter.NewDirectory(people{}),
	}

	w.beating.Store(true)

	// the control plane's settings as it is served, its Docker image among
	// them, which the node's engine is given too, as a vmhost is.
	controlPlaneConfigs := configs.NewWorkloadControlPlane()

	// docker is there only in a Docker VM, as it is on a node: a machine VM
	// has no such command. A VM that runs a program of its own, a task's, runs
	// what the test says it does.
	w.engine = memory.New(
		memory.WithCapacity(16, 64<<30, 1000<<30),
		memory.WithDockerImage(controlPlaneConfigs.VMDockerImage),
		memory.WithMain(w.programs.main),
		memory.WithExec(func(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
			if spec, err := w.engine.Spec(id); err != nil || vm.KindOf(spec.Image, controlPlaneConfigs.VMDockerImage) != vm.KindDocker {
				_, _ = io.WriteString(stderr, "sh: docker: not found\n")

				return 127
			}

			return w.dockerd.exec(ctx, id, options, stdin, stdout, stderr)
		}),
	)

	// what a Docker VM's dockerd holds is on the VM's disk.
	w.dockerd.disks = w.engine

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

	nodeEngine := throughVMHost(t, &booting{Engine: w.engine, held: &w.boots}, logger)

	node, err := providers.NewOrchestratorWorkload(providers.OrchestratorDependencies{
		NATS:     nodeConnection,
		Engine:   nodeEngine,
		Archives: w.archives,
		Producer: nodeMessages,
		Configs:  nodeConfigs,
		NodeName: nodeName,
		Logger:   logger,
	})
	require.NoError(t, err)

	w.kinds = node.Kinds

	// the node's heartbeat, which says what it offers, and what every kind it
	// runs holds on it, VMs among them, each in a heartbeat of its own, as its
	// serve command beats it.
	nodeHeartbeat := orchestratorHeartbeat.NewUseCase(nodeMessages, infraNode.NewManager(nodeEngine), node.Kinds, time.Second, nodeName, logger)

	// the control plane: its API is what the blog's client calls, and what
	// the nodes report comes over JetStream.
	controlPlaneConnection := connect(t, natsURL)

	controlPlaneMessages, err := produceConsumer.NewProduceConsumer(controlPlaneConnection, "workload-controlplane", logger)
	require.NoError(t, err)

	controlPlane, err := providers.NewControlPlaneWorkload(controlPlaneConfigs, providers.ControlPlaneStores{
		Resources:  w.resources,
		Nodes:      w.nodes,
		TaskLogs:   logsMock.NewInMemoryRepository(),
		Executions: w.executions,
		Leases:     leasesMemory.NewRepository(nil),
		Archives:   w.archives,
	}, controlPlaneConnection, controlPlaneMessages, logger, providers.WithReconcileConfig(reconcileConfig))
	require.NoError(t, err)

	mux := http.NewServeMux()
	require.NoError(t, controlPlane.Route(mux))

	api := httptest.NewServer(mux)
	w.api = api.URL

	w.client, err = client.New(api.URL, controlPlaneConfigs.VMDockerImage)
	require.NoError(t, err)

	// each side listens before anything is said: a JetStream subject keeps a
	// message only for those already listening, as it does when served. The
	// control plane hears every kind's results and heartbeats among its
	// workload's subscribers, and each node's own heartbeat beside them.
	for subject, handler := range controlPlane.Subscribers {
		require.NoError(t, controlPlaneMessages.Consume(ctx, subject, handler))
	}

	require.NoError(t, controlPlaneMessages.Consume(ctx, nodeEvents.HeartbeatName, controlPlaneHeartbeatNode.NewHeartbeatHandler(w.nodes)))

	for subject, handler := range node.Subscribers {
		require.NoError(t, nodeMessages.Consume(ctx, subject, handler))
	}

	require.NoError(t, node.Responder.Serve(ctx, noderequest.Subject(nodeName)))

	nodeBeats := every(ctx, beat, func() {
		if w.beating.Load() {
			_ = nodeHeartbeat.Execute(ctx)
		}
	})
	reconciles := every(ctx, 4*beat, func() { _ = controlPlane.Reconcile.Execute(ctx) })

	// the graphs of roots, kept made by the control plane's workers, every
	// root queued as often as the kinds' own loop runs here.
	graphsWorked := make(chan struct{})
	go func() {
		defer close(graphsWorked)

		controlPlane.Graphs.Work(ctx)
	}()

	resyncs := every(ctx, 4*beat, func() {
		if set.resync {
			_ = controlPlane.Graphs.Resync(ctx)
		}
	})

	t.Cleanup(func() {
		cancel()
		<-nodeBeats
		<-reconciles
		<-graphsWorked
		<-resyncs

		controlPlaneMessages.Wait()
		nodeMessages.Wait()
		node.Responder.Wait()

		api.Close()
	})

	// the control plane places VMs on nodes that have said what they offer.
	require.Eventually(t, func() bool {
		n, err := w.nodes.GetOne(ctx, nodeName)

		return err == nil && n.Capacity.Memory > 0
	}, settle, beat, "the node never said what it offers")

	return w
}

// holdBoots holds every VM made from now on in its boot, made by its engine
// only once what it hands back lets it go: a Docker VM is scheduled until
// then, as one coming up is for a minute or two when served.
func (w *workload) holdBoots() (release func()) {
	held := make(chan struct{})
	w.boots.Store(&held)

	var once sync.Once

	return func() {
		once.Do(func() {
			w.boots.Store(nil)
			close(held)
		})
	}
}

// booting is an engine that makes a VM only once the boots it is given are
// let go, when they are held.
type booting struct {
	vm.Engine

	held *atomic.Pointer[chan struct{}]
}

func (b *booting) Create(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	if held := b.held.Load(); held != nil {
		select {
		case <-*held:
		case <-ctx.Done():
			return vm.Instance{}, ctx.Err()
		}
	}

	return b.Engine.Create(ctx, spec)
}

// controlPlane asks the control plane's own API, as its owner, for what the
// blog has no word for, and is its status and its body.
func (w *workload) controlPlane(t *testing.T, method string, path string, query url.Values) (int, []byte) {
	t.Helper()

	return w.controlPlaneWith(t, method, path, query, "")
}

// controlPlaneWith asks the control plane's own API, as its owner, with body,
// and is its status and its body.
func (w *workload) controlPlaneWith(t *testing.T, method string, path string, query url.Values, body string) (int, []byte) {
	t.Helper()

	asked := url.Values{"owner": {ownerUUID}}
	for key, values := range query {
		asked[key] = values
	}

	var sent io.Reader
	if len(body) > 0 {
		sent = strings.NewReader(body)
	}

	request, err := http.NewRequestWithContext(t.Context(), method, w.api+path+"?"+asked.Encode(), sent)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	answered, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, answered
}

// ingress is the kinds the workload's ingress finds, wired as its serve
// command wires them, hearing where the tasks and the VMs are over core NATS
// as it does when served: what their node says of them, and what the control
// plane sends their node, beside the JetStream consumers that keep the same
// messages for the others.
func (w *workload) ingress(t *testing.T) *kind.Registry[kind.IngressBinding] {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	served, err := providers.NewIngressWorkload(ingressMemory.NewLocations(configs.NewWorkloadIngress().ResourceSilentAfter), logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	subscriber := pubsub.NewPublishSubscriber(connect(t, w.natsURL), logger)

	for subject, handler := range served.Subscribers {
		require.NoError(t, subscriber.Subscribe(ctx, subject, handler))
	}

	t.Cleanup(func() {
		cancel()
		subscriber.Wait()
	})

	return served.Kinds
}

// reached is where the ingress finds the resource of the named kind a slug
// names, and why it cannot reach it, once condition holds of the two: it
// hears what is said of the resource as it is said.
func reached(t *testing.T, kinds *kind.Registry[kind.IngressBinding], kindName string, slug string, what string, condition func(kind.Location, error) bool) (kind.Location, error) {
	t.Helper()

	binding, registered := kinds.Lookup(kindName)
	require.True(t, registered, "the ingress finds no %s", kindName)

	var (
		lock     sync.Mutex
		location kind.Location
		failure  error
	)

	held := assert.Eventually(t, func() bool {
		found, err := binding.BySlug(t.Context(), slug)

		lock.Lock()
		defer lock.Unlock()

		location, failure = found, err

		return condition(found, err)
	}, settle, beat)

	lock.Lock()
	defer lock.Unlock()

	if !held {
		t.Fatalf("%s never happened: last found %+v, %v", what, location, failure)
	}

	return location, failure
}

// programs are what the programs VMs run as their main process do, a
// code-runner task's: what each writes, and what it exits with. Until a test
// says, one runs until it is stopped.
type programs struct {
	lock sync.Mutex
	run  memory.MainFunc
}

// are has every program from now on do what run does.
func (p *programs) are(run memory.MainFunc) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.run = run
}

func (p *programs) main(ctx context.Context, spec vm.Spec, log func(line string)) int {
	p.lock.Lock()
	run := p.run
	p.lock.Unlock()

	if run == nil {
		<-ctx.Done()

		return 0
	}

	return run(ctx, spec, log)
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
