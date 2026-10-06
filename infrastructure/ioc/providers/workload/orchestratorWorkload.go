package workload

import (
	"log/slog"
	"maps"
	"time"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"

	orchestratorAnswerQuery "github.com/khanzadimahdi/testproject/application/workload/orchestrator/answerQuery"
	orchestratorAnswerRequest "github.com/khanzadimahdi/testproject/application/workload/orchestrator/answerRequest"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/lock"
	orchestratorRunCommand "github.com/khanzadimahdi/testproject/application/workload/orchestrator/runCommand"
	orchestratorTakeSnapshot "github.com/khanzadimahdi/testproject/application/workload/orchestrator/snapshot/takeSnapshot"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotContract "github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	snapshotEvents "github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/request"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
	infraDocker "github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
	workloadMetrics "github.com/khanzadimahdi/testproject/infrastructure/workload/metrics"
)

const (
	// snapshotPartSize is how much of a snapshot is held in memory while it
	// is streamed to S3: one part at a time, of this size, whatever the size of
	// the disk.
	snapshotPartSize = 16 << 20

	// nodeRequestTimeout is how long a request to a node is given when it has
	// no image to pull, which is also how long the control plane waits for
	// one.
	nodeRequestTimeout = 30 * time.Second
)

// OrchestratorDependencies are what an orchestrator's workload is wired
// from: its own connections and settings, the engine its VMs run on, and the
// bucket their snapshots are kept in.
type OrchestratorDependencies struct {
	NATS      *nats.Conn
	Engine    vm.Engine
	Archives  snapshotContract.Store
	Producer  domain.Producer
	Validator domain.Validator
	Configs   *configs.WorkloadOrchestrator
	NodeName  string
	Logger    *slog.Logger
}

// OrchestratorWorkload is what an orchestrator does for what it holds: the
// handlers that carry out the control plane's commands of every kind it
// runs, VMs and stacks among them, and its snapshots of VMs, which are not a
// kind yet; the responder that answers the control plane's requests, every
// kind's queries among them; and the kinds it runs, which its heartbeat asks
// what they hold and its API routes the streams and the ports of.
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

	// a stack's command waits for its VM's dockerd, and may then pull images.
	kinds, err := nodeKinds(NodeKindDependencies{
		Engine:         d.Engine,
		Daemons:        daemons,
		Archives:       d.Archives,
		Gauges:         recorder,
		NodeName:       d.NodeName,
		CommandTimeout: d.Configs.PullRequestTimeout(),
	})
	if err != nil {
		return nil, err
	}

	takeSnapshot := orchestratorTakeSnapshot.NewUseCase(d.Engine, d.Archives, locks, d.Producer, d.Validator, recorder, d.NodeName)

	subscribers := map[string]domain.MessageHandler{
		snapshotEvents.SnapshotRequestedName: orchestratorTakeSnapshot.NewSnapshotRequestedHandler(takeSnapshot, d.NodeName, d.Logger),

		// every kind's commands, carried out one at a time per resource.
		kind.CommandName: orchestratorRunCommand.NewCommandHandler(orchestratorRunCommand.NewUseCase(kinds, locks, d.Producer), d.NodeName, d.Logger),
	}

	// the control plane's requests to this node, answered a bounded number at
	// once. A docker request waits for its VM's dockerd first, so it is given
	// that wait on top of its own time. A kind's query is its kind's to
	// answer, and the Docker passthrough's operations are the node's own.
	responder := request.NewResponder(d.NATS, orchestratorAnswerQuery.NewUseCase(kinds, orchestratorAnswerRequest.NewUseCase(daemons, recorder)), request.ResponderOptions{
		Concurrency: d.Configs.NodeRequestConcurrency,
		Timeout:     d.Configs.DockerReadyTimeout + nodeRequestTimeout,
		PullTimeout: d.Configs.PullRequestTimeout(),
	}, d.Logger)

	return &OrchestratorWorkload{Subscribers: subscribers, Responder: responder, Kinds: kinds}, nil
}

// bindOrchestratorWorkload adds to subscribers what carries out the control
// plane's commands of every kind this node runs and its snapshots, and binds
// what answers the control plane's requests and the kinds this node runs.
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
func snapshotArchives(storage configs.WorkloadSnapshotStorage) snapshotContract.Store {
	return minio.NewLazy(minio.Options{
		Endpoint:   storage.S3Endpoint,
		AccessKey:  storage.S3AccessKey,
		SecretKey:  storage.S3SecretKey,
		UseSSL:     storage.S3UseSSL,
		BucketName: storage.S3Bucket,
		PartSize:   snapshotPartSize,
	})
}
