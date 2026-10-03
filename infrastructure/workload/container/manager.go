package container

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"time"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	infraNetwork "github.com/khanzadimahdi/testproject/infrastructure/workload/network"
)

var statusMap = map[string]task.Status{
	"created":    task.StatusCreated,
	"running":    task.StatusRunning,
	"paused":     task.StatusPaused,
	"restarting": task.StatusRestarting,
	"exited":     task.StatusExited,
	"removing":   task.StatusRemoving,
	"dead":       task.StatusDead,
}

const (
	readOperation  = "read"
	writeOperation = "write"

	// stopTimeout is how long a container is given to shut down on its own
	// before docker kills it.
	stopTimeout = 10
)

// DockerManager runs one class's tasks as containers on a docker daemon, for
// one node.
//
// It sees and touches only what is its own: containers labelled with its node's
// name, of its class. Several orchestrators share one daemon, and two classes
// may share one too, and without that each would list, stop and delete the
// others' containers as its own.
type DockerManager struct {
	client *client.Client
	logger *slog.Logger
	tracer oteltrace.Tracer

	// node is the orchestrator the containers are run for, and class what
	// they are run as.
	node  string
	class runtime.Class

	// ociRuntime is the OCI runtime the daemon is asked to run a container
	// with, such as sysbox-runc or runsc. Empty is the daemon's default.
	ociRuntime string

	// advertiseHost is where this node reaches the ports its containers are
	// published on. That is the docker daemon's own host, which is not always
	// this one.
	advertiseHost string

	// names is what the class calls the workload's networks on the daemon.
	names infraNetwork.Names
}

var _ task.Runtime = &DockerManager{}

// Scope is what a DockerManager runs its containers as, and for whom.
type Scope struct {
	// Node is the orchestrator the containers are run for. They are labelled
	// with it, and only containers labelled with it are seen.
	Node string

	// Class is what the containers are run as. Empty is sysbox, as every
	// container from before there were classes was.
	Class runtime.Class

	// OCIRuntime is what the daemon is asked to run them with; empty is the
	// daemon's own default.
	OCIRuntime string

	// AdvertiseHost is where the ports the daemon publishes are reached.
	AdvertiseHost string

	// Networks is what the class calls the workload's networks on the daemon.
	Networks infraNetwork.Names
}

// NewDockerManager runs containers for one node and class on the daemon cli
// reaches. The client is the class's own, shared with its networks, and is not
// closed here.
func NewDockerManager(cli *client.Client, scope Scope, logger *slog.Logger) *DockerManager {
	return &DockerManager{
		client:        cli,
		logger:        logger,
		tracer:        otel.Tracer("docker"),
		node:          scope.Node,
		class:         scope.Class.OrSysbox(),
		ociRuntime:    scope.OCIRuntime,
		advertiseHost: scope.AdvertiseHost,
		names:         scope.Networks,
	}
}

// OnNode is every container of this class the named node is holding.
func (m *DockerManager) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	return m.byLabel(ctx, label(NodeNameLabel, nodeName))
}

// Of is the containers running one task on this node, latest attempt and
// whatever is left of the ones before it.
//
// Only this node's: another orchestrator on the same daemon may hold the same
// task's earlier attempts, and those are its to deal with, since every node
// hears every command.
func (m *DockerManager) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	return m.byLabel(ctx, label(taskUUIDLabel, taskUUID), label(NodeNameLabel, m.node))
}

// BySlug is the containers on this node answering to the name a task's ports
// are served under.
func (m *DockerManager) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	return m.byLabel(ctx, label(taskSlugLabel, slug), label(NodeNameLabel, m.node))
}

// label is a docker filter on one label's value.
func label(key string, value string) string {
	return key + "=" + value
}

// byLabel is how docker is asked all three of those: what a container is
// running is written on it, so looking one up is looking at what it says.
//
// A container of another class is left out. Docker can only be asked for a
// label that is there, and sysbox's containers from before there were classes
// carry none, so what docker answers is sorted here; another class's are
// asked for by name, which spares listing what would be left out.
func (m *DockerManager) byLabel(ctx context.Context, labels ...string) ([]task.Execution, error) {
	ctx, span := m.tracer.Start(ctx, "docker.task.list",
		oteltrace.WithAttributes(attribute.StringSlice("labels", labels)),
	)
	defer span.End()

	filter := filters.NewArgs()
	for _, l := range labels {
		filter.Add("label", l)
	}

	if m.class != runtime.Sysbox {
		filter.Add("label", label(taskRuntimeLabel, string(m.class)))
	}

	containers, err := m.client.ContainerList(ctx, containerTypes.ListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	result := make([]task.Execution, 0, len(containers))
	for _, c := range containers {
		if classOf(c.Labels) != m.class {
			continue
		}

		execution := task.Execution{
			ID:           c.ID,
			Name:         c.Names[0],
			Status:       convertToContainerStatus(c.State),
			Image:        c.Image,
			CreatedAt:    time.Unix(c.Created, 0),
			ExposedPorts: convertDockerPortSet(c.Ports),
			PortBindings: convertDockerPortMap(c.Ports),
			Endpoints:    listedEndpoints(c.Ports),
		}

		identify(&execution, c.Labels)

		result = append(result, execution)
	}

	return result, nil
}

// EnsureImage makes sure an image is on this node, pulling it if it is not.
func (m *DockerManager) EnsureImage(ctx context.Context, reference string) error {
	ctx, span := m.tracer.Start(ctx, "docker.image.ensure",
		oteltrace.WithAttributes(attribute.String("image", reference)),
	)
	defer span.End()

	images, err := m.client.ImageList(ctx, image.ListOptions{
		All:     false,
		Filters: filters.NewArgs(filters.Arg("reference", reference)),
	})
	if err != nil {
		return trace.RecordError(span, err)
	}

	if len(images) > 0 {
		return nil
	}

	m.logger.Info("image does not exist, start pulling", "image", reference)

	if err := m.pullImage(ctx, reference); err != nil {
		return trace.RecordError(span, err)
	}

	m.logger.Info("image pulled", "image", reference)

	return nil
}

func (m *DockerManager) Create(ctx context.Context, c *task.Execution) (string, error) {
	ctx, span := m.tracer.Start(ctx, "docker.task.create",
		oteltrace.WithAttributes(attribute.String("image", c.Image), attribute.String("name", c.Name)),
	)
	defer span.End()

	if err := m.EnsureImage(ctx, c.Image); err != nil {
		return "", trace.RecordError(span, err)
	}

	labels := labelsOf(c, m.class)

	// the run is this node's, whatever it was asked to be: this node is the
	// only one that will ever list it again.
	if len(m.node) > 0 {
		labels[NodeNameLabel] = m.node
	}

	config := &containerTypes.Config{
		Image:        c.Image,
		Cmd:          c.Command,
		Env:          c.Environment,
		Labels:       labels,
		ExposedPorts: convertPortSet(c.ExposedPorts),
		WorkingDir:   c.WorkingDirectory,
		Entrypoint:   c.Entrypoint,
	}

	hostConfig := &containerTypes.HostConfig{
		// memory is in bytes here as it is everywhere else, and a cpu is
		// counted in billionths of one.
		//
		// The disk limit every task has to name is not applied. Docker can
		// only hold a container to one with storage-opt size=, which needs
		// overlay2 on an XFS filesystem mounted with project quotas, and the
		// dind daemon the workload runs on has neither. Until a runtime can say
		// that it holds one, a task's disk limit is what it said it needs
		// rather than something it is held to.
		Memory:   memoryLimit(c.ResourceLimits.Memory),
		NanoCPUs: int64(c.ResourceLimits.Cpu * 1e9),
		RestartPolicy: containerTypes.RestartPolicy{
			Name: containerTypes.RestartPolicyMode(c.RestartPolicy),
		},
		PortBindings: convertPortMap(c.PortBindings),
		AutoRemove:   c.AutoRemove,

		// an immutable container writes only to what is mounted into it.
		ReadonlyRootfs: c.ReadOnly,
		NetworkMode:    networkMode(c.Networks, m.names),

		// the OCI runtime is what makes a class out of a daemon: the same
		// daemon runs sysbox's containers under one and gvisor's under
		// another. Empty is the daemon's own default.
		Runtime: m.ociRuntime,
	}

	m.logger.Info("creating container", "name", c.Name, "networks", c.Networks)
	resp, err := m.client.ContainerCreate(ctx, config, hostConfig, endpointsConfig(c.Networks, m.names), nil, c.Name)
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	// a limit docker cannot apply — memory on a daemon whose cgroups have no
	// memory controller, say — is dropped, and this is the only place it says
	// so. Written down, so a task running without a limit it asked for does
	// not go unnoticed.
	if len(resp.Warnings) > 0 {
		m.logger.Warn("container created with warnings", "name", c.Name, "containerID", resp.ID, "warnings", resp.Warnings)
	}

	// a container that reaches both its own stack and the internet sits on two
	// networks, and docker only takes one of them at create time.
	if err := m.connectRemainingNetworks(ctx, resp.ID, c.Networks); err != nil {
		return "", trace.RecordError(span, err)
	}

	m.logger.Info("container created", "image", c.Image, "containerID", resp.ID)

	return resp.ID, nil
}

// pullImage pulls image and waits for the pull to complete, in its own span
// since a cold pull is by far the most variable-latency step of Create.
func (m *DockerManager) pullImage(ctx context.Context, imageName string) error {
	ctx, span := m.tracer.Start(ctx, "docker.image.pull", oteltrace.WithAttributes(attribute.String("image", imageName)))
	defer span.End()

	out, err := m.client.ImagePull(ctx, imageName, image.PullOptions{All: false})
	if err != nil {
		return trace.RecordError(span, err)
	}
	defer out.Close()

	_, err = io.Copy(io.Discard, out)

	return trace.RecordError(span, err)
}

func (m *DockerManager) Start(ctx context.Context, containerUUID string) error {
	ctx, span := m.tracer.Start(ctx, "docker.task.start",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	m.logger.Info("starting container", "containerUUID", containerUUID)
	err := m.client.ContainerStart(ctx, containerUUID, containerTypes.StartOptions{})

	return trace.RecordError(span, err)
}

// gone turns docker's own "no such container" into the domain's way of saying
// it, so that a command for a container that is not there any more reads as
// already done rather than as a failure worth trying again.
func gone(err error) error {
	if client.IsErrNotFound(err) {
		return domain.ErrNotExists
	}

	return err
}

func (m *DockerManager) Stop(ctx context.Context, containerUUID string) error {
	ctx, span := m.tracer.Start(ctx, "docker.task.stop",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	timeout := stopTimeout
	err := m.client.ContainerStop(ctx, containerUUID, containerTypes.StopOptions{
		Timeout: &timeout,
	})

	return trace.RecordError(span, gone(err))
}

// Restart stops the container and starts it again. The container keeps its
// identity, so its logs, its published ports and its name all survive.
func (m *DockerManager) Restart(ctx context.Context, containerUUID string) error {
	ctx, span := m.tracer.Start(ctx, "docker.task.restart",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	timeout := stopTimeout
	err := m.client.ContainerRestart(ctx, containerUUID, containerTypes.StopOptions{
		Timeout: &timeout,
	})

	return trace.RecordError(span, gone(err))
}

// Kill stops the container at once, without the grace period Stop gives it.
func (m *DockerManager) Kill(ctx context.Context, containerUUID string) error {
	ctx, span := m.tracer.Start(ctx, "docker.task.kill",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	err := m.client.ContainerKill(ctx, containerUUID, "SIGKILL")

	return trace.RecordError(span, gone(err))
}

func (m *DockerManager) Delete(ctx context.Context, containerUUID string) error {
	ctx, span := m.tracer.Start(ctx, "docker.task.delete",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	err := m.client.ContainerRemove(ctx, containerUUID, containerTypes.RemoveOptions{
		Force: true,
	})

	return trace.RecordError(span, gone(err))
}

func (m *DockerManager) Inspect(ctx context.Context, containerUUID string) (task.Execution, error) {
	ctx, span := m.tracer.Start(ctx, "docker.task.inspect",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	info, err := m.client.ContainerInspect(ctx, containerUUID)
	if err != nil {
		return task.Execution{}, trace.RecordError(span, err)
	}

	created, err := time.Parse(time.RFC3339Nano, info.Created)
	if err != nil {
		return task.Execution{}, trace.RecordError(span, err)
	}

	// a container that has never run has no start to report, which docker says
	// with a zero time rather than an error.
	started, _ := time.Parse(time.RFC3339Nano, info.State.StartedAt)

	execution := task.Execution{
		ID:               info.ID,
		Name:             info.Name,
		Status:           convertToContainerStatus(info.State.Status),
		Image:            info.Config.Image,
		Environment:      info.Config.Env,
		Command:          info.Config.Cmd,
		Entrypoint:       info.Config.Entrypoint,
		WorkingDirectory: info.Config.WorkingDir,
		ReadOnly:         info.HostConfig.ReadonlyRootfs,
		RestartPolicy:    string(info.HostConfig.RestartPolicy.Name),
		RestartCount:     uint(info.RestartCount),
		CreatedAt:        created,
		StartedAt:        started,
		ExitCode:         info.State.ExitCode,
		ExposedPorts:     convertDockerPortSetFromMap(info.NetworkSettings.Ports),
		PortBindings:     convertDockerPortMapFromMap(info.NetworkSettings.Ports),
		Endpoints:        inspectedEndpoints(info.NetworkSettings.Ports),
		// read back in the units they were given in, so a task's limits are the
		// same number on the way in and on the way out. There is no disk limit
		// to read: docker was never given one.
		ResourceLimits: task.ResourceLimits{
			Memory: uint64(info.HostConfig.Resources.Memory),
			Cpu:    float64(info.HostConfig.Resources.NanoCPUs) / 1e9,
		},
		AutoRemove: info.HostConfig.AutoRemove,
		Networks:   inspectedNetworks(info.NetworkSettings, m.names),
	}

	identify(&execution, info.Config.Labels)

	return execution, nil
}

func (m *DockerManager) Stats(ctx context.Context, containerUUID string) (task.Stats, error) {
	ctx, span := m.tracer.Start(ctx, "docker.task.stats",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	dockerStats, err := m.client.ContainerStats(ctx, containerUUID, false)
	if err != nil {
		return task.Stats{}, trace.RecordError(span, err)
	}
	defer dockerStats.Body.Close()

	var v containerTypes.StatsResponse
	if err := json.NewDecoder(dockerStats.Body).Decode(&v); err != nil {
		return task.Stats{}, trace.RecordError(span, err)
	}

	memoryUsage := v.MemoryStats.Usage
	memoryLimit := v.MemoryStats.Limit

	var memoryPercent float64
	if memoryLimit > 0 {
		memoryPercent = float64(memoryUsage) / float64(memoryLimit) * 100.0
	}

	var netIn, netOut uint64
	for _, n := range v.Networks {
		netIn += n.RxBytes
		netOut += n.TxBytes
	}

	var blockIn, blockOut uint64
	for _, entry := range v.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(entry.Op) {
		case readOperation:
			blockIn += entry.Value
		case writeOperation:
			blockOut += entry.Value
		}
	}

	var cpuPercent float64
	cpuDelta := v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage
	systemDelta := v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage
	if systemDelta != 0 && cpuDelta != 0 {
		onlineCPUs := float64(v.CPUStats.OnlineCPUs)
		if onlineCPUs == 0 {
			onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
		}

		cpuPercent = float64(cpuDelta) / float64(systemDelta) * onlineCPUs * 100.0
	}

	return task.Stats{
		PIDs:          v.PidsStats.Current,
		CPUPercent:    cpuPercent,
		MemoryUsage:   memoryUsage,
		MemoryLimit:   memoryLimit,
		MemoryPercent: memoryPercent,
		NetworkInput:  netIn,
		NetworkOutput: netOut,
		BlockInput:    blockIn,
		BlockOutput:   blockOut,
	}, nil
}

func (m *DockerManager) Logs(ctx context.Context, containerUUID string, writer io.Writer) error {
	ctx, span := m.tracer.Start(ctx, "docker.task.logs",
		oteltrace.WithAttributes(attribute.String("task.id", containerUUID)),
	)
	defer span.End()

	m.logger.Info("getting logs for container", "containerUUID", containerUUID)
	readCloser, err := m.client.ContainerLogs(
		ctx,
		containerUUID, containerTypes.LogsOptions{
			Follow:     false,
			ShowStdout: true,
			ShowStderr: true,
		},
	)
	if err != nil {
		return trace.RecordError(span, err)
	}
	defer readCloser.Close()

	m.logger.Info("got the logs for container", "containerUUID", containerUUID)

	_, err = stdcopy.StdCopy(writer, writer, readCloser)

	return trace.RecordError(span, err)
}

func convertToContainerStatus(status string) task.Status {
	return statusMap[status]
}
