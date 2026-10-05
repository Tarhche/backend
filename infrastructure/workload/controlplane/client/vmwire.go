package client

import (
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// the shapes the control plane's API speaks about VMs, snapshots, containers
// and stacks. What a node reports about dockerd's objects travels in the node
// requests' own shapes, which are the domain's.

type resourcesPayload struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

func newResourcesPayload(r vm.Resources) resourcesPayload {
	return resourcesPayload{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

type networkPayload struct {
	Ingress vm.Access `json:"ingress"`
	Egress  vm.Access `json:"egress"`
}

type statsPayload struct {
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryLimit uint64    `json:"memory_limit"`
	DiskUsed    uint64    `json:"disk_used"`
	DiskTotal   uint64    `json:"disk_total"`
	NetworkRx   uint64    `json:"network_rx"`
	NetworkTx   uint64    `json:"network_tx"`
	SampledAt   time.Time `json:"sampled_at"`
}

type paginationPayload struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

type vmPayload struct {
	UUID            string           `json:"uuid"`
	Name            string           `json:"name"`
	Slug            string           `json:"slug"`
	OwnerUUID       string           `json:"owner_uuid"`
	Kind            string           `json:"kind"`
	Image           string           `json:"image"`
	Resources       resourcesPayload `json:"resources"`
	Ports           []port.Port      `json:"ports"`
	Network         networkPayload   `json:"network"`
	PersistentDisk  bool             `json:"persistent_disk"`
	LifetimeSeconds int64            `json:"lifetime_seconds"`
	ExpiresAt       time.Time        `json:"expires_at"`
	State           string           `json:"state"`
	ExpectedState   string           `json:"expected_state"`
	Reason          string           `json:"reason"`
	NodeName        string           `json:"node_name"`
	Stats           statsPayload     `json:"stats"`
	RestoreFrom     string           `json:"restore_from"`
	LastHeartbeatAt time.Time        `json:"last_heartbeat_at"`
	CreatedAt       time.Time        `json:"created_at"`
	StartedAt       time.Time        `json:"started_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// vmStates maps the words the API uses back onto the VM's own states.
var vmStates = map[string]vm.State{
	vm.Created.String():    vm.Created,
	vm.Scheduled.String():  vm.Scheduled,
	vm.Starting.String():   vm.Starting,
	vm.Running.String():    vm.Running,
	vm.Stopping.String():   vm.Stopping,
	vm.Stopped.String():    vm.Stopped,
	vm.Restarting.String(): vm.Restarting,
	vm.Restoring.String():  vm.Restoring,
	vm.Failed.String():     vm.Failed,
	vm.Deleting.String():   vm.Deleting,
}

func (p *vmPayload) toVM() vm.VM {
	return vm.VM{
		UUID:      p.UUID,
		Name:      p.Name,
		Slug:      p.Slug,
		OwnerUUID: p.OwnerUUID,
		Kind:      vm.Kind(p.Kind),
		Image:     p.Image,
		Resources: vm.Resources{
			CPUs:   p.Resources.CPUs,
			Memory: p.Resources.Memory,
			Disk:   p.Resources.Disk,
		},
		Ports:          p.Ports,
		Network:        vm.Network{Ingress: p.Network.Ingress, Egress: p.Network.Egress},
		PersistentDisk: p.PersistentDisk,
		Lifetime:       time.Duration(p.LifetimeSeconds) * time.Second,
		ExpiresAt:      p.ExpiresAt,
		CurrentState:   vmStates[p.State],
		ExpectedState:  vmStates[p.ExpectedState],
		Reason:         p.Reason,
		NodeName:       p.NodeName,
		Stats: vm.Stats{
			CPUPercent:  p.Stats.CPUPercent,
			MemoryUsed:  p.Stats.MemoryUsed,
			MemoryLimit: p.Stats.MemoryLimit,
			DiskUsed:    p.Stats.DiskUsed,
			DiskTotal:   p.Stats.DiskTotal,
			NetworkRx:   p.Stats.NetworkRx,
			NetworkTx:   p.Stats.NetworkTx,
			SampledAt:   p.Stats.SampledAt,
		},
		RestoreFrom:     p.RestoreFrom,
		LastHeartbeatAt: p.LastHeartbeatAt,
		CreatedAt:       p.CreatedAt,
		StartedAt:       p.StartedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

type vmPagePayload struct {
	Items      []vmPayload       `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

func (p *vmPagePayload) toPage() workloadControlPlane.Page[vm.VM] {
	items := make([]vm.VM, len(p.Items))
	for i := range p.Items {
		items[i] = p.Items[i].toVM()
	}

	return workloadControlPlane.Page[vm.VM]{Items: items, TotalPages: p.Pagination.TotalPages, CurrentPage: p.Pagination.CurrentPage}
}

type createVMPayload struct {
	Name            string           `json:"name"`
	Kind            vm.Kind          `json:"kind,omitempty"`
	Image           string           `json:"image,omitempty"`
	Resources       resourcesPayload `json:"resources"`
	Ports           []port.Port      `json:"ports"`
	Network         networkPayload   `json:"network"`
	PersistentDisk  bool             `json:"persistent_disk"`
	LifetimeSeconds int64            `json:"lifetime_seconds"`
	SnapshotUUID    string           `json:"snapshot_uuid,omitempty"`
}

func newCreateVMPayload(request workloadControlPlane.VMRequest) createVMPayload {
	return createVMPayload{
		Name:            request.Name,
		Kind:            request.Kind,
		Image:           request.Image,
		Resources:       newResourcesPayload(request.Resources),
		Ports:           request.Ports,
		Network:         networkPayload{Ingress: request.Network.Ingress, Egress: request.Network.Egress},
		PersistentDisk:  request.PersistentDisk,
		LifetimeSeconds: int64(request.Lifetime / time.Second),
		SnapshotUUID:    request.SnapshotUUID,
	}
}

type updateVMPayload struct {
	Name            *string           `json:"name,omitempty"`
	LifetimeSeconds *int64            `json:"lifetime_seconds,omitempty"`
	Ports           *[]port.Port      `json:"ports,omitempty"`
	Network         *networkPayload   `json:"network,omitempty"`
	Resources       *resourcesPayload `json:"resources,omitempty"`
}

func newUpdateVMPayload(update workloadControlPlane.VMUpdate) updateVMPayload {
	payload := updateVMPayload{Name: update.Name, Ports: update.Ports}

	if update.Lifetime != nil {
		seconds := int64(*update.Lifetime / time.Second)
		payload.LifetimeSeconds = &seconds
	}

	if update.Network != nil {
		payload.Network = &networkPayload{Ingress: update.Network.Ingress, Egress: update.Network.Egress}
	}

	if update.Resources != nil {
		resources := newResourcesPayload(*update.Resources)
		payload.Resources = &resources
	}

	return payload
}

type vmLogsPayload struct {
	Lines     []noderequest.VMLogLine `json:"lines"`
	Truncated bool                    `json:"truncated"`
}

type snapshotPayload struct {
	UUID        string    `json:"uuid"`
	Name        string    `json:"name"`
	OwnerUUID   string    `json:"owner_uuid"`
	VMUUID      string    `json:"vm_uuid"`
	VMName      string    `json:"vm_name"`
	Kind        string    `json:"kind"`
	Image       string    `json:"image"`
	Disk        uint64    `json:"disk"`
	Engine      string    `json:"engine"`
	Size        int64     `json:"size"`
	State       string    `json:"state"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// snapshotStates maps the words the API uses back onto a snapshot's own
// states.
var snapshotStates = map[string]snapshot.State{
	snapshot.Creating.String(): snapshot.Creating,
	snapshot.Ready.String():    snapshot.Ready,
	snapshot.Failed.String():   snapshot.Failed,
	snapshot.Deleting.String(): snapshot.Deleting,
}

func (p *snapshotPayload) toSnapshot() snapshot.Snapshot {
	return snapshot.Snapshot{
		UUID:        p.UUID,
		Name:        p.Name,
		OwnerUUID:   p.OwnerUUID,
		VMUUID:      p.VMUUID,
		VMName:      p.VMName,
		Kind:        vm.Kind(p.Kind),
		Image:       p.Image,
		Disk:        p.Disk,
		Engine:      p.Engine,
		Size:        p.Size,
		State:       snapshotStates[p.State],
		Reason:      p.Reason,
		CreatedAt:   p.CreatedAt,
		CompletedAt: p.CompletedAt,
	}
}

type snapshotPagePayload struct {
	Items      []snapshotPayload `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

func (p *snapshotPagePayload) toPage() workloadControlPlane.Page[snapshot.Snapshot] {
	items := make([]snapshot.Snapshot, len(p.Items))
	for i := range p.Items {
		items[i] = p.Items[i].toSnapshot()
	}

	return workloadControlPlane.Page[snapshot.Snapshot]{Items: items, TotalPages: p.Pagination.TotalPages, CurrentPage: p.Pagination.CurrentPage}
}

type stackPayload struct {
	UUID          string    `json:"uuid"`
	Name          string    `json:"name"`
	OwnerUUID     string    `json:"owner_uuid"`
	VMUUID        string    `json:"vm_uuid"`
	Slug          string    `json:"slug"`
	Compose       string    `json:"compose"`
	ExpectedState string    `json:"expected_state"`
	State         string    `json:"state"`
	Reason        string    `json:"reason"`
	Output        string    `json:"output"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// stackStates maps the words the API uses back onto a stack's own states.
var stackStates = map[string]stack.State{
	stack.Deploying.String():  stack.Deploying,
	stack.Running.String():    stack.Running,
	stack.Starting.String():   stack.Starting,
	stack.Stopping.String():   stack.Stopping,
	stack.Stopped.String():    stack.Stopped,
	stack.Restarting.String(): stack.Restarting,
	stack.Removing.String():   stack.Removing,
	stack.Failed.String():     stack.Failed,
}

func (p *stackPayload) toStack() stack.Stack {
	return stack.Stack{
		UUID:          p.UUID,
		Name:          p.Name,
		OwnerUUID:     p.OwnerUUID,
		VMUUID:        p.VMUUID,
		Slug:          p.Slug,
		Compose:       p.Compose,
		ExpectedState: stackStates[p.ExpectedState],
		State:         stackStates[p.State],
		Reason:        p.Reason,
		Output:        p.Output,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

type stackPagePayload struct {
	Items      []stackPayload    `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

func (p *stackPagePayload) toPage() workloadControlPlane.Page[stack.Stack] {
	items := make([]stack.Stack, len(p.Items))
	for i := range p.Items {
		items[i] = p.Items[i].toStack()
	}

	return workloadControlPlane.Page[stack.Stack]{Items: items, TotalPages: p.Pagination.TotalPages, CurrentPage: p.Pagination.CurrentPage}
}

type stackDetailPayload struct {
	stackPayload

	Containers   []noderequest.Container `json:"containers"`
	VMNotRunning bool                    `json:"vm_not_running"`
}

// choicePayload is which Docker VM a container or a stack goes into.
type choicePayload struct {
	UUID string        `json:"uuid,omitempty"`
	New  *newVMPayload `json:"new,omitempty"`
}

// newVMPayload is a Docker VM to make. Ports left out are the default ones and
// ports given empty are none, so an empty list travels as one: omitempty would
// leave it out, and a VM asked for with no ports would be made with the
// defaults.
type newVMPayload struct {
	Name      string            `json:"name,omitempty"`
	Resources *resourcesPayload `json:"resources,omitempty"`
	Ports     []port.Port       `json:"ports,omitzero"`
	Network   *networkPayload   `json:"network,omitempty"`
}

func newChoicePayload(choice workloadControlPlane.DockerVMChoice) choicePayload {
	payload := choicePayload{UUID: choice.UUID}

	if choice.New != nil {
		payload.New = &newVMPayload{Name: choice.New.Name, Ports: choice.New.Ports}

		if choice.New.Resources != nil {
			resources := newResourcesPayload(*choice.New.Resources)
			payload.New.Resources = &resources
		}

		if choice.New.Network != nil {
			payload.New.Network = &networkPayload{Ingress: choice.New.Network.Ingress, Egress: choice.New.Network.Egress}
		}
	}

	return payload
}

type chosenVMPayload struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

func (p chosenVMPayload) toChosen() workloadControlPlane.ChosenVM {
	return workloadControlPlane.ChosenVM{UUID: p.UUID, Name: p.Name, Created: p.Created}
}

type createContainerPayload struct {
	VM        choicePayload             `json:"vm"`
	Container noderequest.ContainerSpec `json:"container"`
}

type createdContainerPayload struct {
	VM        chosenVMPayload       `json:"vm"`
	Container noderequest.Container `json:"container"`
}

type vmContainerPayload struct {
	noderequest.Container

	VMUUID string `json:"vm_uuid"`
	VMName string `json:"vm_name"`
}

type containersPayload struct {
	Items []vmContainerPayload `json:"items"`
}

type createStackPayload struct {
	Name    string        `json:"name"`
	Compose string        `json:"compose"`
	VM      choicePayload `json:"vm"`
}

type createdStackPayload struct {
	VM    chosenVMPayload `json:"vm"`
	Stack stackPayload    `json:"stack"`
}

// containersOf is what a node reported as the docker package has it.
func containersOf(travelled []noderequest.Container) []docker.Container {
	containers := make([]docker.Container, len(travelled))
	for i := range travelled {
		containers[i] = travelled[i].ToDocker()
	}

	return containers
}
