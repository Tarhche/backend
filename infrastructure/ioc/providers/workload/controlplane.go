package workload

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	kindsReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcile"
	controlPlaneGetNode "github.com/khanzadimahdi/testproject/application/workload/controlplane/node/getNode"
	controlPlaneGetNodes "github.com/khanzadimahdi/testproject/application/workload/controlplane/node/getNodes"
	controlPlaneHeartbeatNode "github.com/khanzadimahdi/testproject/application/workload/controlplane/node/heartbeatNode"
	controlPlaneDeleteTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	controlPlaneGetTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/getTask"
	controlPlaneHeartbeatTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/heartbeatTask"
	controlPlaneKillTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/killTask"
	controlPlaneLogTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/logTask"
	controlPlaneReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/reconcile"
	controlPlaneRunTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/runTask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/schedule"
	controlPlaneStopTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/workload/node/events"
	taskEvents "github.com/khanzadimahdi/testproject/domain/workload/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	logrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/logs"
	noderepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/nodes"
	resourcerepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	controlPlaneNodeAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/node"
	controlPlaneTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/task"
	"github.com/nats-io/nats.go"
)

const (
	ControlPlaneSubscribers = "workload:controlplane:subscribers"
)

// controlPlaneProvider builds the workload control plane's messaging singleton, HTTP handler
// and message subscribers.
type controlPlaneProvider struct {
	terminate func()
}

var _ provider.Provider = &controlPlaneProvider{}

func NewManagerProvider() *controlPlaneProvider {
	return &controlPlaneProvider{}
}

func (p *controlPlaneProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *controlPlaneProvider) Boot(ctx context.Context, c provider.Container) error {
	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("workload-controlplane")); err != nil {
		return err
	}

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, "workload-controlplane", logger)
	if err != nil {
		return err
	}

	c.Bind(func() domain.Producer { return pc }, provider.Singleton())
	c.Bind(func() domain.Consumer { return pc }, provider.Singleton())
	c.Bind(func() domain.ProduceConsumer { return pc }, provider.Singleton())

	p.terminate = func() {
		defer pc.Wait()
	}

	return c.Bind(controlPlaneConsoleCommand, provider.Singleton())
}

func (p *controlPlaneProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func controlPlaneConsoleCommand(
	database *mongo.Database,
	natsConnection *nats.Conn,
	jetStreamProduceConsumer domain.ProduceConsumer,
	validator domain.Validator,
	translator translatorContract.Translator,
	iocContainer provider.Container,
) (http.Handler, error) {
	ctx := context.Background()

	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams("workload-controlplane")); err != nil {
		return nil, err
	}

	var controlPlaneConfigs *configs.WorkloadControlPlane
	if err := iocContainer.Resolve(&controlPlaneConfigs); err != nil {
		return nil, err
	}

	taskScheduler := roundrobin.New()

	taskRepository := taskrepository.NewRepository(database)
	nodeRepository := noderepository.NewRepository(database)
	logRepository := logrepository.NewRepository(database)

	// a task's log is only ever read one task at a time, in the
	// order it was written, so that is the index it needs.
	if err := logRepository.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	// the one place a task is handed to a node, whether it is being asked
	// for the first time, again, or after a failure.
	taskSchedule := schedule.New(jetStreamProduceConsumer)

	controlPlaneRunTaskUseCase := controlPlaneRunTask.NewUseCase(taskRepository, jetStreamProduceConsumer, validator)
	controlPlaneDeleteTaskUseCase := controlPlaneDeleteTask.NewUseCase(taskRepository, logRepository, jetStreamProduceConsumer, translator)
	controlPlaneKillTaskUseCase := controlPlaneKillTask.NewUseCase(taskRepository, jetStreamProduceConsumer, translator)
	controlPlaneGetTaskUseCase := controlPlaneGetTask.NewUseCase(taskRepository)

	// the control plane's own heartbeat, which the serve command runs on a ticker.
	if err := iocContainer.Bind(func() *controlPlaneReconcile.UseCase {
		return controlPlaneReconcile.NewUseCase(taskRepository, taskSchedule, jetStreamProduceConsumer, logger)
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	controlPlaneGetNodeUseCase := controlPlaneGetNode.NewUseCase(nodeRepository)
	controlPlaneGetNodesUseCase := controlPlaneGetNodes.NewUseCase(nodeRepository)

	// every kind the control plane runs, VMs, snapshots and stacks, what is
	// not a kind yet beside them, and what they are kept in. What lives in a
	// VM, its stacks, goes with it, and is reset with its disk, as each kind's
	// rules say; its snapshots outlive it.
	resourceRepository := resourcerepository.NewRepository(database)

	workload, err := NewControlPlaneWorkload(controlPlaneConfigs, ControlPlaneStores{
		Resources: resourceRepository,
		Nodes:     nodeRepository,
		Tasks:     taskRepository,
		TaskLogs:  logRepository,
		Archives:  snapshotStore(controlPlaneConfigs.SnapshotStorage),
	}, natsConnection, jetStreamProduceConsumer, logger)
	if err != nil {
		return nil, err
	}

	if err := iocContainer.Bind(func() *kind.Registry[kind.ControlPlaneBinding] {
		return workload.Registry
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	// a kind's collection is indexed here rather than by a migration, which
	// runs without the registry and cannot know the kinds there are.
	for _, d := range workload.Registry.Descriptors() {
		if err := resourceRepository.EnsureKind(ctx, d); err != nil {
			return nil, err
		}
	}

	// every kind's own heartbeat, which the serve command runs beside the
	// tasks'.
	if err := iocContainer.Bind(func() *kindsReconcile.UseCase {
		return workload.Reconcile
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "database", Pinger: infraHealth.NewMongodbPinger(database)},
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

	mux := http.NewServeMux()

	// the task healthcheck probes this
	mux.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	// the code runner reads a task back and takes it away; everything else
	// about a task reaches it as the workload's own messages.
	mux.Handle("GET /api/tasks/{uuid}", controlPlaneTaskAPI.NewShowHandler(controlPlaneGetTaskUseCase))
	mux.Handle("DELETE /api/tasks/{uuid}", controlPlaneTaskAPI.NewDeleteHandler(controlPlaneDeleteTaskUseCase))

	mux.Handle("GET /api/nodes", controlPlaneNodeAPI.NewIndexHandler(controlPlaneGetNodesUseCase))
	mux.Handle("GET /api/nodes/{name}", controlPlaneNodeAPI.NewShowHandler(controlPlaneGetNodeUseCase))

	// every kind, VMs, snapshots and stacks, under its own plural, and what
	// is not a kind yet, the containers in Docker VMs, which the blog reaches
	// on its users' behalf. Every route takes an owner, which narrows it to
	// that person's own.
	if err := workload.Route(mux); err != nil {
		return nil, err
	}

	rateLimited, err := middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute)
	if err != nil {
		return nil, err
	}

	var tracedProfiler *profiler.TracedProfiler
	if err := iocContainer.Resolve(&tracedProfiler); err != nil {
		return nil, err
	}

	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/workload/controlplane",
				// inside Telemetry so profile samples link to the request span
				middleware.NewProfilingMiddleware(
					middleware.NewLogMiddleware(
						middleware.NewCORSMiddleware(
							rateLimited,
						),
						logger,
					),
					tracedProfiler,
				),
			),
		),
		logger,
	)

	subscribers := map[string]domain.MessageHandler{
		nodeEvents.HeartbeatName:        controlPlaneHeartbeatNode.NewHeartbeatHandler(nodeRepository, workload.Observer),
		taskEvents.HeartbeatName:        controlPlaneHeartbeatTask.NewHeartbeatHandler(taskRepository, jetStreamProduceConsumer, controlPlaneDeleteTaskUseCase, controlPlaneKillTaskUseCase),
		taskEvents.TaskRunRequestedName: controlPlaneRunTask.NewTaskRunRequested(controlPlaneRunTaskUseCase, logger),
		taskEvents.TaskCreatedName:      controlPlaneRunTask.NewTaskCreated(taskRepository, nodeRepository, taskScheduler, taskSchedule, logger),
		taskEvents.TaskRanName:          controlPlaneRunTask.NewTaskRan(taskRepository),
		taskEvents.TaskRestartedName:    controlPlaneRunTask.NewTaskRestarted(taskRepository),
		taskEvents.TaskCompletedName:    controlPlaneRunTask.NewTaskCompleted(taskRepository),
		taskEvents.TaskFailedName:       controlPlaneRunTask.NewTaskFailed(taskRepository, logRepository, taskSchedule, controlPlaneDeleteTaskUseCase, logger),
		taskEvents.TaskStoppedName:      controlPlaneStopTask.NewTaskStopped(taskRepository),
		taskEvents.TaskLoggedName:       controlPlaneLogTask.NewTaskLogged(taskRepository, logRepository, controlPlaneConfigs.MaxLogBytes, logger),
	}

	maps.Copy(subscribers, workload.Subscribers)

	// control plane subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(ControlPlaneSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}

// snapshotStore is the bucket snapshots' archives are kept in, reached the
// first time one is taken away so that the control plane starts whether or not
// the bucket can be reached. With no endpoint configured there is none, and an
// archive is left where it is when its snapshot goes.
func snapshotStore(storage configs.WorkloadSnapshotStorage) snapshotKind.Store {
	if len(storage.S3Endpoint) == 0 {
		return nil
	}

	return minio.NewLazy(minio.Options{
		Endpoint:   storage.S3Endpoint,
		AccessKey:  storage.S3AccessKey,
		SecretKey:  storage.S3SecretKey,
		UseSSL:     storage.S3UseSSL,
		BucketName: storage.S3Bucket,
	})
}
