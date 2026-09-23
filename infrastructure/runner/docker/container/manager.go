package container

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
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

type DockerManager struct {
	client *client.Client
	logger *slog.Logger
	tracer oteltrace.Tracer

	// advertiseHost is where this node reaches the ports its containers are
	// published on. That is the docker daemon's own host, which is not always
	// this one.
	advertiseHost string
}

var _ task.Runtime = &DockerManager{}

func NewDockerManager(dockerHost string, advertiseHost string, logger *slog.Logger) (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerManager{
		client:        cli,
		logger:        logger,
		tracer:        otel.Tracer("docker"),
		advertiseHost: advertiseHost,
	}, nil
}

// OnNode is every container this node is holding.
func (m *DockerManager) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	return m.byLabel(ctx, NodeNameLabel, nodeName)
}

// Of is the containers running one task, latest attempt and whatever is left
// of the ones before it.
func (m *DockerManager) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	return m.byLabel(ctx, taskUUIDLabel, taskUUID)
}

// BySlug is the containers answering to the name a task's ports are served
// under.
func (m *DockerManager) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	return m.byLabel(ctx, taskSlugLabel, slug)
}

// byLabel is how docker is asked all three of those: what a container is
// running is written on it, so looking one up is looking at what it says.
func (m *DockerManager) byLabel(ctx context.Context, label string, value string) ([]task.Execution, error) {
	ctx, span := m.tracer.Start(ctx, "docker.task.list",
		oteltrace.WithAttributes(attribute.String("label", label+"="+value)),
	)
	defer span.End()

	filter := filters.NewArgs()
	filter.Add("label", fmt.Sprintf("%s=%s", label, value))

	containers, err := m.client.ContainerList(ctx, containerTypes.ListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	result := make([]task.Execution, len(containers))
	for i, c := range containers {
		result[i] = task.Execution{
			ID:           c.ID,
			Name:         c.Names[0],
			Status:       convertToContainerStatus(c.State),
			Image:        c.Image,
			CreatedAt:    time.Unix(c.Created, 0),
			ExposedPorts: listedExposedPorts(c.Ports),
			Endpoints:    listedEndpoints(c.Ports),
		}

		identify(&result[i], c.Labels)
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

	config := &containerTypes.Config{
		Image:        c.Image,
		Cmd:          c.Command,
		Env:          c.Environment,
		Labels:       labelsOf(c),
		ExposedPorts: exposedPorts(c.ExposedPorts),
		WorkingDir:   c.WorkingDirectory,
		Entrypoint:   c.Entrypoint,
	}

	hostConfig := &containerTypes.HostConfig{
		Resources: containerTypes.Resources{
			Memory:   int64(c.ResourceLimits.Memory * 1024 * 1024),
			NanoCPUs: int64(c.ResourceLimits.Cpu * 1e9),
		},
		RestartPolicy: containerTypes.RestartPolicy{
			Name: containerTypes.RestartPolicyMode(c.RestartPolicy),
		},
		PortBindings: publishAll(c.ExposedPorts),
		AutoRemove:   c.AutoRemove,

		// an immutable container writes only to what is mounted into it.
		ReadonlyRootfs: c.ReadOnly,
		NetworkMode:    networkMode(c.Networks),
	}

	m.logger.Info("creating container", "name", c.Name, "networks", c.Networks)
	resp, err := m.client.ContainerCreate(ctx, config, hostConfig, endpointsConfig(c.Networks), nil, c.Name)
	if err != nil {
		return "", trace.RecordError(span, err)
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
		ExposedPorts:     inspectedExposedPorts(info.NetworkSettings.Ports),
		Endpoints:        inspectedEndpoints(info.NetworkSettings.Ports),
		ResourceLimits: task.ResourceLimits{
			Memory: uint64(info.HostConfig.Resources.Memory),
			Cpu:    float64(info.HostConfig.Resources.NanoCPUs) / 1e9,
		},
		AutoRemove: info.HostConfig.AutoRemove,
		Networks:   inspectedNetworks(info.NetworkSettings),
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
