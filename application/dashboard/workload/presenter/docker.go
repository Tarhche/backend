package presenter

import (
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// Container is one container in a Docker VM, as its node last reported it.
type Container struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`

	// State is docker's: created, running, paused, restarting, removing,
	// exited or dead. Status is the sentence docker puts it in.
	State   string `json:"state"`
	Status  string `json:"status"`
	Command string `json:"command"`

	Ports    []PortBinding     `json:"ports"`
	Networks []string          `json:"networks"`
	Mounts   []Mount           `json:"mounts"`
	Labels   map[string]string `json:"labels,omitempty"`

	// Stack and Service are the compose project and service it belongs to,
	// when a stack deployed it. The project is the stack's slug, and
	// StackUUID is the stack's uuid when the request could see that stack.
	Stack     string `json:"stack,omitempty"`
	StackUUID string `json:"stack_uuid,omitempty"`
	Service   string `json:"service,omitempty"`

	RestartPolicy string    `json:"restart_policy,omitempty"`
	CreatedAt     time.Time `json:"created_at"`

	// Unmanaged says nobody keeps it: it was made from its VM's terminal, so
	// nothing brings it back when it stops or goes.
	Unmanaged bool `json:"unmanaged,omitempty"`
}

// VMContainer is a container together with the Docker VM it was found in,
// which is what a listing across VMs needs to say.
type VMContainer struct {
	Container

	VMUUID string `json:"vm_uuid"`
	VMName string `json:"vm_name"`
}

// PortBinding is a container port published on its VM.
type PortBinding struct {
	ContainerPort uint   `json:"container_port"`
	HostPort      uint   `json:"host_port"`
	Protocol      string `json:"protocol"`
}

// Mount is something mounted into a container: a volume, a path of the VM
// (bind) or a tmpfs.
type Mount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// ContainerStats is one sample of what a container uses. Memory, network and
// block counters are bytes.
type ContainerStats struct {
	// CPUPercent is 0 to 100 of all of the CPUs the container can see, which
	// are its VM's vCPUs: 100 is every one of them busy.
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryLimit uint64    `json:"memory_limit"`
	NetworkRx   uint64    `json:"network_rx"`
	NetworkTx   uint64    `json:"network_tx"`
	BlockRead   uint64    `json:"block_read"`
	BlockWrite  uint64    `json:"block_write"`
	PIDs        uint64    `json:"pids"`
	SampledAt   time.Time `json:"sampled_at"`
}

// LogLine is one line a container wrote, and whether it went to stdout or to
// stderr.
type LogLine struct {
	At     time.Time `json:"at"`
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
}

// Image is one image a Docker VM holds. Size is in bytes.
type Image struct {
	ID        string    `json:"id"`
	Tags      []string  `json:"tags"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`

	// InUse says a container was created from it.
	InUse bool `json:"in_use"`

	// Unmanaged says nobody keeps it: it was pulled from its VM's terminal,
	// and no container the platform keeps, nor any of a stack's, uses it.
	Unmanaged bool `json:"unmanaged,omitempty"`
}

// DockerNetwork is one docker network inside a Docker VM. It never reaches
// past the VM it is in.
type DockerNetwork struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Scope    string `json:"scope"`
	Internal bool   `json:"internal"`

	// Containers are the containers attached to it.
	Containers []string `json:"containers"`

	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`

	// Unmanaged says nobody keeps it: it was made from its VM's terminal.
	Unmanaged bool `json:"unmanaged,omitempty"`
}

// Volume is one docker volume inside a Docker VM.
type Volume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	Labels     map[string]string `json:"labels,omitempty"`

	// InUse says a container mounts it.
	InUse bool `json:"in_use"`

	CreatedAt time.Time `json:"created_at"`

	// Unmanaged says nobody keeps it: it was made from its VM's terminal.
	Unmanaged bool `json:"unmanaged,omitempty"`
}

func NewContainer(c docker.Container) Container {
	ports := make([]PortBinding, len(c.Ports))
	for i, p := range c.Ports {
		ports[i] = PortBinding{
			ContainerPort: uint(p.ContainerPort),
			HostPort:      uint(p.HostPort),
			Protocol:      p.Protocol,
		}
	}

	mounts := make([]Mount, len(c.Mounts))
	for i, m := range c.Mounts {
		mounts[i] = Mount{
			Type:     m.Type,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		}
	}

	return Container{
		ID:            c.ID,
		Name:          c.Name,
		Image:         c.Image,
		State:         c.State,
		Status:        c.Status,
		Command:       c.Command,
		Ports:         ports,
		Networks:      list(c.Networks),
		Mounts:        mounts,
		Labels:        c.Labels,
		Stack:         c.Stack,
		Service:       c.Service,
		RestartPolicy: c.RestartPolicy,
		CreatedAt:     c.CreatedAt,
		Unmanaged:     c.Unmanaged,
	}
}

func NewContainers(containers []docker.Container) []Container {
	items := make([]Container, len(containers))
	for i := range containers {
		items[i] = NewContainer(containers[i])
	}

	return items
}

func NewVMContainers(containers []workloadControlPlane.VMContainer) []VMContainer {
	items := make([]VMContainer, len(containers))
	for i := range containers {
		items[i] = VMContainer{
			Container: NewContainer(containers[i].Container),
			VMUUID:    containers[i].VMUUID,
			VMName:    containers[i].VMName,
		}
	}

	return items
}

func NewContainerStats(s docker.Stats) ContainerStats {
	return ContainerStats{
		CPUPercent:  s.CPUPercent,
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		BlockRead:   s.BlockRead,
		BlockWrite:  s.BlockWrite,
		PIDs:        s.PIDs,
		SampledAt:   s.SampledAt,
	}
}

func NewLogLines(lines []docker.LogLine) []LogLine {
	items := make([]LogLine, len(lines))
	for i, l := range lines {
		items[i] = LogLine{At: l.At, Stream: l.Stream, Line: l.Line}
	}

	return items
}

func NewImage(i docker.Image) Image {
	return Image{
		ID:        i.ID,
		Tags:      list(i.Tags),
		Size:      i.Size,
		CreatedAt: i.CreatedAt,
		InUse:     i.InUse,
		Unmanaged: i.Unmanaged,
	}
}

func NewImages(images []docker.Image) []Image {
	items := make([]Image, len(images))
	for i := range images {
		items[i] = NewImage(images[i])
	}

	return items
}

func NewDockerNetwork(n docker.Network) DockerNetwork {
	return DockerNetwork{
		ID:         n.ID,
		Name:       n.Name,
		Driver:     n.Driver,
		Scope:      n.Scope,
		Internal:   n.Internal,
		Containers: list(n.Containers),
		Labels:     n.Labels,
		CreatedAt:  n.CreatedAt,
		Unmanaged:  n.Unmanaged,
	}
}

func NewDockerNetworks(networks []docker.Network) []DockerNetwork {
	items := make([]DockerNetwork, len(networks))
	for i := range networks {
		items[i] = NewDockerNetwork(networks[i])
	}

	return items
}

func NewVolume(v docker.Volume) Volume {
	return Volume{
		Name:       v.Name,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		Labels:     v.Labels,
		InUse:      v.InUse,
		CreatedAt:  v.CreatedAt,
		Unmanaged:  v.Unmanaged,
	}
}

func NewVolumes(volumes []docker.Volume) []Volume {
	items := make([]Volume, len(volumes))
	for i := range volumes {
		items[i] = NewVolume(volumes[i])
	}

	return items
}

// list is a list that is a list even when it is empty, so a client reads
// [] rather than null.
func list(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}
