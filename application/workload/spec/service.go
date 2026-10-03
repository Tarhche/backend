// Package spec reads a task's specification in the shape a docker compose
// service has, so a block of a compose file can be handed to the workload as it
// stands.
//
// Compose accepts several shapes for the same field — a command as a string or
// a list, an environment as a map or a list of "K=V" — and this package takes
// all of them, then normalises them into what the domain works in.
package spec

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Service is one task, in a compose service's shape.
type Service struct {
	Image string `json:"image"`

	// Runtime is the class the task is run with — sysbox, firecracker —
	// under compose's own runtime: key, which is where a compose file says
	// what a container is run under. Naming none leaves it to the workload's
	// default. Which classes may be named is the workload's to say as well,
	// so only the shape of a name is checked here.
	Runtime runtime.Class `json:"runtime,omitempty"`

	Command     StringOrSlice `json:"command,omitempty"`
	Entrypoint  StringOrSlice `json:"entrypoint,omitempty"`
	WorkingDir  string        `json:"working_dir,omitempty"`
	Environment Environment   `json:"environment,omitempty"`
	Ports       Ports         `json:"ports,omitempty"`
	Restart     string        `json:"restart,omitempty"`

	// ReadOnly makes the task's filesystem immutable, so nothing it runs
	// can change the image it was started from. It is compose's read_only.
	ReadOnly bool `json:"read_only,omitempty"`

	// NetworkMode is how much of the network the task reaches: "none",
	// "isolated" or "public". It is not docker's own network_mode — the workload
	// decides which networks a task joins — but it sits in the same place
	// a compose file puts that decision.
	NetworkMode string `json:"network_mode,omitempty"`

	Deploy Deploy `json:"deploy"`
}

// Deploy carries the resource limits and the restart policy, where a compose
// file puts them.
type Deploy struct {
	Resources     Resources     `json:"resources"`
	RestartPolicy RestartPolicy `json:"restart_policy"`
}

// RestartPolicy is how hard the workload tries to make a task what it was
// asked to be.
type RestartPolicy struct {
	// MaxAttempts is how many times a task that failed is asked for
	// again. Nothing at all leaves it to the workload, zero is not at all, and
	// -1 never gives up.
	MaxAttempts *int `json:"max_attempts,omitempty"`
}

type Resources struct {
	Limits Limits `json:"limits"`
}

// Limits accepts compose's own units: cpus as a decimal string or number, and
// memory and disk as a size like "256M", which is held in bytes from the moment
// it is read.
type Limits struct {
	CPUs   Decimal  `json:"cpus,omitempty"`
	Memory ByteSize `json:"memory,omitempty"`
	Disk   ByteSize `json:"disk,omitempty"`
}

// the restart policies docker accepts.
var restartPolicies = map[string]struct{}{
	"":               {},
	"no":             {},
	"always":         {},
	"on-failure":     {},
	"unless-stopped": {},
}

// Validate reports what is wrong with a service, under the field names the
// client sent. The prefix names the service inside a stack, and is empty for a
// task that stands on its own.
func (s *Service) Validate(prefix string) domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	field := func(name string) string {
		if len(prefix) == 0 {
			return name
		}

		return prefix + "." + name
	}

	if len(s.Image) == 0 {
		validationErrors[field("image")] = "required_field"
	}

	if len(s.Runtime) > 0 && !s.Runtime.IsValid() {
		validationErrors[field("runtime")] = "invalid_value"
	}

	if _, ok := restartPolicies[s.Restart]; !ok {
		validationErrors[field("restart")] = "invalid_value"
	}

	policy := s.NetworkPolicy()
	if !policy.IsValid() {
		validationErrors[field("network_mode")] = "invalid_network_policy"
	}

	for _, p := range s.Ports {
		if p.Task == 0 {
			validationErrors[field("ports")] = "invalid_value"

			break
		}
	}

	// a task with no network has nothing to publish a port on, so asking
	// for both is a contradiction rather than something to silently drop.
	if len(s.Ports) > 0 && policy.IsValid() && !policy.AllowsPorts() {
		validationErrors[field("ports")] = "ports_require_network"
	}

	limits := s.Deploy.Resources.Limits
	switch {
	case limits.CPUs < 0 || limits.Memory < 0 || limits.Disk < 0:
		validationErrors[field("deploy.resources.limits")] = "invalid_value"

	// docker will not create a container with less memory than this. Saying
	// so here, before anything is created, is what keeps a stack from being
	// stood up halfway and a task from failing on its node over a typo.
	case limits.Memory > 0 && limits.Memory < task.MinMemory:
		validationErrors[field("deploy.resources.limits")] = "memory_below_minimum"
	}

	if attempts := s.Deploy.RestartPolicy.MaxAttempts; attempts != nil && *attempts < task.RetryForever {
		validationErrors[field("deploy.restart_policy.max_attempts")] = "invalid_value"
	}

	return validationErrors
}

// NetworkPolicy is the policy this service asked for, or the default when it
// asked for none.
func (s *Service) NetworkPolicy() network.Policy {
	if len(s.NetworkMode) == 0 {
		return network.DefaultPolicy
	}

	return network.Policy(s.NetworkMode)
}

// ExposedPorts are the task ports the workload publishes. Only the task
// side of a compose port is honoured: the workload picks the host port itself,
// and serves it on the task's own hostname.
func (s *Service) ExposedPorts() []port.Port {
	seen := make(map[port.Port]struct{}, len(s.Ports))
	ports := make([]port.Port, 0, len(s.Ports))

	for _, p := range s.Ports {
		if _, duplicate := seen[p.Task]; duplicate {
			continue
		}

		seen[p.Task] = struct{}{}
		ports = append(ports, p.Task)
	}

	return ports
}

// ResourceLimits are the limits this service asked for, with the defaults
// filled in where it asked for nothing.
func (s *Service) ResourceLimits(defaults task.ResourceLimits) task.ResourceLimits {
	limits := task.ResourceLimits{
		Cpu:    float64(s.Deploy.Resources.Limits.CPUs),
		Memory: uint64(s.Deploy.Resources.Limits.Memory),
		Disk:   uint64(s.Deploy.Resources.Limits.Disk),
	}

	if limits.Cpu <= 0 {
		limits.Cpu = defaults.Cpu
	}

	if limits.Memory == 0 {
		limits.Memory = defaults.Memory
	}

	if limits.Disk == 0 {
		limits.Disk = defaults.Disk
	}

	return limits
}
