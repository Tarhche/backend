package noderequest

import (
	"maps"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// The payloads a request carries and the results a reply does, one shape for
// each operation, as the doc of its Op says. They are the wire's own types: the
// docker package's are values with no shape on a wire, and are converted to and
// from these at either end.

// LogsRequest narrows what is read of a VM's log.
type LogsRequest struct {
	Since time.Time `json:"since"`
	Tail  uint      `json:"tail"`
}

// NewLogsRequest asks for what options describe.
func NewLogsRequest(options vm.LogOptions) LogsRequest {
	return LogsRequest{Since: options.Since, Tail: options.Tail}
}

// ToVM is the request as the vm package has it.
func (r LogsRequest) ToVM() vm.LogOptions {
	return vm.LogOptions{Since: r.Since, Tail: r.Tail}
}

// VMLogLine is one line of a VM's log.
type VMLogLine struct {
	At     time.Time `json:"at"`
	Source string    `json:"source"`
	Line   string    `json:"line"`
}

func NewVMLogLine(line vm.LogLine) VMLogLine {
	return VMLogLine{At: line.At, Source: line.Source, Line: line.Line}
}

func (l VMLogLine) ToVM() vm.LogLine {
	return vm.LogLine{At: l.At, Source: l.Source, Line: l.Line}
}

// ContainersRequest narrows a listing of containers.
type ContainersRequest struct {
	All   bool   `json:"all"`
	Stack string `json:"stack,omitempty"`
}

func NewContainersRequest(filter docker.ContainerFilter) ContainersRequest {
	return ContainersRequest{All: filter.All, Stack: filter.Stack}
}

func (r ContainersRequest) ToDocker() docker.ContainerFilter {
	return docker.ContainerFilter{All: r.All, Stack: r.Stack}
}

// ContainerRequest names one container, by its id or its name.
type ContainerRequest struct {
	ID string `json:"id"`
}

// RemoveRequest names one container, image, network or volume to remove. A
// volume is named by its name, which is the only id it has.
type RemoveRequest struct {
	ID    string `json:"id"`
	Force bool   `json:"force"`
}

// ContainerLogsRequest narrows what is read of a container's log.
type ContainerLogsRequest struct {
	ID    string    `json:"id"`
	Since time.Time `json:"since"`
	Tail  uint      `json:"tail"`
}

func NewContainerLogsRequest(id string, options docker.LogOptions) ContainerLogsRequest {
	return ContainerLogsRequest{ID: id, Since: options.Since, Tail: options.Tail}
}

// Options is what the request narrows the log to, as the docker package has
// it.
func (r ContainerLogsRequest) Options() docker.LogOptions {
	return docker.LogOptions{Since: r.Since, Tail: r.Tail}
}

// ConnectRequest attaches a container to a network.
type ConnectRequest struct {
	Network   string   `json:"network"`
	Container string   `json:"container"`
	Aliases   []string `json:"aliases,omitempty"`
}

// DisconnectRequest detaches a container from a network.
type DisconnectRequest struct {
	Network   string `json:"network"`
	Container string `json:"container"`
	Force     bool   `json:"force"`
}

// PullRequest names an image to pull.
type PullRequest struct {
	Reference string `json:"reference"`
}

// PortBinding is a docker.PortBinding as it travels.
type PortBinding struct {
	ContainerPort port.Port `json:"container_port"`
	HostPort      port.Port `json:"host_port"`
	Protocol      string    `json:"protocol"`
}

// Mount is a docker.Mount as it travels.
type Mount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// Container is a docker.Container as it travels.
type Container struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	State         string            `json:"state"`
	Status        string            `json:"status"`
	Command       string            `json:"command"`
	Ports         []PortBinding     `json:"ports"`
	Networks      []string          `json:"networks"`
	Mounts        []Mount           `json:"mounts"`
	Labels        map[string]string `json:"labels,omitempty"`
	Stack         string            `json:"stack,omitempty"`
	Service       string            `json:"service,omitempty"`
	RestartPolicy string            `json:"restart_policy"`
	CreatedAt     time.Time         `json:"created_at"`
}

func NewContainer(c docker.Container) Container {
	return Container{
		ID:            c.ID,
		Name:          c.Name,
		Image:         c.Image,
		State:         c.State,
		Status:        c.Status,
		Command:       c.Command,
		Ports:         newPortBindings(c.Ports),
		Networks:      slices.Clone(c.Networks),
		Mounts:        newMounts(c.Mounts),
		Labels:        maps.Clone(c.Labels),
		Stack:         c.Stack,
		Service:       c.Service,
		RestartPolicy: c.RestartPolicy,
		CreatedAt:     c.CreatedAt,
	}
}

func (c Container) ToDocker() docker.Container {
	return docker.Container{
		ID:            c.ID,
		Name:          c.Name,
		Image:         c.Image,
		State:         c.State,
		Status:        c.Status,
		Command:       c.Command,
		Ports:         dockerPortBindings(c.Ports),
		Networks:      slices.Clone(c.Networks),
		Mounts:        dockerMounts(c.Mounts),
		Labels:        maps.Clone(c.Labels),
		Stack:         c.Stack,
		Service:       c.Service,
		RestartPolicy: c.RestartPolicy,
		CreatedAt:     c.CreatedAt,
	}
}

// ContainerSpec is a docker.ContainerSpec as it travels.
type ContainerSpec struct {
	Name          string            `json:"name,omitempty"`
	Image         string            `json:"image"`
	Command       []string          `json:"command,omitempty"`
	Entrypoint    []string          `json:"entrypoint,omitempty"`
	Env           []string          `json:"env,omitempty"`
	WorkingDir    string            `json:"working_dir,omitempty"`
	Ports         []PortBinding     `json:"ports,omitempty"`
	Mounts        []Mount           `json:"mounts,omitempty"`
	Networks      []string          `json:"networks,omitempty"`
	RestartPolicy string            `json:"restart_policy,omitempty"`
	CPUs          float64           `json:"cpus,omitempty"`
	Memory        uint64            `json:"memory,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

func NewContainerSpec(s docker.ContainerSpec) ContainerSpec {
	return ContainerSpec{
		Name:          s.Name,
		Image:         s.Image,
		Command:       slices.Clone(s.Command),
		Entrypoint:    slices.Clone(s.Entrypoint),
		Env:           slices.Clone(s.Env),
		WorkingDir:    s.WorkingDir,
		Ports:         newPortBindings(s.Ports),
		Mounts:        newMounts(s.Mounts),
		Networks:      slices.Clone(s.Networks),
		RestartPolicy: s.RestartPolicy,
		CPUs:          s.CPUs,
		Memory:        s.Memory,
		Labels:        maps.Clone(s.Labels),
	}
}

func (s ContainerSpec) ToDocker() docker.ContainerSpec {
	return docker.ContainerSpec{
		Name:          s.Name,
		Image:         s.Image,
		Command:       slices.Clone(s.Command),
		Entrypoint:    slices.Clone(s.Entrypoint),
		Env:           slices.Clone(s.Env),
		WorkingDir:    s.WorkingDir,
		Ports:         dockerPortBindings(s.Ports),
		Mounts:        dockerMounts(s.Mounts),
		Networks:      slices.Clone(s.Networks),
		RestartPolicy: s.RestartPolicy,
		CPUs:          s.CPUs,
		Memory:        s.Memory,
		Labels:        maps.Clone(s.Labels),
	}
}

// LogLine is a docker.LogLine as it travels.
type LogLine struct {
	At     time.Time `json:"at"`
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
}

func NewLogLine(line docker.LogLine) LogLine {
	return LogLine{At: line.At, Stream: line.Stream, Line: line.Line}
}

func (l LogLine) ToDocker() docker.LogLine {
	return docker.LogLine{At: l.At, Stream: l.Stream, Line: l.Line}
}

// Stats is a docker.Stats as it travels.
type Stats struct {
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

func NewStats(s docker.Stats) Stats {
	return Stats{
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

func (s Stats) ToDocker() docker.Stats {
	return docker.Stats{
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

// Image is a docker.Image as it travels.
type Image struct {
	ID        string    `json:"id"`
	Tags      []string  `json:"tags"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	InUse     bool      `json:"in_use"`
}

func NewImage(i docker.Image) Image {
	return Image{ID: i.ID, Tags: slices.Clone(i.Tags), Size: i.Size, CreatedAt: i.CreatedAt, InUse: i.InUse}
}

func (i Image) ToDocker() docker.Image {
	return docker.Image{ID: i.ID, Tags: slices.Clone(i.Tags), Size: i.Size, CreatedAt: i.CreatedAt, InUse: i.InUse}
}

// Network is a docker.Network as it travels.
type Network struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Containers []string          `json:"containers"`
	Labels     map[string]string `json:"labels,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

func NewNetwork(n docker.Network) Network {
	return Network{
		ID:         n.ID,
		Name:       n.Name,
		Driver:     n.Driver,
		Scope:      n.Scope,
		Internal:   n.Internal,
		Containers: slices.Clone(n.Containers),
		Labels:     maps.Clone(n.Labels),
		CreatedAt:  n.CreatedAt,
	}
}

func (n Network) ToDocker() docker.Network {
	return docker.Network{
		ID:         n.ID,
		Name:       n.Name,
		Driver:     n.Driver,
		Scope:      n.Scope,
		Internal:   n.Internal,
		Containers: slices.Clone(n.Containers),
		Labels:     maps.Clone(n.Labels),
		CreatedAt:  n.CreatedAt,
	}
}

// NetworkSpec is a docker.NetworkSpec as it travels.
type NetworkSpec struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver,omitempty"`
	Internal bool              `json:"internal"`
	Labels   map[string]string `json:"labels,omitempty"`
}

func NewNetworkSpec(s docker.NetworkSpec) NetworkSpec {
	return NetworkSpec{Name: s.Name, Driver: s.Driver, Internal: s.Internal, Labels: maps.Clone(s.Labels)}
}

func (s NetworkSpec) ToDocker() docker.NetworkSpec {
	return docker.NetworkSpec{Name: s.Name, Driver: s.Driver, Internal: s.Internal, Labels: maps.Clone(s.Labels)}
}

// Volume is a docker.Volume as it travels.
type Volume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	Labels     map[string]string `json:"labels,omitempty"`
	InUse      bool              `json:"in_use"`
	CreatedAt  time.Time         `json:"created_at"`
}

func NewVolume(v docker.Volume) Volume {
	return Volume{
		Name:       v.Name,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		Labels:     maps.Clone(v.Labels),
		InUse:      v.InUse,
		CreatedAt:  v.CreatedAt,
	}
}

func (v Volume) ToDocker() docker.Volume {
	return docker.Volume{
		Name:       v.Name,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		Labels:     maps.Clone(v.Labels),
		InUse:      v.InUse,
		CreatedAt:  v.CreatedAt,
	}
}

// VolumeSpec is a docker.VolumeSpec as it travels.
type VolumeSpec struct {
	Name   string            `json:"name"`
	Driver string            `json:"driver,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

func NewVolumeSpec(s docker.VolumeSpec) VolumeSpec {
	return VolumeSpec{Name: s.Name, Driver: s.Driver, Labels: maps.Clone(s.Labels)}
}

func (s VolumeSpec) ToDocker() docker.VolumeSpec {
	return docker.VolumeSpec{Name: s.Name, Driver: s.Driver, Labels: maps.Clone(s.Labels)}
}

func newPortBindings(bindings []docker.PortBinding) []PortBinding {
	if bindings == nil {
		return nil
	}

	travelling := make([]PortBinding, len(bindings))
	for i, b := range bindings {
		travelling[i] = PortBinding{ContainerPort: b.ContainerPort, HostPort: b.HostPort, Protocol: b.Protocol}
	}

	return travelling
}

func dockerPortBindings(bindings []PortBinding) []docker.PortBinding {
	if bindings == nil {
		return nil
	}

	arrived := make([]docker.PortBinding, len(bindings))
	for i, b := range bindings {
		arrived[i] = docker.PortBinding{ContainerPort: b.ContainerPort, HostPort: b.HostPort, Protocol: b.Protocol}
	}

	return arrived
}

func newMounts(mounts []docker.Mount) []Mount {
	if mounts == nil {
		return nil
	}

	travelling := make([]Mount, len(mounts))
	for i, m := range mounts {
		travelling[i] = Mount{Type: m.Type, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly}
	}

	return travelling
}

func dockerMounts(mounts []Mount) []docker.Mount {
	if mounts == nil {
		return nil
	}

	arrived := make([]docker.Mount, len(mounts))
	for i, m := range mounts {
		arrived[i] = docker.Mount{Type: m.Type, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly}
	}

	return arrived
}
