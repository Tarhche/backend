// Package presenter is how the control plane's API shows a VM, a snapshot, a
// stack and a container: snake_case JSON, states as words, sizes as bytes and
// durations as seconds.
//
// The blog's control plane client reads exactly these shapes back, so a field
// renamed here is renamed there too.
package presenter

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Pagination says where a page is in its listing.
type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

// NewPagination is the page currentPage of a listing of total items, limit to
// a page.
func NewPagination(total uint, limit uint, currentPage uint) Pagination {
	totalPages := total / limit
	if totalPages*limit != total {
		totalPages++
	}

	return Pagination{TotalPages: totalPages, CurrentPage: currentPage}
}

// Offset is where page starts in a listing of limit to a page. Page zero is
// the first page, as page one is.
func Offset(page uint, limit uint) (uint, uint) {
	if page == 0 {
		page = 1
	}

	return (page - 1) * limit, page
}

type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

func NewResources(r vm.Resources) Resources {
	return Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

type Network struct {
	Ingress string `json:"ingress"`
	Egress  string `json:"egress"`
}

func NewNetwork(n vm.Network) Network {
	return Network{Ingress: string(n.Ingress), Egress: string(n.Egress)}
}

type Stats struct {
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryLimit uint64    `json:"memory_limit"`
	DiskUsed    uint64    `json:"disk_used"`
	DiskTotal   uint64    `json:"disk_total"`
	NetworkRx   uint64    `json:"network_rx"`
	NetworkTx   uint64    `json:"network_tx"`
	SampledAt   time.Time `json:"sampled_at"`
}

// VM is a VM as the API shows it. The addresses its ports are served at are
// the blog's to build, from the ingress domain it knows and the slug here.
type VM struct {
	UUID            string      `json:"uuid"`
	Name            string      `json:"name"`
	Slug            string      `json:"slug"`
	OwnerUUID       string      `json:"owner_uuid"`
	Kind            string      `json:"kind"`
	Image           string      `json:"image"`
	Resources       Resources   `json:"resources"`
	Ports           []port.Port `json:"ports"`
	Network         Network     `json:"network"`
	PersistentDisk  bool        `json:"persistent_disk"`
	LifetimeSeconds int64       `json:"lifetime_seconds"`
	ExpiresAt       time.Time   `json:"expires_at"`
	State           string      `json:"state"`
	ExpectedState   string      `json:"expected_state"`
	Reason          string      `json:"reason"`
	NodeName        string      `json:"node_name"`
	Stats           Stats       `json:"stats"`
	RestoreFrom     string      `json:"restore_from"`
	LastHeartbeatAt time.Time   `json:"last_heartbeat_at"`
	CreatedAt       time.Time   `json:"created_at"`
	StartedAt       time.Time   `json:"started_at"`
	UpdatedAt       time.Time   `json:"updated_at"`

	// ManagedBy names what keeps a VM that is not a record of its own: the
	// code runner, for one of its runs. It is left out for every other VM.
	ManagedBy string `json:"managed_by,omitempty"`
}

func NewVM(v *vm.VM) VM {
	ports := v.Ports
	if ports == nil {
		ports = []port.Port{}
	}

	return VM{
		UUID:            v.UUID,
		Name:            v.Name,
		Slug:            v.Slug,
		OwnerUUID:       v.OwnerUUID,
		Kind:            string(v.Kind),
		Image:           v.Image,
		Resources:       NewResources(v.Resources),
		Ports:           ports,
		Network:         NewNetwork(v.Network),
		PersistentDisk:  v.PersistentDisk,
		LifetimeSeconds: int64(v.Lifetime / time.Second),
		ExpiresAt:       v.ExpiresAt,
		State:           v.CurrentState.String(),
		ExpectedState:   v.ExpectedState.String(),
		Reason:          v.Reason,
		NodeName:        v.NodeName,
		Stats: Stats{
			CPUPercent:  v.Stats.CPUPercent,
			MemoryUsed:  v.Stats.MemoryUsed,
			MemoryLimit: v.Stats.MemoryLimit,
			DiskUsed:    v.Stats.DiskUsed,
			DiskTotal:   v.Stats.DiskTotal,
			NetworkRx:   v.Stats.NetworkRx,
			NetworkTx:   v.Stats.NetworkTx,
			SampledAt:   v.Stats.SampledAt,
		},
		RestoreFrom:     v.RestoreFrom,
		LastHeartbeatAt: v.LastHeartbeatAt,
		CreatedAt:       v.CreatedAt,
		StartedAt:       v.StartedAt,
		UpdatedAt:       v.UpdatedAt,
		ManagedBy:       v.ManagedBy,
	}
}

func NewVMs(vms []vm.VM) []VM {
	items := make([]VM, len(vms))
	for i := range vms {
		items[i] = NewVM(&vms[i])
	}

	return items
}

// Snapshot is a snapshot as the API shows it.
type Snapshot struct {
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

func NewSnapshot(s *snapshot.Snapshot) Snapshot {
	return Snapshot{
		UUID:        s.UUID,
		Name:        s.Name,
		OwnerUUID:   s.OwnerUUID,
		VMUUID:      s.VMUUID,
		VMName:      s.VMName,
		Kind:        string(s.Kind),
		Image:       s.Image,
		Disk:        s.Disk,
		Engine:      s.Engine,
		Size:        s.Size,
		State:       s.State.String(),
		Reason:      s.Reason,
		CreatedAt:   s.CreatedAt,
		CompletedAt: s.CompletedAt,
	}
}

func NewSnapshots(snapshots []snapshot.Snapshot) []Snapshot {
	items := make([]Snapshot, len(snapshots))
	for i := range snapshots {
		items[i] = NewSnapshot(&snapshots[i])
	}

	return items
}

// Stack is a stack as the API shows it. VMName is what its VM is called when
// it is read, so a VM that is renamed is named anew.
type Stack struct {
	UUID          string    `json:"uuid"`
	Name          string    `json:"name"`
	OwnerUUID     string    `json:"owner_uuid"`
	VMUUID        string    `json:"vm_uuid"`
	VMName        string    `json:"vm_name"`
	Slug          string    `json:"slug"`
	Compose       string    `json:"compose"`
	ExpectedState string    `json:"expected_state"`
	State         string    `json:"state"`
	Reason        string    `json:"reason"`
	Output        string    `json:"output"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewStack(s *stack.Stack) Stack {
	return Stack{
		UUID:          s.UUID,
		Name:          s.Name,
		OwnerUUID:     s.OwnerUUID,
		VMUUID:        s.VMUUID,
		VMName:        s.VMName,
		Slug:          s.Slug,
		Compose:       s.Compose,
		ExpectedState: s.ExpectedState.String(),
		State:         s.State.String(),
		Reason:        s.Reason,
		Output:        s.Output,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func NewStacks(stacks []stack.Stack) []Stack {
	items := make([]Stack, len(stacks))
	for i := range stacks {
		items[i] = NewStack(&stacks[i])
	}

	return items
}

// Container is a container as a node reports it, in the shape node requests
// carry it.
type Container = noderequest.Container

func NewContainers(containers []docker.Container) []Container {
	items := make([]Container, len(containers))
	for i := range containers {
		items[i] = noderequest.NewContainer(containers[i])
	}

	return items
}

// ChosenVM is the Docker VM a container or a stack went into, and whether it
// was made for it.
type ChosenVM struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// VMContainer is a container and the Docker VM it is in.
type VMContainer struct {
	Container

	VMUUID string `json:"vm_uuid"`
	VMName string `json:"vm_name"`
}
