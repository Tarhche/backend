package workload

import (
	"log/slog"
	"maps"
	"time"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"

	orchestratorAnswerQuery "github.com/khanzadimahdi/testproject/application/workload/orchestrator/answerQuery"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/lock"
	orchestratorRunCommand "github.com/khanzadimahdi/testproject/application/workload/orchestrator/runCommand"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/request"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
	workloadMetrics "github.com/khanzadimahdi/testproject/infrastructure/workload/metrics"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/task/vmruntime"
)

const (
	// snapshotPartSize is how much of a snapshot is held in memory while it
	// is streamed to S3: one part at a time, of this size, whatever the size of
	// the disk.
	snapshotPartSize = 16 << 20

	// nodeRequestTimeout is how long a request to a node is given once its
	// VM's dockerd answers, when it asks one, which is also how long the
	// control plane waits for one.
	nodeRequestTimeout = 30 * time.Second
)

// OrchestratorDependencies are what an orchestrator's workload is wired
// from: its own connections and settings, the engine its VMs run on, the
// runtime the code runner's tasks run on, and the bucket their snapshots are
// kept in. A runtime of nil runs every task as a VM of its own on the
// engine, as the node does.
type OrchestratorDependencies struct {
	NATS     *nats.Conn
	Engine   vm.Engine
	Tasks    task.Runtime
	Archives snapshotKind.Store
	Producer domain.Producer
	Configs  *configs.WorkloadOrchestrator
	NodeName string
	Logger   *slog.Logger
}

// OrchestratorWorkload is what an orchestrator does for what it holds: the
// handler that carries out the control plane's commands of every kind it
// runs, VMs, their snapshots, stacks and tasks among them; the responder that
// answers the control plane's requests, every kind's queries among them; and
// the kinds it runs, which its heartbeat asks what they hold and its API
// routes the streams and the ports of.
type OrchestratorWorkload struct {
	Subscribers map[string]domain.MessageHandler
	Responder   *request.Responder
	Kinds       *kind.Registry[kind.NodeBinding]
}

// NewOrchestratorWorkload wires an orchestrator's workload.
//
// It is the serve command's wiring, and it is what the workload's end-to-end
// test builds an orchestrator from, over an engine and a bucket kept in
// memory: what is tested is what is served.
func NewOrchestratorWorkload(d OrchestratorDependencies) (*OrchestratorWorkload, error) {
	recorder, err := workloadMetrics.New(otel.GetMeterProvider())
	if err != nil {
		return nil, err
	}

	daemons := infraDocker.NewDaemons(d.Engine, d.Configs.DockerReadyTimeout, d.Logger)

	// what changes a resource, every kind's commands and a snapshot taken of
	// a VM, waits for whatever else is being done to it.
	locks := lock.New()

	tasks := d.Tasks
	if tasks == nil {
		tasks = vmruntime.New(d.Engine, d.Logger)
	}

	// a stack's command waits for its VM's dockerd, and may then pull images;
	// a snapshot is taken under its VM's lock.
	kinds, err := nodeKinds(NodeKindDependencies{
		Engine:         d.Engine,
		Tasks:          tasks,
		Daemons:        daemons,
		Archives:       d.Archives,
		Locks:          locks,
		Gauges:         recorder,
		Snapshots:      recorder,
		Timings:        recorder,
		NodeName:       d.NodeName,
		CommandTimeout: d.Configs.PullRequestTimeout(),
	})
	if err != nil {
		return nil, err
	}

	subscribers := map[string]domain.MessageHandler{
		// every kind's commands, carried out one at a time per resource.
		kind.ActOnResourceName: orchestratorRunCommand.NewCommandHandler(orchestratorRunCommand.NewUseCase(kinds, locks, d.Producer), d.NodeName, d.Logger),
	}

	// the control plane's requests to this node, every kind's queries, and
	// the commands for what nobody keeps a record of, which take their
	// resources' locks as every command does: answered a bounded number at
	// once. What asks a Docker VM's dockerd waits for it first, so it is
	// given that wait on top of its own time.
	responder := request.NewResponder(d.NATS, orchestratorAnswerQuery.NewUseCase(kinds, orchestratorAnswerQuery.WithLocks(locks)), request.ResponderOptions{
		Concurrency: d.Configs.NodeRequestConcurrency,
		Timeout:     d.Configs.DockerReadyTimeout + nodeRequestTimeout,
	}, d.Logger)

	return &OrchestratorWorkload{Subscribers: subscribers, Responder: responder, Kinds: kinds}, nil
}

// bindOrchestratorWorkload adds to subscribers what carries out the control
// plane's commands of every kind this node runs, and binds what answers the
// control plane's requests and the kinds this node runs.
func bindOrchestratorWorkload(c provider.Container, d OrchestratorDependencies, subscribers map[string]domain.MessageHandler) (*OrchestratorWorkload, error) {
	workload, err := NewOrchestratorWorkload(d)
	if err != nil {
		return nil, err
	}

	maps.Copy(subscribers, workload.Subscribers)

	if err := c.Bind(func() *request.Responder { return workload.Responder }, provider.Singleton()); err != nil {
		return nil, err
	}

	if err := c.Bind(func() *kind.Registry[kind.NodeBinding] { return workload.Kinds }, provider.Singleton()); err != nil {
		return nil, err
	}

	return workload, nil
}

// snapshotArchives is the bucket this node keeps snapshots in. It is reached
// when a snapshot is first taken or restored, so S3 being away fails that
// rather than this node.
func snapshotArchives(storage configs.WorkloadSnapshotStorage) snapshotKind.Store {
	return minio.NewLazy(minio.Options{
		Endpoint:   storage.S3Endpoint,
		AccessKey:  storage.S3AccessKey,
		SecretKey:  storage.S3SecretKey,
		UseSSL:     storage.S3UseSSL,
		BucketName: storage.S3Bucket,
		PartSize:   snapshotPartSize,
	})
}
