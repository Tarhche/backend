package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
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
}

var _ container.Manager = &DockerManager{}

func NewDockerManager(dockerHost string) (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerManager{client: cli}, nil
}

func (m *DockerManager) GetAll() ([]container.Container, error) {
	containers, err := m.client.ContainerList(context.Background(), containerTypes.ListOptions{All: true})
	if err != nil {
		return nil, err
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

func (m *DockerManager) GetByLabel(labelName string, labelValue string) ([]container.Container, error) {
	filter := filters.NewArgs()
	filter.Add("label", fmt.Sprintf("%s=%s", labelName, labelValue))

	containers, err := m.client.ContainerList(context.Background(), containerTypes.ListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return nil, err
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

func (m *DockerManager) Create(c *container.Container) (string, error) {
	// check if image exists
	log.Println("checking if image exists", c.Image, time.Now().Format(time.UnixDate))
	images, err := m.client.ImageList(context.Background(), image.ListOptions{
		All:     false,
		Filters: filters.NewArgs(filters.Arg("reference", c.Image)),
	})
	if err != nil {
		return "", err
	}

	log.Println("image exists", len(images) > 0, time.Now().Format(time.UnixDate))

	if len(images) == 0 {
		log.Println("image does not exist, start pulling", c.Image, time.Now().Format(time.UnixDate))

		_, err := m.client.ImagePull(context.Background(), c.Image, image.PullOptions{All: false})
		if err != nil {
			return "", err
		}

		log.Println("image pulled", c.Image, time.Now().Format(time.UnixDate))
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

	log.Println("creating container", c.Name, time.Now().Format(time.UnixDate))
	resp, err := m.client.ContainerCreate(context.Background(), config, hostConfig, nil, nil, c.Name)
	if err != nil {
		return "", err
	}
	log.Println("container created", c.Image, resp.ID, time.Now().Format(time.UnixDate))

	return resp.ID, nil
}

func (m *DockerManager) Start(containerUUID string) error {
	log.Println("starting container", containerUUID, time.Now().Format(time.UnixDate))
	err := m.client.ContainerStart(context.Background(), containerUUID, containerTypes.StartOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (m *DockerManager) Stop(containerUUID string) error {
	timeout := 10
	err := m.client.ContainerStop(context.Background(), containerUUID, containerTypes.StopOptions{
		Timeout: &timeout,
	})
	if err != nil {
		return err
	}
	return nil
}

func (m *DockerManager) Delete(containerUUID string) error {
	err := m.client.ContainerRemove(context.Background(), containerUUID, containerTypes.RemoveOptions{
		Force: true,
	})
	if err != nil {
		return err
	}

	return nil
}

func (m *DockerManager) Inspect(containerUUID string) (container.Container, error) {
	info, err := m.client.ContainerInspect(context.Background(), containerUUID)
	if err != nil {
		return container.Container{}, err
	}

	created, err := time.Parse(time.RFC3339Nano, info.Created)
	if err != nil {
		return container.Container{}, err
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

func (m *DockerManager) Stats(containerUUID string) (container.Stats, error) {
	dockerStats, err := m.client.ContainerStats(context.Background(), containerUUID, false)
	if err != nil {
		return container.Stats{}, err
	}
	defer dockerStats.Body.Close()

	var v containerTypes.StatsResponse
	if err := json.NewDecoder(dockerStats.Body).Decode(&v); err != nil {
		return container.Stats{}, err
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

func (m *DockerManager) Logs(containerUUID string, writer io.Writer) error {
	log.Println("getting logs for container", containerUUID, time.Now().Format(time.UnixDate))
	readCloser, err := m.client.ContainerLogs(
		context.Background(),
		containerUUID, containerTypes.LogsOptions{
			Follow:     false,
			ShowStdout: true,
			ShowStderr: true,
		},
	)
	if err != nil {
		return err
	}
	defer readCloser.Close()

	log.Println("got the logs for container", containerUUID, time.Now().Format(time.UnixDate))

	_, err = stdcopy.StdCopy(writer, writer, readCloser)

	return err
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
