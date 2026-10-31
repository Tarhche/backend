package blocks

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// Timings is where how long each request to a Docker VM's dockerd took is
// recorded, by its operation: what the dashboards show of Docker VMs.
type Timings interface {
	DockerRequest(ctx context.Context, op string, took time.Duration)
}

// Timed are daemons whose every request is recorded into timings, by its
// operation, "docker.containers.create" say, and a VM's inventory as
// "docker.inventory". With no timings, they are daemons as they are.
func Timed(daemons Daemons, timings Timings) Daemons {
	if timings == nil {
		return daemons
	}

	return timedDaemons{daemons: daemons, timings: timings}
}

type timedDaemons struct {
	daemons Daemons
	timings Timings
}

func (t timedDaemons) Daemon(vmUUID string) docker.Daemon {
	return &timed{daemon: t.daemons.Daemon(vmUUID), timings: t.timings}
}

// timed is one Docker VM's dockerd, every request to it timed.
type timed struct {
	daemon  docker.Daemon
	timings Timings
}

var _ docker.Daemon = &timed{}

// took records a request of op started at started, once it is answered.
func (t *timed) took(ctx context.Context, op string, started time.Time) {
	t.timings.DockerRequest(ctx, op, time.Since(started))
}

func (t *timed) Ping(ctx context.Context) error {
	defer t.took(ctx, "docker.ping", time.Now())

	return t.daemon.Ping(ctx)
}

func (t *timed) Inventory(ctx context.Context) (docker.Inventory, error) {
	defer t.took(ctx, "docker.inventory", time.Now())

	return t.daemon.Inventory(ctx)
}

func (t *timed) Containers(ctx context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	defer t.took(ctx, "docker.containers.list", time.Now())

	return t.daemon.Containers(ctx, filter)
}

func (t *timed) Container(ctx context.Context, id string) (docker.Container, error) {
	defer t.took(ctx, "docker.containers.inspect", time.Now())

	return t.daemon.Container(ctx, id)
}

func (t *timed) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (docker.Container, error) {
	defer t.took(ctx, "docker.containers.create", time.Now())

	return t.daemon.CreateContainer(ctx, spec)
}

func (t *timed) StartContainer(ctx context.Context, id string) error {
	defer t.took(ctx, "docker.containers.start", time.Now())

	return t.daemon.StartContainer(ctx, id)
}

func (t *timed) StopContainer(ctx context.Context, id string) error {
	defer t.took(ctx, "docker.containers.stop", time.Now())

	return t.daemon.StopContainer(ctx, id)
}

func (t *timed) RestartContainer(ctx context.Context, id string) error {
	defer t.took(ctx, "docker.containers.restart", time.Now())

	return t.daemon.RestartContainer(ctx, id)
}

func (t *timed) RemoveContainer(ctx context.Context, id string, force bool) error {
	defer t.took(ctx, "docker.containers.remove", time.Now())

	return t.daemon.RemoveContainer(ctx, id, force)
}

func (t *timed) ContainerLogs(ctx context.Context, id string, options docker.LogOptions) ([]docker.LogLine, error) {
	defer t.took(ctx, "docker.containers.logs", time.Now())

	return t.daemon.ContainerLogs(ctx, id, options)
}

func (t *timed) ContainerStats(ctx context.Context, id string) (docker.Stats, error) {
	defer t.took(ctx, "docker.containers.stats", time.Now())

	return t.daemon.ContainerStats(ctx, id)
}

func (t *timed) ConnectNetwork(ctx context.Context, network string, container string, aliases []string) error {
	defer t.took(ctx, "docker.containers.connect", time.Now())

	return t.daemon.ConnectNetwork(ctx, network, container, aliases)
}

func (t *timed) DisconnectNetwork(ctx context.Context, network string, container string, force bool) error {
	defer t.took(ctx, "docker.containers.disconnect", time.Now())

	return t.daemon.DisconnectNetwork(ctx, network, container, force)
}

func (t *timed) Images(ctx context.Context) ([]docker.Image, error) {
	defer t.took(ctx, "docker.images.list", time.Now())

	return t.daemon.Images(ctx)
}

func (t *timed) PullImage(ctx context.Context, reference string) (docker.Image, error) {
	defer t.took(ctx, "docker.images.pull", time.Now())

	return t.daemon.PullImage(ctx, reference)
}

func (t *timed) RemoveImage(ctx context.Context, id string, force bool) error {
	defer t.took(ctx, "docker.images.remove", time.Now())

	return t.daemon.RemoveImage(ctx, id, force)
}

func (t *timed) Networks(ctx context.Context) ([]docker.Network, error) {
	defer t.took(ctx, "docker.networks.list", time.Now())

	return t.daemon.Networks(ctx)
}

func (t *timed) CreateNetwork(ctx context.Context, spec docker.NetworkSpec) (docker.Network, error) {
	defer t.took(ctx, "docker.networks.create", time.Now())

	return t.daemon.CreateNetwork(ctx, spec)
}

func (t *timed) RemoveNetwork(ctx context.Context, id string) error {
	defer t.took(ctx, "docker.networks.remove", time.Now())

	return t.daemon.RemoveNetwork(ctx, id)
}

func (t *timed) Volumes(ctx context.Context) ([]docker.Volume, error) {
	defer t.took(ctx, "docker.volumes.list", time.Now())

	return t.daemon.Volumes(ctx)
}

func (t *timed) CreateVolume(ctx context.Context, spec docker.VolumeSpec) (docker.Volume, error) {
	defer t.took(ctx, "docker.volumes.create", time.Now())

	return t.daemon.CreateVolume(ctx, spec)
}

func (t *timed) RemoveVolume(ctx context.Context, name string, force bool) error {
	defer t.took(ctx, "docker.volumes.remove", time.Now())

	return t.daemon.RemoveVolume(ctx, name, force)
}
