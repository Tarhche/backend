// Package microsandbox runs VMs on microsandbox: the engine a vmhost embeds and
// serves to its orchestrator.
//
// microsandbox has no server of its own. Its SDK is a cgo runtime that owns the
// VMs it starts, so the engine lives in the one process that may hold it, inside
// the microsandbox container, and is built only with the microsandbox build tag.
// Every other build gets a New that says so, which keeps the rest of the module
// static.
package microsandbox

import (
	"context"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Engine is a vm.Engine, together with what its host does with it when it is
// going away.
type Engine interface {
	vm.Engine

	// Shutdown stops every VM it holds gracefully, all at once, and gives up on
	// those still running when ctx ends. A VM stopped this way loses nothing it
	// wrote; one killed with its container loses what the guest had not yet
	// flushed.
	Shutdown(ctx context.Context) error
}

// Options are what a node's engine is given.
type Options struct {
	// Home is where the engine keeps its VMs, images and its own records of
	// them. It has to outlive the container.
	Home string

	// BindAddress is this vmhost's own IP on its pair network. A VM's published
	// ports are bound to it, and an Endpoint's address is it.
	BindAddress string

	// OrchestratorAddress is the only address a VM's published ports take a
	// connection from. Whether anybody outside may reach a VM is the
	// orchestrator's proxy's decision, not the engine's.
	OrchestratorAddress string

	// FirstPort and LastPort bound the host ports a VM's published ports are
	// given. What a VM is given it keeps, across restarts and restores.
	FirstPort port.Port
	LastPort  port.Port

	// DockerImage is what a Docker VM boots from, and what tells one: an
	// instance whose image is it, or another tag of it, is booted as a Docker
	// VM, and said to be one (vm.KindOf, vm.LabelDocker). Nothing else asks
	// for one.
	DockerImage string

	// Capacity is the budget this node offers to VMs. An instance that would
	// take it past it is refused with vm.ErrNoCapacity.
	Capacity vm.Resources

	// MaxConcurrentBoots bounds how many VMs boot at once.
	MaxConcurrentBoots uint

	Logger *slog.Logger
}
