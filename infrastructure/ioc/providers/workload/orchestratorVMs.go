package workload

import (
	"log/slog"
	"maps"
	"time"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"

	orchestratorAnswerRequest "github.com/khanzadimahdi/testproject/application/workload/orchestrator/answerRequest"
	orchestratorRunCommand "github.com/khanzadimahdi/testproject/application/workload/orchestrator/runCommand"
	orchestratorRunStackAction "github.com/khanzadimahdi/testproject/application/workload/orchestrator/stack/runStackAction"
	orchestratorVMHeartbeat "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/beatHeart"
	orchestratorCreateVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/createVM"
	orchestratorDeleteVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/deleteVM"
	orchestratorGetVMLogs "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/getVMLogs"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	orchestratorReconfigureVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/reconfigureVM"
	orchestratorRestartVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/restartVM"
	orchestratorRestoreVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/restoreVM"
	orchestratorSnapshotVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/snapshotVM"
	orchestratorStartVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/startVM"
	orchestratorStopVM "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/stopVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotContract "github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	snapshotEvents "github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	stackEvents "github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmEvents "github.com/khanzadimahdi/testproject/domain/workload/vm/events"
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

// OrchestratorVMDependencies is what an orchestrator's VMs are wired from: its
// own connections and settings, the engine its VMs run on, and the bucket
// their snapshots are kept in.
type OrchestratorVMDependencies struct {
	NATS      *nats.Conn
	Engine    vm.Engine
	Archives  snapshotContract.Store
	Producer  domain.Producer
	Validator domain.Validator
	Configs   *configs.WorkloadOrchestrator
	NodeName  string
	Logger    *slog.Logger
}

// OrchestratorVMs is what an orchestrator does for the VMs it holds: the
// handlers that carry out the control plane's commands about them, their
// snapshots and their stacks, the responder that answers the control plane's
// requests, and the heartbeat that reports them.
//
// It is also what the orchestrator does for every kind it runs, alike: the
// kinds' commands, on workloadCommand, are among the subscribers.
type OrchestratorVMs struct {
	Subscribers map[string]domain.MessageHandler
	Responder   *request.Responder
	Heartbeat   *orchestratorVMHeartbeat.UseCase

	// Kinds are the kinds this node runs, by their node strategies, which
	// the node's heartbeat asks what they hold and its API routes the streams
	// and the ports of.
	Kinds *kind.Registry[kind.NodeBinding]
}

// NewOrchestratorVMs wires an orchestrator's VMs.
//
// It is the serve command's wiring, and it is what the workload's end-to-end
// test builds an orchestrator from, over an engine and a bucket kept in
// memory: what is tested is what is served.
func NewOrchestratorVMs(d OrchestratorVMDependencies) (*OrchestratorVMs, error) {
	recorder, err := workloadMetrics.New(otel.GetMeterProvider())
	if err != nil {
		return nil, err
	}

	daemons := infraDocker.NewDaemons(d.Engine, d.Configs.DockerReadyTimeout, d.Logger)
	locks := lock.New()

	kinds, err := nodeKinds()
	if err != nil {
		return nil, err
	}

	createVM := orchestratorCreateVM.NewUseCase(d.Engine, d.Archives, locks, d.Producer, d.Validator, d.NodeName)
	startVM := orchestratorStartVM.NewUseCase(d.Engine, locks, d.Producer, d.Validator, d.NodeName)
	stopVM := orchestratorStopVM.NewUseCase(d.Engine, daemons, locks, d.Producer, d.Validator, d.NodeName)
	restartVM := orchestratorRestartVM.NewUseCase(d.Engine, daemons, locks, d.Producer, d.Validator, d.NodeName)
	deleteVM := orchestratorDeleteVM.NewUseCase(d.Engine, daemons, locks, d.Producer, d.Validator, d.NodeName)
	reconfigureVM := orchestratorReconfigureVM.NewUseCase(d.Engine, daemons, locks, d.Producer, d.Validator, d.NodeName)
	restoreVM := orchestratorRestoreVM.NewUseCase(d.Engine, d.Archives, daemons, locks, d.Producer, d.Validator, d.NodeName)
	snapshotVM := orchestratorSnapshotVM.NewUseCase(d.Engine, d.Archives, locks, d.Producer, d.Validator, recorder, d.NodeName)

	// a stack action waits for dockerd, and may then pull images.
	runStackAction := orchestratorRunStackAction.NewUseCase(daemons, d.Producer, d.Validator, d.NodeName,
		d.Configs.PullRequestTimeout())

	subscribers := map[string]domain.MessageHandler{
		vmEvents.VMScheduledName:             orchestratorCreateVM.NewVMScheduledHandler(createVM, d.Producer, d.NodeName, d.Logger),
		vmEvents.VMStartRequestedName:        orchestratorStartVM.NewVMStartRequestedHandler(startVM, d.Producer, d.NodeName, d.Logger),
		vmEvents.VMStopRequestedName:         orchestratorStopVM.NewVMStopRequestedHandler(stopVM, d.Producer, d.NodeName, d.Logger),
		vmEvents.VMRestartRequestedName:      orchestratorRestartVM.NewVMRestartRequestedHandler(restartVM, d.Producer, d.NodeName, d.Logger),
		vmEvents.VMDeleteRequestedName:       orchestratorDeleteVM.NewVMDeleteRequestedHandler(deleteVM, d.Producer, d.NodeName, d.Logger),
		vmEvents.VMReconfigureRequestedName:  orchestratorReconfigureVM.NewVMReconfigureRequestedHandler(reconfigureVM, d.Producer, d.NodeName, d.Logger),
		vmEvents.VMRestoreRequestedName:      orchestratorRestoreVM.NewVMRestoreRequestedHandler(restoreVM, d.Producer, d.NodeName, d.Logger),
		snapshotEvents.SnapshotRequestedName: orchestratorSnapshotVM.NewSnapshotRequestedHandler(snapshotVM, d.NodeName, d.Logger),
		stackEvents.StackRequestedName:       orchestratorRunStackAction.NewStackRequestedHandler(runStackAction, d.NodeName, d.Logger),

		// every kind's commands, carried out under the same locks as the
		// VMs' own, so what is done to a VM by either waits for the other.
		kind.CommandName: orchestratorRunCommand.NewCommandHandler(orchestratorRunCommand.NewUseCase(kinds, locks, d.Producer), d.NodeName, d.Logger),
	}

	// the control plane's requests to this node, answered a bounded number at
	// once. A docker request waits for its VM's dockerd first, so it is given
	// that wait on top of its own time.
	answerRequest := orchestratorAnswerRequest.NewUseCase(daemons, orchestratorGetVMLogs.NewUseCase(d.Engine, d.Validator), recorder)

	responder := request.NewResponder(d.NATS, answerRequest, request.ResponderOptions{
		Concurrency: d.Configs.NodeRequestConcurrency,
		Timeout:     d.Configs.DockerReadyTimeout + nodeRequestTimeout,
		PullTimeout: d.Configs.PullRequestTimeout(),
	}, d.Logger)

	// the VM heartbeat, which also says whether the vmhost is answering.
	vmHeartbeat := orchestratorVMHeartbeat.NewUseCase(d.Engine, d.Producer, recorder, d.NodeName, d.Logger)

	return &OrchestratorVMs{Subscribers: subscribers, Responder: responder, Heartbeat: vmHeartbeat, Kinds: kinds}, nil
}

// bindOrchestratorVMs adds to subscribers what carries out the control plane's
// commands about this node's VMs, their snapshots and their stacks, and those
// of every kind it runs, and binds the VM heartbeat, what answers the control
// plane's requests, and the kinds this node runs.
func bindOrchestratorVMs(c provider.Container, d OrchestratorVMDependencies, subscribers map[string]domain.MessageHandler) (*OrchestratorVMs, error) {
	vms, err := NewOrchestratorVMs(d)
	if err != nil {
		return nil, err
	}

	maps.Copy(subscribers, vms.Subscribers)

	if err := c.Bind(func() *request.Responder { return vms.Responder }, provider.Singleton()); err != nil {
		return nil, err
	}

	if err := c.Bind(func() *kind.Registry[kind.NodeBinding] { return vms.Kinds }, provider.Singleton()); err != nil {
		return nil, err
	}

	if err := c.Bind(func() *orchestratorVMHeartbeat.UseCase { return vms.Heartbeat }, provider.Singleton()); err != nil {
		return nil, err
	}

	return vms, nil
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
