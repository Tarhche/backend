package client

import (
	"context"
	"errors"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// ErrNotImplemented is what everything the control plane does not answer yet
// returns: VMs, snapshots, Docker VMs and stacks are part of the contract
// before they are part of the control plane's API.
var ErrNotImplemented = errors.New("not implemented yet: the workload control plane does not answer this")

func (c *Client) VMs(ctx context.Context, ownerUUID string, kind vm.Kind, page uint) (workloadControlPlane.Page[vm.VM], error) {
	return workloadControlPlane.Page[vm.VM]{}, ErrNotImplemented
}

func (c *Client) VM(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error) {
	return vm.VM{}, ErrNotImplemented
}

func (c *Client) CreateVM(ctx context.Context, ownerUUID string, request workloadControlPlane.VMRequest) (vm.VM, error) {
	return vm.VM{}, ErrNotImplemented
}

func (c *Client) UpdateVM(ctx context.Context, ownerUUID string, uuid string, update workloadControlPlane.VMUpdate) (vm.VM, error) {
	return vm.VM{}, ErrNotImplemented
}

func (c *Client) DeleteVM(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

func (c *Client) StartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

func (c *Client) StopVM(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

func (c *Client) RestartVM(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

func (c *Client) RestoreVM(ctx context.Context, ownerUUID string, uuid string, snapshotUUID string) error {
	return ErrNotImplemented
}

func (c *Client) VMLogs(ctx context.Context, ownerUUID string, uuid string, options vm.LogOptions) ([]vm.LogLine, error) {
	return nil, ErrNotImplemented
}

func (c *Client) Snapshots(ctx context.Context, ownerUUID string, vmUUID string, page uint) (workloadControlPlane.Page[snapshot.Snapshot], error) {
	return workloadControlPlane.Page[snapshot.Snapshot]{}, ErrNotImplemented
}

func (c *Client) Snapshot(ctx context.Context, ownerUUID string, uuid string) (snapshot.Snapshot, error) {
	return snapshot.Snapshot{}, ErrNotImplemented
}

func (c *Client) CreateSnapshot(ctx context.Context, ownerUUID string, vmUUID string, name string) (snapshot.Snapshot, error) {
	return snapshot.Snapshot{}, ErrNotImplemented
}

func (c *Client) RenameSnapshot(ctx context.Context, ownerUUID string, uuid string, name string) (snapshot.Snapshot, error) {
	return snapshot.Snapshot{}, ErrNotImplemented
}

func (c *Client) DeleteSnapshot(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

func (c *Client) Docker(ownerUUID string, vmUUID string) docker.Daemon {
	return pendingDaemon{}
}

func (c *Client) CreateContainer(ctx context.Context, ownerUUID string, request workloadControlPlane.ContainerRequest) (workloadControlPlane.CreatedContainer, error) {
	return workloadControlPlane.CreatedContainer{}, ErrNotImplemented
}

func (c *Client) Containers(ctx context.Context, ownerUUID string, vmUUID string) ([]workloadControlPlane.VMContainer, error) {
	return nil, ErrNotImplemented
}

func (c *Client) Stacks(ctx context.Context, ownerUUID string, vmUUID string, page uint) (workloadControlPlane.Page[stack.Stack], error) {
	return workloadControlPlane.Page[stack.Stack]{}, ErrNotImplemented
}

func (c *Client) Stack(ctx context.Context, ownerUUID string, uuid string) (workloadControlPlane.StackDetail, error) {
	return workloadControlPlane.StackDetail{}, ErrNotImplemented
}

func (c *Client) CreateStack(ctx context.Context, ownerUUID string, request workloadControlPlane.StackRequest) (workloadControlPlane.CreatedStack, error) {
	return workloadControlPlane.CreatedStack{}, ErrNotImplemented
}

func (c *Client) DeleteStack(ctx context.Context, ownerUUID string, uuid string, removeVolumes bool) error {
	return ErrNotImplemented
}

func (c *Client) StartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

func (c *Client) StopStack(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

func (c *Client) RestartStack(ctx context.Context, ownerUUID string, uuid string) error {
	return ErrNotImplemented
}

// pendingDaemon is a Docker VM's dockerd as the control plane does not reach
// it yet: every call is refused the same way.
type pendingDaemon struct{}

var _ docker.Daemon = pendingDaemon{}

func (pendingDaemon) Ping(ctx context.Context) error {
	return ErrNotImplemented
}

func (pendingDaemon) Containers(ctx context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	return nil, ErrNotImplemented
}

func (pendingDaemon) Container(ctx context.Context, id string) (docker.Container, error) {
	return docker.Container{}, ErrNotImplemented
}

func (pendingDaemon) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (docker.Container, error) {
	return docker.Container{}, ErrNotImplemented
}

func (pendingDaemon) StartContainer(ctx context.Context, id string) error {
	return ErrNotImplemented
}

func (pendingDaemon) StopContainer(ctx context.Context, id string) error {
	return ErrNotImplemented
}

func (pendingDaemon) RestartContainer(ctx context.Context, id string) error {
	return ErrNotImplemented
}

func (pendingDaemon) RemoveContainer(ctx context.Context, id string, force bool) error {
	return ErrNotImplemented
}

func (pendingDaemon) ContainerLogs(ctx context.Context, id string, options docker.LogOptions) ([]docker.LogLine, error) {
	return nil, ErrNotImplemented
}

func (pendingDaemon) ContainerStats(ctx context.Context, id string) (docker.Stats, error) {
	return docker.Stats{}, ErrNotImplemented
}

func (pendingDaemon) ConnectNetwork(ctx context.Context, network string, container string, aliases []string) error {
	return ErrNotImplemented
}

func (pendingDaemon) DisconnectNetwork(ctx context.Context, network string, container string, force bool) error {
	return ErrNotImplemented
}

func (pendingDaemon) Images(ctx context.Context) ([]docker.Image, error) {
	return nil, ErrNotImplemented
}

func (pendingDaemon) PullImage(ctx context.Context, reference string) (docker.Image, error) {
	return docker.Image{}, ErrNotImplemented
}

func (pendingDaemon) RemoveImage(ctx context.Context, id string, force bool) error {
	return ErrNotImplemented
}

func (pendingDaemon) Networks(ctx context.Context) ([]docker.Network, error) {
	return nil, ErrNotImplemented
}

func (pendingDaemon) CreateNetwork(ctx context.Context, spec docker.NetworkSpec) (docker.Network, error) {
	return docker.Network{}, ErrNotImplemented
}

func (pendingDaemon) RemoveNetwork(ctx context.Context, id string) error {
	return ErrNotImplemented
}

func (pendingDaemon) Volumes(ctx context.Context) ([]docker.Volume, error) {
	return nil, ErrNotImplemented
}

func (pendingDaemon) CreateVolume(ctx context.Context, spec docker.VolumeSpec) (docker.Volume, error) {
	return docker.Volume{}, ErrNotImplemented
}

func (pendingDaemon) RemoveVolume(ctx context.Context, name string, force bool) error {
	return ErrNotImplemented
}
