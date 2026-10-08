// Package input is what the workload's dashboard requests have in common, and
// the rules each piece is held to before anything is asked of the workload.
//
// These are shapes and simple ranges only: a port is a port, a kind is a kind.
// What is allowed — how much memory, how many VMs, how long a lifetime — is
// the control plane's to say, since it holds the bounds and the quotas, and
// what it refuses comes back to the caller as it said it.
package input

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// MaxPorts is the most ports one VM exposes through the ingress: each is
	// a port of its node, taken from a range every VM on the node shares.
	MaxPorts = 16

	// MaxNameLength is the longest a name is, in characters.
	MaxNameLength = 255

	// maxLifetimeSeconds is the longest lifetime a duration can hold. The
	// control plane's own bound is far shorter; this only keeps a number from
	// wrapping round into a negative duration on its way there.
	maxLifetimeSeconds = math.MaxInt64 / int64(time.Second)
)

// dockerName is what docker accepts as the name of a container or a volume,
// and what it is held to here for a network too.
var dockerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)

// Name is a name somebody gives a VM, a snapshot or a stack: anything they
// like, as a task's name was, since the address built from it is sanitized
// anyway, as long as there is one and it is not a page long.
func Name(errs domain.ValidationErrors, field string, name string) {
	switch {
	case len(strings.TrimSpace(name)) == 0:
		errs[field] = "required_field"
	case utf8.RuneCountInString(name) > MaxNameLength:
		errs[field] = "exceeds_limit"
	}
}

// DockerName is the name of a docker object, which docker restricts. Empty is
// left to the caller to decide on, since some are optional.
func DockerName(errs domain.ValidationErrors, field string, name string) {
	if len(name) > 0 && (!dockerName.MatchString(name) || len(name) > MaxNameLength) {
		errs[field] = "invalid_name"
	}
}

// Image is a reference to an image, which is required and has no spaces in
// it. Whether it names an image that exists is the registry's to say.
func Image(errs domain.ValidationErrors, field string, reference string) {
	switch {
	case len(strings.TrimSpace(reference)) == 0:
		errs[field] = "required_field"
	case strings.ContainsFunc(reference, unicode.IsSpace):
		errs[field] = "invalid_image"
	}
}

// Ports are the guest ports a VM exposes: at most MaxPorts, each one from 1
// to 65535 and none of them twice. A port that is wrong is named by where it
// is in the list, as ports.2, so a form can say which one.
func Ports(errs domain.ValidationErrors, field string, ports []uint) {
	if len(ports) > MaxPorts {
		errs[field] = "too_many_ports"

		return
	}

	seen := make(map[uint]struct{}, len(ports))
	for i, p := range ports {
		switch _, twice := seen[p]; {
		case !IsPort(p):
			errs[Item(field, i)] = "invalid_port"
		case twice:
			errs[Item(field, i)] = "duplicate_port"
		}

		seen[p] = struct{}{}
	}
}

// IsPort reports whether p is a port at all.
func IsPort(p uint) bool {
	return p > 0 && p <= math.MaxUint16
}

// Item names one item of a list the way a validation error does: by the
// list's field and the item's place in it, as ports.0.
func Item(field string, index int) string {
	return field + "." + strconv.Itoa(index)
}

// Lifetime is how long a VM is kept, in seconds: zero is until it is
// deleted, and it is never negative.
func Lifetime(errs domain.ValidationErrors, field string, seconds int64) {
	if seconds < 0 || seconds > maxLifetimeSeconds {
		errs[field] = "invalid_lifetime"
	}
}

// LifetimeOf is a lifetime in seconds as the workload counts it.
func LifetimeOf(seconds int64) time.Duration {
	return time.Duration(seconds) * time.Second
}

// PortsOf are ports as the workload counts them, sorted, since a VM keeps
// its ports in order.
func PortsOf(ports []uint) []port.Port {
	if ports == nil {
		return nil
	}

	converted := make([]port.Port, len(ports))
	for i, p := range ports {
		converted[i] = port.Port(p)
	}

	slices.Sort(converted)

	return converted
}

// Resources are what a VM is given: whole vCPUs, and memory and disk in
// bytes.
type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

func (r Resources) VM() vm.Resources {
	return vm.Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

// Validate holds resources that are asked for whole to having all of them.
// A disk may be left to a snapshot, which says how big it has to be.
func (r Resources) Validate(errs domain.ValidationErrors, field string, diskOptional bool) {
	if r == (Resources{}) {
		errs[field] = "required_field"

		return
	}

	if r.CPUs == 0 {
		errs[field+".cpus"] = "greater_than_zero"
	}

	if r.Memory == 0 {
		errs[field+".memory"] = "greater_than_zero"
	}

	if r.Disk == 0 && !diskOptional {
		errs[field+".disk"] = "greater_than_zero"
	}
}

// Network is how much of the network a VM has, allow or deny each way:
// ingress lets the ingress reach its ports, egress lets it reach the public
// internet.
type Network struct {
	Ingress string `json:"ingress"`
	Egress  string `json:"egress"`
}

func (n Network) VM() vm.Network {
	return vm.Network{Ingress: vm.Access(n.Ingress), Egress: vm.Access(n.Egress)}
}

// Validate holds a network that is asked for whole to saying both ways.
func (n Network) Validate(errs domain.ValidationErrors, field string) {
	if n == (Network{}) {
		errs[field] = "required_field"

		return
	}

	access(errs, field+".ingress", n.Ingress, true)
	access(errs, field+".egress", n.Egress, true)
}

// ValidatePartly holds a network whose missing ways are left to a default.
func (n Network) ValidatePartly(errs domain.ValidationErrors, field string) {
	access(errs, field+".ingress", n.Ingress, false)
	access(errs, field+".egress", n.Egress, false)
}

func access(errs domain.ValidationErrors, field string, value string, required bool) {
	switch {
	case len(value) == 0 && required:
		errs[field] = "required_field"
	case len(value) > 0 && !vm.Access(value).IsValid():
		errs[field] = "invalid_access"
	}
}

// NewDockerVM is a Docker VM to make for a container or a stack, when there is
// none to put it in or a new one is wanted. Anything left out is the
// workload's Docker default.
type NewDockerVM struct {
	Name      string     `json:"name,omitempty"`
	Resources *Resources `json:"resources,omitempty"`
	Ports     []uint     `json:"ports,omitempty"`
	Network   *Network   `json:"network,omitempty"`
}

// DockerVM holds where a container or a stack is to go: a Docker VM named by
// its uuid, a new one described, or neither, which leaves the choice to the
// workload. Naming one and describing another is asking for two things.
func DockerVM(errs domain.ValidationErrors, vmUUID string, described *NewDockerVM) {
	if described == nil {
		return
	}

	if len(vmUUID) > 0 {
		errs["vm"] = "vm_or_new_vm"

		return
	}

	if len(described.Name) > 0 {
		Name(errs, "vm.name", described.Name)
	}

	Ports(errs, "vm.ports", described.Ports)

	if described.Network != nil {
		described.Network.ValidatePartly(errs, "vm.network")
	}
}

// DockerVMChoice is where a container or a stack is to go, as the workload is
// asked.
func DockerVMChoice(vmUUID string, described *NewDockerVM) workloadControlPlane.DockerVMChoice {
	choice := workloadControlPlane.DockerVMChoice{UUID: vmUUID}
	if described == nil {
		return choice
	}

	choice.New = &workloadControlPlane.NewDockerVM{
		Name:  described.Name,
		Ports: PortsOf(described.Ports),
	}

	if described.Resources != nil {
		resources := described.Resources.VM()
		choice.New.Resources = &resources
	}

	if described.Network != nil {
		network := described.Network.VM()
		choice.New.Network = &network
	}

	return choice
}

// DockerVMRefused is what the workload refused about the Docker VM a container
// or a stack was to go into, under the fields this request asked for it with.
//
// The control plane is asked for a VM to use as vm.uuid and for one to make as
// vm.new, and says so when it refuses either; the dashboard asks for them as
// vm_uuid and vm. So a VM that is not a Docker VM is refused under vm_uuid,
// and a new VM's memory under vm.resources.memory, where the form asked for
// it. Anything else is left where it was said.
func DockerVMRefused(refused domain.ValidationErrors) domain.ValidationErrors {
	if len(refused) == 0 {
		return refused
	}

	named := make(domain.ValidationErrors, len(refused))
	for field, reason := range refused {
		switch {
		case field == "vm.uuid":
			field = "vm_uuid"
		case field == "vm.new":
			field = "vm"
		case strings.HasPrefix(field, "vm.new."):
			field = "vm." + strings.TrimPrefix(field, "vm.new.")
		}

		named[field] = reason
	}

	return named
}
