package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Docker is the dockerd of a Docker VM, reached through the control plane:
// every call is one request to the node holding the VM, and the answer is
// read from dockerd as it is now.
func (c *Client) Docker(ownerUUID string, vmUUID string) docker.Daemon {
	return &daemon{client: c, ownerUUID: ownerUUID, vmUUID: vmUUID}
}

// daemon is one Docker VM's dockerd, as the control plane's passthrough
// reaches it: each method is one operation.
type daemon struct {
	client    *Client
	ownerUUID string
	vmUUID    string
}

var _ docker.Daemon = &daemon{}

// dockerResult is dockerd's answer, as the node gave it.
type dockerResult struct {
	Result    json.RawMessage `json:"result"`
	Truncated bool            `json:"truncated"`
}

// ask puts one operation to the VM's dockerd, with its payload, and decodes
// its result into out when there is one to decode.
func (d *daemon) ask(ctx context.Context, op noderequest.Op, payload any, out any) error {
	timeout := nodeRequestTimeout
	if op.MayPull() {
		timeout = pullRequestTimeout
	}

	endpoint := d.client.path(vmPath(d.vmUUID)+"/docker/"+strings.TrimPrefix(string(op), "docker."), owned(d.ownerUUID, nil))

	var answer dockerResult
	if err := d.client.callWithin(ctx, timeout, http.MethodPost, endpoint, payload, &answer); err != nil {
		return err
	}

	if out == nil || len(answer.Result) == 0 {
		return nil
	}

	return json.Unmarshal(answer.Result, out)
}

func (d *daemon) Ping(ctx context.Context) error {
	return d.ask(ctx, noderequest.OpPing, nil, nil)
}

func (d *daemon) Containers(ctx context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	var listed []noderequest.Container
	if err := d.ask(ctx, noderequest.OpContainersList, noderequest.NewContainersRequest(filter), &listed); err != nil {
		return nil, err
	}

	return containersOf(listed), nil
}

func (d *daemon) Container(ctx context.Context, id string) (docker.Container, error) {
	var inspected noderequest.Container
	if err := d.ask(ctx, noderequest.OpContainersInspect, noderequest.ContainerRequest{ID: id}, &inspected); err != nil {
		return docker.Container{}, err
	}

	return inspected.ToDocker(), nil
}

// CreateContainer pulls the image when the VM does not hold it, then creates
// the container and starts it.
func (d *daemon) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (docker.Container, error) {
	var created noderequest.Container
	if err := d.ask(ctx, noderequest.OpContainersCreate, noderequest.NewContainerSpec(spec), &created); err != nil {
		return docker.Container{}, err
	}

	return created.ToDocker(), nil
}

func (d *daemon) StartContainer(ctx context.Context, id string) error {
	return d.ask(ctx, noderequest.OpContainersStart, noderequest.ContainerRequest{ID: id}, nil)
}

func (d *daemon) StopContainer(ctx context.Context, id string) error {
	return d.ask(ctx, noderequest.OpContainersStop, noderequest.ContainerRequest{ID: id}, nil)
}

func (d *daemon) RestartContainer(ctx context.Context, id string) error {
	return d.ask(ctx, noderequest.OpContainersRestart, noderequest.ContainerRequest{ID: id}, nil)
}

func (d *daemon) RemoveContainer(ctx context.Context, id string, force bool) error {
	return d.ask(ctx, noderequest.OpContainersRemove, noderequest.RemoveRequest{ID: id, Force: force}, nil)
}

func (d *daemon) ContainerLogs(ctx context.Context, id string, options docker.LogOptions) ([]docker.LogLine, error) {
	var lines []noderequest.LogLine
	if err := d.ask(ctx, noderequest.OpContainersLogs, noderequest.NewContainerLogsRequest(id, options), &lines); err != nil {
		return nil, err
	}

	read := make([]docker.LogLine, len(lines))
	for i := range lines {
		read[i] = lines[i].ToDocker()
	}

	return read, nil
}

func (d *daemon) ContainerStats(ctx context.Context, id string) (docker.Stats, error) {
	var stats noderequest.Stats
	if err := d.ask(ctx, noderequest.OpContainersStats, noderequest.ContainerRequest{ID: id}, &stats); err != nil {
		return docker.Stats{}, err
	}

	return stats.ToDocker(), nil
}

func (d *daemon) ConnectNetwork(ctx context.Context, network string, container string, aliases []string) error {
	return d.ask(ctx, noderequest.OpContainersConnect, noderequest.ConnectRequest{Network: network, Container: container, Aliases: aliases}, nil)
}

func (d *daemon) DisconnectNetwork(ctx context.Context, network string, container string, force bool) error {
	return d.ask(ctx, noderequest.OpContainersDisconnect, noderequest.DisconnectRequest{Network: network, Container: container, Force: force}, nil)
}

func (d *daemon) Images(ctx context.Context) ([]docker.Image, error) {
	var listed []noderequest.Image
	if err := d.ask(ctx, noderequest.OpImagesList, nil, &listed); err != nil {
		return nil, err
	}

	images := make([]docker.Image, len(listed))
	for i := range listed {
		images[i] = listed[i].ToDocker()
	}

	return images, nil
}

func (d *daemon) PullImage(ctx context.Context, reference string) (docker.Image, error) {
	var pulled noderequest.Image
	if err := d.ask(ctx, noderequest.OpImagesPull, noderequest.PullRequest{Reference: reference}, &pulled); err != nil {
		return docker.Image{}, err
	}

	return pulled.ToDocker(), nil
}

func (d *daemon) RemoveImage(ctx context.Context, id string, force bool) error {
	return d.ask(ctx, noderequest.OpImagesRemove, noderequest.RemoveRequest{ID: id, Force: force}, nil)
}

func (d *daemon) Networks(ctx context.Context) ([]docker.Network, error) {
	var listed []noderequest.Network
	if err := d.ask(ctx, noderequest.OpNetworksList, nil, &listed); err != nil {
		return nil, err
	}

	networks := make([]docker.Network, len(listed))
	for i := range listed {
		networks[i] = listed[i].ToDocker()
	}

	return networks, nil
}

func (d *daemon) CreateNetwork(ctx context.Context, spec docker.NetworkSpec) (docker.Network, error) {
	var created noderequest.Network
	if err := d.ask(ctx, noderequest.OpNetworksCreate, noderequest.NewNetworkSpec(spec), &created); err != nil {
		return docker.Network{}, err
	}

	return created.ToDocker(), nil
}

func (d *daemon) RemoveNetwork(ctx context.Context, id string) error {
	return d.ask(ctx, noderequest.OpNetworksRemove, noderequest.RemoveRequest{ID: id}, nil)
}

func (d *daemon) Volumes(ctx context.Context) ([]docker.Volume, error) {
	var listed []noderequest.Volume
	if err := d.ask(ctx, noderequest.OpVolumesList, nil, &listed); err != nil {
		return nil, err
	}

	volumes := make([]docker.Volume, len(listed))
	for i := range listed {
		volumes[i] = listed[i].ToDocker()
	}

	return volumes, nil
}

func (d *daemon) CreateVolume(ctx context.Context, spec docker.VolumeSpec) (docker.Volume, error) {
	var created noderequest.Volume
	if err := d.ask(ctx, noderequest.OpVolumesCreate, noderequest.NewVolumeSpec(spec), &created); err != nil {
		return docker.Volume{}, err
	}

	return created.ToDocker(), nil
}

// RemoveVolume takes a volume away; a volume is named by its name, which is
// the only id it has.
func (d *daemon) RemoveVolume(ctx context.Context, name string, force bool) error {
	return d.ask(ctx, noderequest.OpVolumesRemove, noderequest.RemoveRequest{ID: name, Force: force}, nil)
}
