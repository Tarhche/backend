package createContainer

import (
	"math"
	"strings"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Request is a container to create and start in one of the caller's Docker
// VMs. It is always created for whoever asks.
//
// Which VM it goes in is the request's to say: one named by VMUUID, a new one
// described by VM, or neither, which puts it in the caller's only Docker VM,
// or in one made for it when they have none.
type Request struct {
	VMUUID string             `json:"vm_uuid,omitempty"`
	VM     *input.NewDockerVM `json:"vm,omitempty"`

	// Name is docker's name for it; empty lets docker pick one.
	Name string `json:"name,omitempty"`

	// Image is pulled first when the VM does not hold it.
	Image string `json:"image"`

	Command    []string `json:"command,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`

	// Env is the environment, each as KEY=value.
	Env        []string `json:"env,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`

	// Ports publish container ports on the VM. A VM port the VM also exposes
	// through the ingress is reachable from outside; any other is reachable
	// only from inside the VM.
	Ports  []PortBinding `json:"ports,omitempty"`
	Mounts []Mount       `json:"mounts,omitempty"`

	// Networks are the VM's docker networks to attach it to, beside the
	// default one.
	Networks []string `json:"networks,omitempty"`

	// RestartPolicy is no, always, unless-stopped or on-failure.
	RestartPolicy string `json:"restart_policy,omitempty"`

	// CPUs is in cores and Memory in bytes; zero is no limit beyond the VM's.
	CPUs   float64 `json:"cpus,omitempty"`
	Memory uint64  `json:"memory,omitempty"`

	OwnerUUID string `json:"-"`
}

// PortBinding publishes a container port on the VM. A host port of zero lets
// docker pick one.
type PortBinding struct {
	ContainerPort uint `json:"container_port"`
	HostPort      uint `json:"host_port"`

	// Protocol is tcp or udp; empty is tcp.
	Protocol string `json:"protocol,omitempty"`
}

// Mount is something mounted into the container: a volume of the VM, a path
// of the VM (bind), or a tmpfs, at an absolute path inside the container.
type Mount struct {
	Type     string `json:"type"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// the protocols, mount types and restart policies docker takes.
var (
	protocols = map[string]struct{}{
		"":    {},
		"tcp": {},
		"udp": {},
	}

	mountTypes = map[string]struct{}{
		"volume": {},
		"bind":   {},
		"tmpfs":  {},
	}

	restartPolicies = map[string]struct{}{
		"":               {},
		"no":             {},
		"always":         {},
		"unless-stopped": {},
		"on-failure":     {},
	}
)

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	input.DockerVM(validationErrors, r.VMUUID, r.VM)
	input.Image(validationErrors, "image", r.Image)
	input.DockerName(validationErrors, "name", r.Name)

	r.validatePorts(validationErrors)
	r.validateMounts(validationErrors)

	if _, ok := restartPolicies[r.RestartPolicy]; !ok {
		validationErrors["restart_policy"] = "invalid_restart_policy"
	}

	if r.CPUs < 0 || math.IsNaN(r.CPUs) || math.IsInf(r.CPUs, 0) {
		validationErrors["cpus"] = "invalid_value"
	}

	return validationErrors
}

func (r *Request) validatePorts(validationErrors domain.ValidationErrors) {
	// one host port carries one protocol for one container; docker picks a
	// free one for each binding that names none.
	taken := make(map[PortBinding]struct{}, len(r.Ports))

	for i, binding := range r.Ports {
		field := input.Item("ports", i)
		published := PortBinding{HostPort: binding.HostPort, Protocol: protocol(binding.Protocol)}

		switch _, twice := taken[published]; {
		case !input.IsPort(binding.ContainerPort):
			validationErrors[field+".container_port"] = "invalid_port"
		case binding.HostPort > math.MaxUint16:
			validationErrors[field+".host_port"] = "invalid_port"
		case !isProtocol(binding.Protocol):
			validationErrors[field+".protocol"] = "invalid_protocol"
		case binding.HostPort > 0 && twice:
			validationErrors[field+".host_port"] = "duplicate_port"
		}

		taken[published] = struct{}{}
	}
}

func (r *Request) validateMounts(validationErrors domain.ValidationErrors) {
	for i, mount := range r.Mounts {
		field := input.Item("mounts", i)

		_, known := mountTypes[mount.Type]

		switch {
		case !known:
			validationErrors[field+".type"] = "invalid_mount_type"

		// a tmpfs is made where it is mounted; anything else is mounted from
		// somewhere, and into an absolute path.
		case !strings.HasPrefix(mount.Target, "/"),
			mount.Type != "tmpfs" && len(strings.TrimSpace(mount.Source)) == 0:
			validationErrors[field] = "invalid_mount"
		}
	}
}

// Spec is the container as docker is asked for it.
func (r *Request) Spec() docker.ContainerSpec {
	var ports []docker.PortBinding
	if len(r.Ports) > 0 {
		ports = make([]docker.PortBinding, len(r.Ports))
		for i, binding := range r.Ports {
			ports[i] = docker.PortBinding{
				ContainerPort: port.Port(binding.ContainerPort),
				HostPort:      port.Port(binding.HostPort),
				Protocol:      protocol(binding.Protocol),
			}
		}
	}

	var mounts []docker.Mount
	if len(r.Mounts) > 0 {
		mounts = make([]docker.Mount, len(r.Mounts))
		for i, mount := range r.Mounts {
			mounts[i] = docker.Mount{
				Type:     mount.Type,
				Source:   mount.Source,
				Target:   mount.Target,
				ReadOnly: mount.ReadOnly,
			}
		}
	}

	return docker.ContainerSpec{
		Name:          r.Name,
		Image:         r.Image,
		Command:       r.Command,
		Entrypoint:    r.Entrypoint,
		Env:           r.Env,
		WorkingDir:    r.WorkingDir,
		Ports:         ports,
		Mounts:        mounts,
		Networks:      r.Networks,
		RestartPolicy: r.RestartPolicy,
		CPUs:          r.CPUs,
		Memory:        r.Memory,
	}
}

func isProtocol(p string) bool {
	_, ok := protocols[p]

	return ok
}

// protocol is a binding's protocol as docker reads it: tcp unless it says
// udp.
func protocol(p string) string {
	if len(p) == 0 {
		return "tcp"
	}

	return p
}
