// Package network is the network kind: a docker network in a Docker VM,
// stored and reconciled, declared once for every service that runs it
// (domain/workload/kind).
//
// It is a building block of its Docker VM (domain/workload/kinds/blocks): it
// lives in the VM, goes with it, and is reset to what a restored disk holds.
// The platform labels the network it makes as this one, so what the VM's
// dockerd holds is matched to its record exactly.
//
// Its spec is its name, its driver and whether it is internal, none of which
// changes. Which containers are on it is theirs to say: a container names the
// networks it is on, and is put back on them. What is decided of it:
//
//	expected  observed                       asked
//	present   pending or missing, VM running  create
package network

import (
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

const (
	// Name is the kind's word, and Plural what its routes are named by. Its
	// permissions are the containers': workload.containers.<verb>.
	Name   = "network"
	Plural = "networks"

	// Parent is the kind a network lives in: a Docker VM.
	Parent = blocks.Parent
)

// A network's states: those of a building block that is either there or
// not (blocks.Presence).
const (
	Pending  = blocks.Pending
	Creating = blocks.Creating
	Present  = blocks.Present
	Removing = blocks.Removing

	Waiting = kind.Waiting
	Missing = kind.Missing
	Failed  = kind.Failed
	Deleted = kind.Deleted
)

// A network's actions.
const (
	// ActionCreate makes a network that is not there. It is the workload's
	// own to ask for.
	ActionCreate = blocks.ActionCreate

	ActionDelete = blocks.ActionDelete
	ActionState  = blocks.ActionState
)

// Spec is a network as it is asked for.
type Spec struct {
	Name string `json:"name"`

	// Driver is bridge, which is also what empty is.
	Driver string `json:"driver,omitempty"`

	// Internal keeps the containers on it from reaching anything outside it.
	Internal bool `json:"internal,omitempty"`

	// Labels are what it is labelled with beside the platform's own.
	Labels map[string]string `json:"labels,omitempty"`
}

// Docker is the network spec as docker is asked to make it, labelled as the
// platform's, as the resource uuid names.
func (s Spec) Docker(uuid string) docker.NetworkSpec {
	labels := blocks.Labels(Name, uuid)
	for key, value := range s.Labels {
		if _, platform := labels[key]; !platform {
			labels[key] = value
		}
	}

	return docker.NetworkSpec{Name: s.Name, Driver: s.Driver, Internal: s.Internal, Labels: labels}
}

// Status is what a network is: its state, and what its VM's dockerd last said
// of it.
type Status struct {
	kind.Status

	// Docker is the network as its VM's dockerd last had it, and nothing
	// until it is made.
	Docker *Docker `json:"docker,omitempty"`

	// Failure is what the last command on it failed with, in the codes every
	// side knows; an empty one is a command that did not fail since.
	Failure *noderequest.Error `json:"failure,omitempty"`
}

// Docker is a network as its VM's dockerd has it.
type Docker struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Scope    string `json:"scope"`
	Internal bool   `json:"internal"`

	// Containers are the containers attached to it, by their names.
	Containers []string `json:"containers"`

	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitzero"`
}

// DockerOf is a network as docker has it, as a status has it.
func DockerOf(n docker.Network) *Docker {
	containers := append([]string{}, n.Containers...)

	return &Docker{ID: n.ID, Name: n.Name, Driver: n.Driver, Scope: n.Scope, Internal: n.Internal, Containers: containers, Labels: n.Labels, CreatedAt: n.CreatedAt}
}

// Network is it as docker has it.
func (d *Docker) Network() docker.Network {
	if d == nil {
		return docker.Network{}
	}

	return docker.Network{
		ID:         d.ID,
		Name:       d.Name,
		Driver:     d.Driver,
		Scope:      d.Scope,
		Internal:   d.Internal,
		Containers: append([]string{}, d.Containers...),
		Labels:     d.Labels,
		CreatedAt:  d.CreatedAt,
	}
}

// Network is a network, as its strategies are handed one.
type Network = kind.Resource[Spec, Status]

// VMOf is the uuid of the Docker VM a network lives in: its parent.
func VMOf(n Network) string {
	if parent, ok := n.Metadata.Owner(Parent); ok {
		return parent.UUID
	}

	return ""
}

// Default reports whether a network is one every dockerd has of its own,
// which nobody makes and nobody removes: bridge, host and none.
func Default(name string) bool {
	switch strings.TrimSpace(name) {
	case "bridge", "host", "none":
		return true
	}

	return false
}

// Descriptor is the network kind, a new one every time.
func Descriptor() kind.Descriptor {
	return kind.Descriptor{
		Name:          Name,
		Plural:        Plural,
		PermissionsOf: blocks.PermissionsOf,
		StateBy:       kind.OnNode,
		Parent:        Parent,
		OnParent:      blocks.OnParent(),
		Machine:       Machine(),
		Actions: []kind.Action{
			{Name: ActionCreate, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Pending, Missing}, Internal: true, Payload: kind.NoPayload},
			{Name: ActionDelete, Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: ActionState, Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// Machine is a network's states and the moves between them: those of a
// building block that is either there or not.
func Machine() kind.Machine {
	return blocks.Presence()
}
