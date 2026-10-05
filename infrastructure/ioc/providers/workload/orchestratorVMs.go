package workload

import (
	"log/slog"
	"time"

	"github.com/danceable/provider"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"

	orchestratorAnswerRequest "github.com/khanzadimahdi/testproject/application/workload/orchestrator/answerRequest"
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

// vmDependencies is what the VMs' wiring needs from the orchestrator's own.
type vmDependencies struct {
	natsConnection *nats.Conn
	engine         vm.Engine
	producer       domain.Producer
	validator      domain.Validator
	configs        *configs.WorkloadOrchestrator
	nodeName       string
	logger         *slog.Logger
}

// bindOrchestratorVMs adds to subscribers what carries out the control plane's
// commands about this node's VMs, their snapshots and their stacks, and binds
// the VM heartbeat and what answers the control plane's requests.
func bindOrchestratorVMs(c provider.Container, d vmDependencies, subscribers map[string]domain.MessageHandler) error {
	recorder, err := workloadMetrics.New(otel.GetMeterProvider())
	if err != nil {
		return err
	}

	// the snapshots bucket is reached when a snapshot is first taken or
	// restored, so S3 being away fails that rather than this node.
	store := minio.NewLazy(minio.Options{
		Endpoint:   d.configs.SnapshotStorage.S3Endpoint,
		AccessKey:  d.configs.SnapshotStorage.S3AccessKey,
		SecretKey:  d.configs.SnapshotStorage.S3SecretKey,
		UseSSL:     d.configs.SnapshotStorage.S3UseSSL,
		BucketName: d.configs.SnapshotStorage.S3Bucket,
		PartSize:   snapshotPartSize,
	})

	daemons := infraDocker.NewDaemons(d.engine, d.configs.DockerReadyTimeout, d.logger)
	locks := lock.New()

	createVM := orchestratorCreateVM.NewUseCase(d.engine, store, locks, d.producer, d.validator, d.nodeName)
	startVM := orchestratorStartVM.NewUseCase(d.engine, locks, d.producer, d.validator, d.nodeName)
	stopVM := orchestratorStopVM.NewUseCase(d.engine, daemons, locks, d.producer, d.validator, d.nodeName)
	restartVM := orchestratorRestartVM.NewUseCase(d.engine, daemons, locks, d.producer, d.validator, d.nodeName)
	deleteVM := orchestratorDeleteVM.NewUseCase(d.engine, daemons, locks, d.producer, d.validator, d.nodeName)
	reconfigureVM := orchestratorReconfigureVM.NewUseCase(d.engine, daemons, locks, d.producer, d.validator, d.nodeName)
	restoreVM := orchestratorRestoreVM.NewUseCase(d.engine, store, daemons, locks, d.producer, d.validator, d.nodeName)
	snapshotVM := orchestratorSnapshotVM.NewUseCase(d.engine, store, locks, d.producer, d.validator, recorder, d.nodeName)

	// a stack action waits for dockerd, and may then pull images.
	runStackAction := orchestratorRunStackAction.NewUseCase(daemons, d.producer, d.validator, d.nodeName,
		d.configs.DockerReadyTimeout+d.configs.DockerPullTimeout)

	subscribers[vmEvents.VMScheduledName] = orchestratorCreateVM.NewVMScheduledHandler(createVM, d.producer, d.nodeName, d.logger)
	subscribers[vmEvents.VMStartRequestedName] = orchestratorStartVM.NewVMStartRequestedHandler(startVM, d.producer, d.nodeName, d.logger)
	subscribers[vmEvents.VMStopRequestedName] = orchestratorStopVM.NewVMStopRequestedHandler(stopVM, d.producer, d.nodeName, d.logger)
	subscribers[vmEvents.VMRestartRequestedName] = orchestratorRestartVM.NewVMRestartRequestedHandler(restartVM, d.producer, d.nodeName, d.logger)
	subscribers[vmEvents.VMDeleteRequestedName] = orchestratorDeleteVM.NewVMDeleteRequestedHandler(deleteVM, d.producer, d.nodeName, d.logger)
	subscribers[vmEvents.VMReconfigureRequestedName] = orchestratorReconfigureVM.NewVMReconfigureRequestedHandler(reconfigureVM, d.producer, d.nodeName, d.logger)
	subscribers[vmEvents.VMRestoreRequestedName] = orchestratorRestoreVM.NewVMRestoreRequestedHandler(restoreVM, d.producer, d.nodeName, d.logger)
	subscribers[snapshotEvents.SnapshotRequestedName] = orchestratorSnapshotVM.NewSnapshotRequestedHandler(snapshotVM, d.nodeName, d.logger)
	subscribers[stackEvents.StackRequestedName] = orchestratorRunStackAction.NewStackRequestedHandler(runStackAction, d.nodeName, d.logger)

	// the control plane's requests to this node, answered a bounded number at
	// once. A docker request waits for its VM's dockerd first, so it is given
	// that wait on top of its own time.
	answerRequest := orchestratorAnswerRequest.NewUseCase(daemons, orchestratorGetVMLogs.NewUseCase(d.engine, d.validator), recorder)

	responder := request.NewResponder(d.natsConnection, answerRequest, request.ResponderOptions{
		Concurrency: d.configs.NodeRequestConcurrency,
		Timeout:     d.configs.DockerReadyTimeout + nodeRequestTimeout,
		PullTimeout: d.configs.DockerReadyTimeout + d.configs.DockerPullTimeout,
	}, d.logger)

	if err := c.Bind(func() *request.Responder { return responder }, provider.Singleton()); err != nil {
		return err
	}

	// the VM heartbeat, which also says whether the vmhost is answering.
	vmHeartbeat := orchestratorVMHeartbeat.NewUseCase(d.engine, d.producer, recorder, d.nodeName, d.logger)

	return c.Bind(func() *orchestratorVMHeartbeat.UseCase { return vmHeartbeat }, provider.Singleton())
}
