package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

var statusMap = map[string]container.Status{
	"created":    container.StatusCreated,
	"running":    container.StatusRunning,
	"paused":     container.StatusPaused,
	"restarting": container.StatusRestarting,
	"exited":     container.StatusExited,
	"removing":   container.StatusRemoving,
	"dead":       container.StatusDead,
}

const (
	readOperation  = "read"
	writeOperation = "write"
)

type DockerManager struct {
	client *client.Client
	logger *slog.Logger
	tracer oteltrace.Tracer
}

var _ container.Manager = &DockerManager{}

func NewDockerManager(dockerHost string, logger *slog.Logger) (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerManager{client: cli, logger: logger, tracer: otel.Tracer("docker")}, nil
}

func (m *DockerManager) GetAll(ctx context.Context) ([]container.Container, error) {
	ctx, span := m.tracer.Start(ctx, "docker.container.list")
	defer span.End()

	containers, err := m.client.ContainerList(ctx, containerTypes.ListOptions{All: true})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	result := make([]container.Container, len(containers))
	for i, c := range containers {
		result[i] = container.Container{
			ID:           c.ID,
			Name:         c.Names[0],
			Status:       convertToContainerStatus(c.State),
			Image:        c.Image,
			Labels:       c.Labels,
			CreatedAt:    time.Unix(c.Created, 0),
			ExposedPorts: convertDockerPortSet(c.Ports),
			PortBindings: convertDockerPortMap(c.Ports),
		}
	}

	return result, nil
}

func (m *DockerManager) GetByLabel(ctx context.Context, labelName string, labelValue string) ([]container.Container, error) {
	ctx, span := m.tracer.Start(ctx, "docker.container.list",
		oteltrace.WithAttributes(attribute.String("label", labelName+"="+labelValue)),
	)
	defer span.End()

	filter := filters.NewArgs()
	filter.Add("label", fmt.Sprintf("%s=%s", labelName, labelValue))

	containers, err := m.client.ContainerList(ctx, containerTypes.ListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	result := make([]container.Container, len(containers))
	for i, c := range containers {
		result[i] = container.Container{
			ID:           c.ID,
			Name:         c.Names[0],
			Status:       convertToContainerStatus(c.State),
			Image:        c.Image,
			Labels:       c.Labels,
			CreatedAt:    time.Unix(c.Created, 0),
			ExposedPorts: convertDockerPortSet(c.Ports),
			PortBindings: convertDockerPortMap(c.Ports),
		}
	}

	return result, nil
}

func (m *DockerManager) Create(ctx context.Context, c *container.Container) (string, error) {
	ctx, span := m.tracer.Start(ctx, "docker.container.create",
		oteltrace.WithAttributes(attribute.String("image", c.Image), attribute.String("name", c.Name)),
	)
	defer span.End()

	// check if image exists
	m.logger.Info("checking if image exists", "image", c.Image)
	images, err := m.client.ImageList(ctx, image.ListOptions{
		All:     false,
		Filters: filters.NewArgs(filters.Arg("reference", c.Image)),
	})
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	m.logger.Info("image existence checked", "exists", len(images) > 0)

	if len(images) == 0 {
		m.logger.Info("image does not exist, start pulling", "image", c.Image)

		if err := m.pullImage(ctx, c.Image); err != nil {
			return "", trace.RecordError(span, err)
		}

		m.logger.Info("image pulled", "image", c.Image)
	}

	config := &containerTypes.Config{
		Image:        c.Image,
		Cmd:          c.Command,
		Env:          c.Environment,
		Labels:       c.Labels,
		ExposedPorts: convertPortSet(c.ExposedPorts),
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
		PortBindings: convertPortMap(c.PortBindings),
		AutoRemove:   c.AutoRemove,
	}

	m.logger.Info("creating container", "name", c.Name)
	resp, err := m.client.ContainerCreate(ctx, config, hostConfig, nil, nil, c.Name)
	if err != nil {
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
	ctx, span := m.tracer.Start(ctx, "docker.container.start",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	m.logger.Info("starting container", "containerUUID", containerUUID)
	err := m.client.ContainerStart(ctx, containerUUID, containerTypes.StartOptions{})

	return trace.RecordError(span, err)
}

func (m *DockerManager) Stop(ctx context.Context, containerUUID string) error {
	ctx, span := m.tracer.Start(ctx, "docker.container.stop",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	timeout := 10
	err := m.client.ContainerStop(ctx, containerUUID, containerTypes.StopOptions{
		Timeout: &timeout,
	})

	return trace.RecordError(span, err)
}

func (m *DockerManager) Delete(ctx context.Context, containerUUID string) error {
	ctx, span := m.tracer.Start(ctx, "docker.container.delete",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	err := m.client.ContainerRemove(ctx, containerUUID, containerTypes.RemoveOptions{
		Force: true,
	})

	return trace.RecordError(span, err)
}

func (m *DockerManager) Inspect(ctx context.Context, containerUUID string) (container.Container, error) {
	ctx, span := m.tracer.Start(ctx, "docker.container.inspect",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	info, err := m.client.ContainerInspect(ctx, containerUUID)
	if err != nil {
		return container.Container{}, trace.RecordError(span, err)
	}

	created, err := time.Parse(time.RFC3339Nano, info.Created)
	if err != nil {
		return container.Container{}, trace.RecordError(span, err)
	}

	return container.Container{
		ID:               info.ID,
		Name:             info.Name,
		Status:           convertToContainerStatus(info.State.Status),
		Image:            info.Config.Image,
		Labels:           info.Config.Labels,
		Environment:      info.Config.Env,
		Command:          info.Config.Cmd,
		Entrypoint:       info.Config.Entrypoint,
		WorkingDirectory: info.Config.WorkingDir,
		RestartPolicy:    string(info.HostConfig.RestartPolicy.Name),
		RestartCount:     uint(info.RestartCount),
		CreatedAt:        created,
		ExposedPorts:     convertDockerPortSetFromMap(info.NetworkSettings.Ports),
		PortBindings:     convertDockerPortMapFromMap(info.NetworkSettings.Ports),
		ResourceLimits: container.ResourceLimits{
			Memory: uint64(info.HostConfig.Resources.Memory),
			Cpu:    float64(info.HostConfig.Resources.NanoCPUs) / 1e9,
		},
		AutoRemove: info.HostConfig.AutoRemove,
	}, nil
}

func (m *DockerManager) Stats(ctx context.Context, containerUUID string) (container.Stats, error) {
	ctx, span := m.tracer.Start(ctx, "docker.container.stats",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	dockerStats, err := m.client.ContainerStats(ctx, containerUUID, false)
	if err != nil {
		return container.Stats{}, trace.RecordError(span, err)
	}
	defer dockerStats.Body.Close()

	var v containerTypes.StatsResponse
	if err := json.NewDecoder(dockerStats.Body).Decode(&v); err != nil {
		return container.Stats{}, trace.RecordError(span, err)
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

	return container.Stats{
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
	ctx, span := m.tracer.Start(ctx, "docker.container.logs",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
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

func (m *DockerManager) EvaluateTaskState(status container.Status) task.State {
	switch status {
	case container.StatusCreated:
		return task.Scheduled
	case container.StatusRunning:
		return task.Running
	case container.StatusRestarting:
		return task.Stopping
	case container.StatusPaused:
		return task.Stopped
	case container.StatusDead:
		return task.Failed
	case container.StatusExited, container.StatusRemoving:
		return task.Completed
	default:
		return task.Failed
	}
}

func convertPortSet(ports port.PortSet) nat.PortSet {
	result := make(nat.PortSet)
	for p := range ports {
		result[nat.Port(fmt.Sprintf("%d/tcp", p))] = struct{}{}
	}
	return result
}

func convertPortMap(bindings port.PortMap) nat.PortMap {
	result := make(nat.PortMap)
	for p, bindings := range bindings {
		portStr := fmt.Sprintf("%d/tcp", p)
		result[nat.Port(portStr)] = make([]nat.PortBinding, len(bindings))
		for i, b := range bindings {
			result[nat.Port(portStr)][i] = nat.PortBinding{
				HostIP:   b.HostIP,
				HostPort: fmt.Sprintf("%d", b.HostPort),
			}
		}
	}
	return result
}

func convertDockerPortSet(ports []types.Port) port.PortSet {
	result := make(port.PortSet)
	for _, p := range ports {
		result[port.Port(p.PrivatePort)] = struct{}{}
	}
	return result
}

func convertDockerPortMap(ports []types.Port) port.PortMap {
	result := make(port.PortMap)
	for _, p := range ports {
		if p.PublicPort != 0 {
			result[port.Port(p.PrivatePort)] = []port.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: port.Port(p.PublicPort),
				},
			}
		}
	}
	return result
}

func convertDockerPortSetFromMap(ports nat.PortMap) port.PortSet {
	result := make(port.PortSet)
	for p := range ports {
		var portNum port.Port
		fmt.Sscanf(string(p), "%d/tcp", &portNum)
		result[portNum] = struct{}{}
	}
	return result
}

func convertDockerPortMapFromMap(ports nat.PortMap) port.PortMap {
	result := make(port.PortMap)
	for p, bindings := range ports {
		var portNum port.Port
		fmt.Sscanf(string(p), "%d/tcp", &portNum)
		result[portNum] = make([]port.PortBinding, len(bindings))
		for i, b := range bindings {
			var hostPort port.Port
			fmt.Sscanf(b.HostPort, "%d", &hostPort)
			result[portNum][i] = port.PortBinding{
				HostIP:   b.HostIP,
				HostPort: hostPort,
			}
		}
	}
	return result
}

func convertToContainerStatus(status string) container.Status {
	return statusMap[status]
}
