package client

import (
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// the shapes the control plane's API speaks about snapshots and containers,
// and about a Docker VM a container is put in; a VM and a stack are
// manifests. What a node reports about dockerd's objects travels in the node
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

type paginationPayload struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
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

// choicePayload is which Docker VM a container goes into.
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

// containersOf is what a node reported as the docker package has it.
func containersOf(travelled []noderequest.Container) []docker.Container {
	containers := make([]docker.Container, len(travelled))
	for i := range travelled {
		containers[i] = travelled[i].ToDocker()
	}

	return containers
}
