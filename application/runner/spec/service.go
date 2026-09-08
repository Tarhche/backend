// Package spec reads a container's specification in the shape a docker compose
// service has, so a block of a compose file can be handed to the runner as it
// stands.
//
// Compose accepts several shapes for the same field — a command as a string or
// a list, an environment as a map or a list of "K=V" — and this package takes
// all of them, then normalises them into what the domain works in.
package spec

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Service is one container, in a compose service's shape.
type Service struct {
	Image       string        `json:"image"`
	Command     StringOrSlice `json:"command,omitempty"`
	Entrypoint  StringOrSlice `json:"entrypoint,omitempty"`
	WorkingDir  string        `json:"working_dir,omitempty"`
	Environment Environment   `json:"environment,omitempty"`
	Ports       Ports         `json:"ports,omitempty"`
	Restart     string        `json:"restart,omitempty"`

	// ReadOnly makes the container's filesystem immutable, so nothing it runs
	// can change the image it was started from. It is compose's read_only.
	ReadOnly bool `json:"read_only,omitempty"`

	// NetworkMode is how much of the network the container reaches: "none",
	// "isolated" or "public". It is not docker's own network_mode — the runner
	// decides which networks a container joins — but it sits in the same place
	// a compose file puts that decision.
	NetworkMode string `json:"network_mode,omitempty"`

	Deploy Deploy `json:"deploy,omitempty"`
}

// Deploy carries the resource limits and the restart policy, where a compose
// file puts them.
type Deploy struct {
	Resources     Resources     `json:"resources,omitempty"`
	RestartPolicy RestartPolicy `json:"restart_policy,omitempty"`
}

// RestartPolicy is how hard the runner tries to make a container what it was
// asked to be.
type RestartPolicy struct {
	// MaxAttempts is how many times a container that failed is asked for
	// again. Nothing at all leaves it to the runner, zero is not at all, and
	// -1 never gives up.
	MaxAttempts *int `json:"max_attempts,omitempty"`
}

type Resources struct {
	Limits Limits `json:"limits,omitempty"`
}

// Limits accepts compose's own units: cpus as a decimal string or number, and
// memory as a size like "256M".
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
// container that stands on its own.
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

	if _, ok := restartPolicies[s.Restart]; !ok {
		validationErrors[field("restart")] = "invalid_value"
	}

	policy := s.NetworkPolicy()
	if !policy.IsValid() {
		validationErrors[field("network_mode")] = "invalid_network_policy"
	}

	for _, p := range s.Ports {
		if p.Container == 0 {
			validationErrors[field("ports")] = "invalid_value"

			break
		}
	}

	// a container with no network has nothing to publish a port on, so asking
	// for both is a contradiction rather than something to silently drop.
	if len(s.Ports) > 0 && policy.IsValid() && !policy.AllowsPorts() {
		validationErrors[field("ports")] = "ports_require_network"
	}

	if s.Deploy.Resources.Limits.CPUs < 0 || s.Deploy.Resources.Limits.Memory < 0 || s.Deploy.Resources.Limits.Disk < 0 {
		validationErrors[field("deploy.resources.limits")] = "invalid_value"
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

// ExposedPorts are the container ports the runner publishes. Only the container
// side of a compose port is honoured: the runner picks the host port itself,
// and serves it on the container's own hostname.
func (s *Service) ExposedPorts() []port.Port {
	seen := make(map[port.Port]struct{}, len(s.Ports))
	ports := make([]port.Port, 0, len(s.Ports))

	for _, p := range s.Ports {
		if _, duplicate := seen[p.Container]; duplicate {
			continue
		}

		seen[p.Container] = struct{}{}
		ports = append(ports, p.Container)
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
