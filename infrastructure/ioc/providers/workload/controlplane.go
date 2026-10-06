package workload

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/mongo"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	controlPlaneCreateContainer "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/createContainer"
	controlPlaneGetContainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/getContainers"
	controlPlaneRequestDocker "github.com/khanzadimahdi/testproject/application/workload/controlplane/docker/requestDocker"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/cascade"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	kindsReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcile"
	controlPlaneGetNode "github.com/khanzadimahdi/testproject/application/workload/controlplane/node/getNode"
	controlPlaneGetNodes "github.com/khanzadimahdi/testproject/application/workload/controlplane/node/getNodes"
	controlPlaneHeartbeatNode "github.com/khanzadimahdi/testproject/application/workload/controlplane/node/heartbeatNode"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/archive"
	controlPlaneCreateSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/createSnapshot"
	controlPlaneDeleteSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/deleteSnapshot"
	controlPlaneGetSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshot"
	controlPlaneGetSnapshots "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshots"
	controlPlaneRenameSnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/renameSnapshot"
	controlPlaneDeleteTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	controlPlaneGetTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/getTask"
	controlPlaneHeartbeatTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/heartbeatTask"
	controlPlaneKillTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/killTask"
	controlPlaneLogTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/logTask"
	controlPlaneReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/reconcile"
	controlPlaneRunTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/runTask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/schedule"
	controlPlaneStopTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/stopTask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/command"
	controlPlaneCreateVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	controlPlaneDeleteVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/dockerVM"
	controlPlaneFailVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/failVM"
	controlPlaneGetVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVM"
	controlPlaneGetVMLogs "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVMLogs"
	controlPlaneGetVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVMs"
	controlPlaneHeartbeatVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/heartbeatVM"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/parents"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/quota"
	controlPlaneVMReconcile "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/reconcile"
	controlPlaneRestartVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/restartVM"
	controlPlaneRestoreVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/restoreVM"
	controlPlaneStartVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/startVM"
	controlPlaneStopVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/stopVM"
	controlPlaneUpdateVM "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/updateVM"
	"github.com/khanzadimahdi/testproject/domain"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	nodeContract "github.com/khanzadimahdi/testproject/domain/workload/node"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/workload/node/events"
	snapshotContract "github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	snapshotEvents "github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	taskContract "github.com/khanzadimahdi/testproject/domain/workload/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/workload/task/events"
	vmContract "github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmEvents "github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/request"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	logrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/logs"
	noderepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/nodes"
	resourcerepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
	snapshotrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/snapshots"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/tasks"
	vmrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/vms"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	infraTranslator "github.com/khanzadimahdi/testproject/infrastructure/translator"
	infraValidator "github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	controlPlaneContainerAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/container"
	controlPlaneDockerAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/docker"
	controlPlaneNodeAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/node"
	controlPlaneSnapshotAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/snapshot"
	controlPlaneTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/task"
	controlPlaneVMAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/vm"
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

	// every kind the control plane runs, and what their resources are kept
	// in. What lives in a VM, its stacks, goes with the VM, and is reset with
	// its disk, as each kind's rules say.
	registry, err := NewControlPlaneRegistry()
	if err != nil {
		return nil, err
	}

	resourceRepository := resourcerepository.NewRepository(database)

	vms, err := NewControlPlaneVMs(controlPlaneConfigs, ControlPlaneVMStores{
		VMs:       vmrepository.NewRepository(database),
		Snapshots: snapshotrepository.NewRepository(database),
		Children:  cascade.New(registry, resourceRepository),
		Nodes:     nodeRepository,
		Tasks:     taskRepository,
		TaskLogs:  logRepository,
		Archives:  snapshotStore(controlPlaneConfigs.SnapshotStorage),
	}, natsConnection, jetStreamProduceConsumer, logger)
	if err != nil {
		return nil, err
	}

	// the VMs' own heartbeat, which the serve command runs beside the tasks'.
	if err := iocContainer.Bind(func() *controlPlaneVMReconcile.UseCase {
		return vms.Reconcile
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	if err := RegisterControlPlaneKinds(registry, vms, resourceRepository); err != nil {
		return nil, err
	}

	if err := iocContainer.Bind(func() *kind.Registry[kind.ControlPlaneBinding] {
		return registry
	}, provider.Singleton()); err != nil {
		return nil, err
	}

	// a kind's collection is indexed here rather than by a migration, which
	// runs without the registry and cannot know the kinds there are.
	for _, d := range registry.Descriptors() {
		if err := resourceRepository.EnsureKind(ctx, d); err != nil {
			return nil, err
		}
	}

	kinds := NewControlPlaneKinds(
		registry,
		ControlPlaneKindStores{Resources: resourceRepository, Nodes: nodeRepository},
		request.NewRequester(natsConnection, controlPlaneConfigs.NodeRequestTimeout, controlPlaneConfigs.PullRequestTimeout()),
		jetStreamProduceConsumer,
		logger,
		WithParents(vms.Parents),
	)

	// every kind's own heartbeat, which the serve command runs beside the
	// tasks' and the VMs'.
	if err := iocContainer.Bind(func() *kindsReconcile.UseCase {
		return kinds.Reconcile
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

	// VMs, their snapshots and the containers in Docker VMs, which the blog
	// reaches on its users' behalf. Every route takes an owner, which narrows
	// it to that person's own.
	vms.Route(mux)

	// the resource API of every kind registered, each under its own plural,
	// stacks among them, and the kinds themselves.
	if err := kinds.Route(mux); err != nil {
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
		nodeEvents.HeartbeatName:        controlPlaneHeartbeatNode.NewHeartbeatHandler(nodeRepository, kinds.Observer),
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

	maps.Copy(subscribers, vms.Subscribers)
	maps.Copy(subscribers, kinds.Subscribers)

	// control plane subscribers
	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(ControlPlaneSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}

// ControlPlaneVMStores are what the control plane keeps VMs and snapshots in,
// what lives in VMs, which is told when one goes or has its disk restored,
// the nodes and tasks it weighs them against, the tasks' logs, which go with
// a run of the code runner's when one is taken away from among the VMs, and
// the bucket a snapshot's archive is taken out of when the snapshot goes.
// Archives may be nil: with no bucket configured, an archive is left where it
// is.
type ControlPlaneVMStores struct {
	VMs       vmContract.Repository
	Snapshots snapshotContract.Repository
	Children  lifecycle.Children
	Nodes     nodeContract.Repository
	Tasks     taskContract.Repository
	TaskLogs  taskContract.LogRepository
	Archives  snapshotContract.Store
}

// ControlPlaneVMs is the control plane's VMs, snapshots and containers: the
// routes the blog reaches them on, what it hears from the nodes about them,
// and the heartbeat that keeps them as they were asked to be.
//
// It is also what the kinds that live in Docker VMs are built from: the VMs,
// the chooser of the Docker VM a stack goes into, and what a VM is doing to
// what lives in it.
type ControlPlaneVMs struct {
	Route       func(mux *http.ServeMux)
	Subscribers map[string]domain.MessageHandler
	Reconcile   *controlPlaneVMReconcile.UseCase

	VMs     vmContract.Repository
	Chooser *dockerVM.Chooser
	Parents observe.Parents
}

// NewControlPlaneVMs builds what the control plane keeps and asks about VMs.
//
// It is the serve command's wiring, and it is what the workload's end-to-end
// test builds a control plane from, over stores kept in memory: what is tested
// is what is served.
func NewControlPlaneVMs(
	controlPlaneConfigs *configs.WorkloadControlPlane,
	stores ControlPlaneVMStores,
	natsConnection *nats.Conn,
	producer domain.Producer,
	logger *slog.Logger,
) (*ControlPlaneVMs, error) {
	vmRepository := stores.VMs
	snapshotRepository := stores.Snapshots
	nodeRepository := stores.Nodes
	taskRepository := stores.Tasks

	// the control plane answers the blog with the codes it refused a request
	// for, and the blog puts them into the words of whoever asked.
	codes := infraValidator.New(infraTranslator.Codes{})

	// a node is asked over core NATS and given as long as the operation needs:
	// one that may pull an image as long as the node may take over it, which
	// is the wait for the VM's dockerd and then the pull.
	requester := request.NewRequester(natsConnection, controlPlaneConfigs.NodeRequestTimeout, controlPlaneConfigs.PullRequestTimeout())

	dockerDefaults, err := dockerVMDefaults(controlPlaneConfigs)
	if err != nil {
		return nil, err
	}

	limits := quota.Limits{
		MinMemory:       controlPlaneConfigs.VMMinMemory,
		MinDisk:         controlPlaneConfigs.VMMinDisk,
		DockerMinMemory: controlPlaneConfigs.VMDockerMinMemory,
		DockerMinDisk:   controlPlaneConfigs.VMDockerMinDisk,
		MaxCPUs:         controlPlaneConfigs.VMMaxCPUs,
		MaxMemory:       controlPlaneConfigs.VMMaxMemory,
		MaxDisk:         controlPlaneConfigs.VMMaxDisk,
		UserMaxVMs:      controlPlaneConfigs.VMUserMaxVMs,
		UserCPUs:        controlPlaneConfigs.VMUserCPUs,
		UserMemory:      controlPlaneConfigs.VMUserMemory,
		UserDisk:        controlPlaneConfigs.VMUserDisk,
		MaxLifetime:     controlPlaneConfigs.VMMaxLifetime,
	}

	vmPlacement := placement.New(nodeRepository, vmRepository, controlPlaneConfigs.VMCPUOvercommit)
	vmQuota := quota.New(vmRepository, limits)
	commander := command.New(producer)
	vmLifecycle := lifecycle.New(vmRepository, stores.Children, nodeRepository, vmPlacement, commander)
	remover := archive.NewRemover(stores.Archives, logger)

	// the code runner's runs, shown among anybody's VMs: read from their tasks,
	// and stopped and taken away as their tasks are.
	runs := coderunner.New(taskRepository, producer, controlPlaneDeleteTask.NewUseCase(taskRepository, stores.TaskLogs, producer, infraTranslator.Codes{}))

	createVM := controlPlaneCreateVM.NewUseCase(vmRepository, taskRepository, snapshotRepository, vmQuota, vmLifecycle, codes, controlPlaneCreateVM.Images{
		Machine: controlPlaneConfigs.VMDefaultImage,
		Docker:  controlPlaneConfigs.VMDockerImage,
	})
	chooser := dockerVM.NewChooser(vmRepository, createVM, vmLifecycle, dockerDefaults)

	reconcile := controlPlaneVMReconcile.NewUseCase(vmRepository, nodeRepository, vmLifecycle, commander, logger)

	route := func(mux *http.ServeMux) {
		mux.Handle("GET /api/vms", controlPlaneVMAPI.NewIndexHandler(controlPlaneGetVMs.NewUseCase(vmRepository, runs)))
		mux.Handle("POST /api/vms", controlPlaneVMAPI.NewCreateHandler(createVM))
		mux.Handle("GET /api/vms/{uuid}", controlPlaneVMAPI.NewShowHandler(controlPlaneGetVM.NewUseCase(vmRepository, runs)))
		mux.Handle("PATCH /api/vms/{uuid}", controlPlaneVMAPI.NewUpdateHandler(controlPlaneUpdateVM.NewUseCase(vmRepository, runs, vmQuota, vmPlacement, vmLifecycle, codes)))
		mux.Handle("DELETE /api/vms/{uuid}", controlPlaneVMAPI.NewDeleteHandler(controlPlaneDeleteVM.NewUseCase(vmRepository, runs, vmLifecycle, codes)))
		mux.Handle("POST /api/vms/{uuid}/start", controlPlaneVMAPI.NewStartHandler(controlPlaneStartVM.NewUseCase(vmRepository, runs, vmLifecycle, codes)))
		mux.Handle("POST /api/vms/{uuid}/stop", controlPlaneVMAPI.NewStopHandler(controlPlaneStopVM.NewUseCase(vmRepository, runs, vmLifecycle, codes)))
		mux.Handle("POST /api/vms/{uuid}/restart", controlPlaneVMAPI.NewRestartHandler(controlPlaneRestartVM.NewUseCase(vmRepository, runs, vmLifecycle, codes)))
		mux.Handle("POST /api/vms/{uuid}/restore", controlPlaneVMAPI.NewRestoreHandler(controlPlaneRestoreVM.NewUseCase(vmRepository, runs, snapshotRepository, nodeRepository, vmLifecycle, commander, codes)))
		mux.Handle("GET /api/vms/{uuid}/logs", controlPlaneVMAPI.NewLogsHandler(controlPlaneGetVMLogs.NewUseCase(vmRepository, runs, requester, codes)))

		mux.Handle("GET /api/snapshots", controlPlaneSnapshotAPI.NewIndexHandler(controlPlaneGetSnapshots.NewUseCase(snapshotRepository)))
		mux.Handle("POST /api/vms/{uuid}/snapshots", controlPlaneSnapshotAPI.NewCreateHandler(controlPlaneCreateSnapshot.NewUseCase(vmRepository, runs, snapshotRepository, vmLifecycle, producer, codes, controlPlaneConfigs.SnapshotUserMax)))
		mux.Handle("GET /api/snapshots/{uuid}", controlPlaneSnapshotAPI.NewShowHandler(controlPlaneGetSnapshot.NewUseCase(snapshotRepository)))
		mux.Handle("PATCH /api/snapshots/{uuid}", controlPlaneSnapshotAPI.NewRenameHandler(controlPlaneRenameSnapshot.NewUseCase(snapshotRepository, codes)))
		mux.Handle("DELETE /api/snapshots/{uuid}", controlPlaneSnapshotAPI.NewDeleteHandler(controlPlaneDeleteSnapshot.NewUseCase(snapshotRepository, vmRepository, vmLifecycle, remover, codes)))

		mux.Handle("POST /api/vms/{uuid}/docker/{op}", controlPlaneDockerAPI.NewRequestHandler(controlPlaneRequestDocker.NewUseCase(vmRepository, requester, codes)))

		mux.Handle("GET /api/containers", controlPlaneContainerAPI.NewIndexHandler(controlPlaneGetContainers.NewUseCase(vmRepository, requester, logger)))
		mux.Handle("POST /api/containers", controlPlaneContainerAPI.NewCreateHandler(controlPlaneCreateContainer.NewUseCase(vmRepository, chooser, requester, codes)))
	}

	subscribers := map[string]domain.MessageHandler{
		vmEvents.VMHeartbeatName:             controlPlaneHeartbeatVM.NewHeartbeat(vmRepository, nodeRepository, vmLifecycle, commander, logger),
		vmEvents.VMFailedName:                controlPlaneFailVM.NewVMFailed(vmRepository, logger),
		vmEvents.VMRestoredName:              controlPlaneRestoreVM.NewVMRestored(vmRepository, vmLifecycle, commander, logger),
		vmEvents.VMDeletedName:               controlPlaneDeleteVM.NewVMDeleted(vmRepository, vmLifecycle, logger),
		snapshotEvents.SnapshotCompletedName: controlPlaneCreateSnapshot.NewSnapshotCompleted(snapshotRepository, remover, logger),
		snapshotEvents.SnapshotFailedName:    controlPlaneCreateSnapshot.NewSnapshotFailed(snapshotRepository, remover, logger),
	}

	return &ControlPlaneVMs{
		Route:       route,
		Subscribers: subscribers,
		Reconcile:   reconcile,
		VMs:         vmRepository,
		Chooser:     chooser,
		Parents:     parents.New(vmRepository),
	}, nil
}

// dockerVMDefaults is what a Docker VM made for a container or a stack is
// given, as the control plane was configured. Settings that cannot be what
// they say are refused here, when the control plane starts, rather than when
// somebody first adds a container.
func dockerVMDefaults(controlPlaneConfigs *configs.WorkloadControlPlane) (dockerVM.Defaults, error) {
	ports, err := controlPlaneConfigs.DockerDefaultPorts()
	if err != nil {
		return dockerVM.Defaults{}, err
	}

	network := vmContract.Network{
		Ingress: vmContract.Access(controlPlaneConfigs.VMDockerDefaultIngress),
		Egress:  vmContract.Access(controlPlaneConfigs.VMDockerDefaultEgress),
	}

	if !network.Ingress.IsValid() || !network.Egress.IsValid() {
		return dockerVM.Defaults{}, fmt.Errorf("a docker vm's default network is allow or deny each way, got ingress %q and egress %q", network.Ingress, network.Egress)
	}

	return dockerVM.Defaults{
		Resources: vmContract.Resources{
			CPUs:   controlPlaneConfigs.VMDockerDefaultCPUs,
			Memory: controlPlaneConfigs.VMDockerDefaultMemory,
			Disk:   controlPlaneConfigs.VMDockerDefaultDisk,
		},
		Ports:          ports,
		Network:        network,
		PersistentDisk: controlPlaneConfigs.VMDockerDefaultPersistentDisk,
		Lifetime:       controlPlaneConfigs.VMDockerDefaultLifetime,
	}, nil
}

// snapshotStore is the bucket snapshots' archives are kept in, reached the
// first time one is taken away so that the control plane starts whether or not
// the bucket can be reached. With no endpoint configured there is none, and an
// archive is left where it is when its snapshot goes.
func snapshotStore(storage configs.WorkloadSnapshotStorage) snapshotContract.Store {
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
