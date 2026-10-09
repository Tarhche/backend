// Package wire is what a vmhost and its orchestrator say to each other over
// the unix socket they share: the JSON its API speaks, the errors it answers
// with, and the frames an exec session is carried in once its connection has
// been upgraded.
//
// The server (presentation/http/workload/vmhost) and the client
// (infrastructure/workload/vm/vmhost) both speak it from here, so the two
// cannot drift apart. It is a vm.Engine as it travels: every shape converts to
// and from the vm package's own, and nothing is lost on the way. A nil list
// stays nil and an empty one stays empty, which matters for a spec's
// Entrypoint and Command, where nil keeps the image's own and empty clears it.
package wire

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// The API's paths. Everything about one VM is under PathVM.
const (
	PathInfo    = "/v1/info"
	PathVMs     = "/v1/vms"
	PathRestore = "/v1/restore"
)

// The actions on one VM, under its path.
const (
	ActionStart    = "start"
	ActionStop     = "stop"
	ActionRestart  = "restart"
	ActionStats    = "stats"
	ActionLogs     = "logs"
	ActionExec     = "exec"
	ActionSnapshot = "snapshot"
)

// The query parameters a log is narrowed with.
const (
	QuerySince = "since"
	QueryTail  = "tail"
)

const (
	// SpecHeader carries the spec a restore is made under, as base64 JSON,
	// since the body of a restore is the archive itself.
	SpecHeader = "X-Workload-VM-Spec"

	// ArchiveTrailer carries what a snapshot wrote, as base64 JSON. It is a
	// trailer because it is known only once the archive has been streamed.
	ArchiveTrailer = "X-Workload-VM-Archive"

	// ErrorTrailer carries why a snapshot failed once its archive had begun
	// streaming, when a status can no longer say so. It is an Error, as
	// base64 JSON.
	ErrorTrailer = "X-Workload-VM-Error"

	// ExecProtocol is what an exec request upgrades its connection to: the
	// frames in frame.go.
	ExecProtocol = "vmhost-exec/1"
)

// PathVM is one VM's path, or one of its actions' when an action is given.
func PathVM(id string, action ...string) string {
	path := PathVMs + "/" + url.PathEscape(id)

	for _, a := range action {
		path += "/" + a
	}

	return path
}

// EncodeHeader is v as a header carries it: base64 JSON.
func EncodeHeader(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(encoded), nil
}

// DecodeHeader reads what EncodeHeader wrote into v.
func DecodeHeader(value string, v any) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%w: not base64: %v", ErrInvalid, err)
	}

	if err := json.Unmarshal(decoded, v); err != nil {
		return fmt.Errorf("%w: not JSON: %v", ErrInvalid, err)
	}

	return nil
}

// Resources are vm.Resources as they travel: whole vCPUs, and bytes.
type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

func NewResources(r vm.Resources) Resources {
	return Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

func (r Resources) ToVM() vm.Resources {
	return vm.Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

// Network is a vm.Network as it travels.
type Network struct {
	Ingress vm.Access `json:"ingress"`
	Egress  vm.Access `json:"egress"`
}

// Spec is a vm.Spec as it travels. No list or map in it is omitted when it is
// empty: null and [] are told apart, as the spec tells nil and empty apart.
// What an instance boots into is not in it: the vmhost reads it off the
// image.
type Spec struct {
	ID             string            `json:"id"`
	Image          string            `json:"image"`
	Resources      Resources         `json:"resources"`
	Ports          []port.Port       `json:"ports"`
	Network        Network           `json:"network"`
	PersistentDisk bool              `json:"persistent_disk"`
	Labels         map[string]string `json:"labels"`
	Entrypoint     []string          `json:"entrypoint"`
	Command        []string          `json:"command"`
	Env            []string          `json:"env"`
	WorkingDir     string            `json:"working_dir"`
}

func NewSpec(s vm.Spec) Spec {
	return Spec{
		ID:             s.ID,
		Image:          s.Image,
		Resources:      NewResources(s.Resources),
		Ports:          slices.Clone(s.Ports),
		Network:        Network{Ingress: s.Network.Ingress, Egress: s.Network.Egress},
		PersistentDisk: s.PersistentDisk,
		Labels:         maps.Clone(s.Labels),
		Entrypoint:     slices.Clone(s.Entrypoint),
		Command:        slices.Clone(s.Command),
		Env:            slices.Clone(s.Env),
		WorkingDir:     s.WorkingDir,
	}
}

func (s Spec) ToVM() vm.Spec {
	return vm.Spec{
		ID:             s.ID,
		Image:          s.Image,
		Resources:      s.Resources.ToVM(),
		Ports:          slices.Clone(s.Ports),
		Network:        vm.Network{Ingress: s.Network.Ingress, Egress: s.Network.Egress},
		PersistentDisk: s.PersistentDisk,
		Labels:         maps.Clone(s.Labels),
		Entrypoint:     slices.Clone(s.Entrypoint),
		Command:        slices.Clone(s.Command),
		Env:            slices.Clone(s.Env),
		WorkingDir:     s.WorkingDir,
	}
}

// Endpoint is a vm.Endpoint as it travels.
type Endpoint struct {
	Port    port.Port `json:"port"`
	Address string    `json:"address"`
}

// Instance is a vm.Instance as it travels.
type Instance struct {
	ID        string            `json:"id"`
	State     vm.InstanceState  `json:"state"`
	Labels    map[string]string `json:"labels"`
	Endpoints []Endpoint        `json:"endpoints"`
	ExitCode  int               `json:"exit_code"`
	Reason    string            `json:"reason"`
	StartedAt time.Time         `json:"started_at"`
}

func NewInstance(i vm.Instance) Instance {
	return Instance{
		ID:     i.ID,
		State:  i.State,
		Labels: maps.Clone(i.Labels),
		Endpoints: convert(i.Endpoints, func(e vm.Endpoint) Endpoint {
			return Endpoint{Port: e.Port, Address: e.Address}
		}),
		ExitCode:  i.ExitCode,
		Reason:    i.Reason,
		StartedAt: i.StartedAt,
	}
}

func (i Instance) ToVM() vm.Instance {
	return vm.Instance{
		ID:     i.ID,
		State:  i.State,
		Labels: maps.Clone(i.Labels),
		Endpoints: convert(i.Endpoints, func(e Endpoint) vm.Endpoint {
			return vm.Endpoint{Port: e.Port, Address: e.Address}
		}),
		ExitCode:  i.ExitCode,
		Reason:    i.Reason,
		StartedAt: i.StartedAt,
	}
}

// NewInstances is a listing, ready to travel.
func NewInstances(instances []vm.Instance) []Instance {
	return convert(instances, NewInstance)
}

// InstancesToVM is a listing as the vm package has it.
func InstancesToVM(instances []Instance) []vm.Instance {
	return convert(instances, Instance.ToVM)
}

// Info is a vm.Info as it travels.
type Info struct {
	Engine    string    `json:"engine"`
	Version   string    `json:"version"`
	CPUs      uint      `json:"cpus"`
	Memory    uint64    `json:"memory"`
	Disk      uint64    `json:"disk"`
	Allocated Resources `json:"allocated"`
}

func NewInfo(i vm.Info) Info {
	return Info{
		Engine:    i.Engine,
		Version:   i.Version,
		CPUs:      i.CPUs,
		Memory:    i.Memory,
		Disk:      i.Disk,
		Allocated: NewResources(i.Allocated),
	}
}

func (i Info) ToVM() vm.Info {
	return vm.Info{
		Engine:    i.Engine,
		Version:   i.Version,
		CPUs:      i.CPUs,
		Memory:    i.Memory,
		Disk:      i.Disk,
		Allocated: i.Allocated.ToVM(),
	}
}

// Stats is a vm.Stats as it travels. CPUPercent keeps the engine's meaning, 0
// to 100 of all of the VM's vCPUs together.
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

func NewStats(s vm.Stats) Stats {
	return Stats{
		CPUPercent:  s.CPUPercent,
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		DiskUsed:    s.DiskUsed,
		DiskTotal:   s.DiskTotal,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		SampledAt:   s.SampledAt,
	}
}

func (s Stats) ToVM() vm.Stats {
	return vm.Stats{
		CPUPercent:  s.CPUPercent,
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		DiskUsed:    s.DiskUsed,
		DiskTotal:   s.DiskTotal,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		SampledAt:   s.SampledAt,
	}
}

// LogLine is a vm.LogLine as it travels.
type LogLine struct {
	At     time.Time `json:"at"`
	Source string    `json:"source"`
	Line   string    `json:"line"`
}

// NewLogLines is a log, ready to travel.
func NewLogLines(lines []vm.LogLine) []LogLine {
	return convert(lines, func(l vm.LogLine) LogLine {
		return LogLine{At: l.At, Source: l.Source, Line: l.Line}
	})
}

// LogLinesToVM is a log as the vm package has it.
func LogLinesToVM(lines []LogLine) []vm.LogLine {
	return convert(lines, func(l LogLine) vm.LogLine {
		return vm.LogLine{At: l.At, Source: l.Source, Line: l.Line}
	})
}

// ExecOptions are vm.ExecOptions as they travel: the body of an exec request.
type ExecOptions struct {
	Command    []string `json:"command"`
	TTY        bool     `json:"tty"`
	Rows       uint     `json:"rows"`
	Cols       uint     `json:"cols"`
	Env        []string `json:"env"`
	WorkingDir string   `json:"working_dir"`
}

func NewExecOptions(o vm.ExecOptions) ExecOptions {
	return ExecOptions{
		Command:    slices.Clone(o.Command),
		TTY:        o.TTY,
		Rows:       o.Rows,
		Cols:       o.Cols,
		Env:        slices.Clone(o.Env),
		WorkingDir: o.WorkingDir,
	}
}

func (o ExecOptions) ToVM() vm.ExecOptions {
	return vm.ExecOptions{
		Command:    slices.Clone(o.Command),
		TTY:        o.TTY,
		Rows:       o.Rows,
		Cols:       o.Cols,
		Env:        slices.Clone(o.Env),
		WorkingDir: o.WorkingDir,
	}
}

// Archive is a vm.Archive as it travels: what a snapshot wrote.
type Archive struct {
	Engine string  `json:"engine"`
	Kind   vm.Kind `json:"kind"`
	Image  string  `json:"image"`
	Disk   uint64  `json:"disk"`
	Size   int64   `json:"size"`
}

func NewArchive(a vm.Archive) Archive {
	return Archive{Engine: a.Engine, Kind: a.Kind, Image: a.Image, Disk: a.Disk, Size: a.Size}
}

func (a Archive) ToVM() vm.Archive {
	return vm.Archive{Engine: a.Engine, Kind: a.Kind, Image: a.Image, Disk: a.Disk, Size: a.Size}
}

// convert is every item of a list converted, keeping a nil list nil.
func convert[T, U any](items []T, f func(T) U) []U {
	if items == nil {
		return nil
	}

	converted := make([]U, len(items))
	for n, item := range items {
		converted[n] = f(item)
	}

	return converted
}
