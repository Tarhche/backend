// Package container is the container kind's control-plane strategy: what a
// container is admitted as, what it is asked for when what it is doing is
// not what it was asked to be, and what is shown beside the containers kept.
//
// A container is admitted into the Docker VM the stacks' rules choose, or
// make: the one it names, its owner's only one, or one made for it. It
// belongs to that VM and is placed where it is, and it starts pending, to be
// made as soon as its VM runs.
//
// What it is asked for after that is decided from what its node last said of
// it:
//
//	expected  observed                          asked
//	any       pending or missing, its VM runs   create, which pulls its image
//	any       waiting                           nothing: its VM's own reconcile
//	stopped   running                           stop
//	running   stopped                           start
//	running   completed                         nothing: a one-off that ended
//	any       off a network its spec names      connect
//
// A command docker refuses fails it, in docker's own words, and it is asked
// again with the loop's backoff. One that runs is not removed but by force.
//
// What a Docker VM's dockerd holds that the platform did not make, a stack's
// containers and those made from the VM's terminal, is shown beside the
// records, and can be started, stopped, restarted, removed and read, through
// its node, but is never reconciled (kinds/blocks).
package container

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Containers is the container kind's control-plane strategy.
type Containers struct {
	*blocks.Blocks
}

var (
	_ kind.ControlPlane[containerKind.Spec, containerKind.Status] = &Containers{}
	_ kind.Preparer[containerKind.Spec, containerKind.Status]     = &Containers{}
	_ blocks.Kind                                                 = &Containers{}
)

// New is the strategy over what the building blocks share.
func New(dependencies blocks.Dependencies) *Containers {
	c := &Containers{}
	c.Blocks = blocks.New(containerKind.Descriptor(), c, dependencies)

	return c
}

// Admit takes in a container its owner asked for: its spec checked, and its
// Docker VM chosen, or made, which has to be running or on its way up.
func (s *Containers) Admit(ctx context.Context, asked containerKind.Container) (containerKind.Container, domain.ValidationErrors, error) {
	choice := asked.Spec.VM
	if parent, named := asked.Metadata.Owner(containerKind.Parent); named && len(choice.UUID) == 0 && choice.New == nil {
		choice.UUID = parent.UUID
	}

	if invalid := validate(asked.Spec, choice); len(invalid) > 0 {
		return containerKind.Container{}, invalid, nil
	}

	chosen, refused, err := s.Chooser.Choose(ctx, asked.Metadata.OwnerUUID, choiceOf(choice))
	if err != nil || len(refused) > 0 {
		return containerKind.Container{}, refused, err
	}

	v := chosen.VM

	if !vmKind.Up(v) {
		return containerKind.Container{}, domain.ValidationErrors{"vm": "not_running"}, nil
	}

	spec := asked.Spec
	spec.Image = strings.TrimSpace(spec.Image)
	spec.VM = stackKind.VMChoice{UUID: v.Metadata.UUID}

	// what it was made with, or nothing more than the defaults: either way,
	// that it was made for this container.
	if chosen.Created {
		spec.VM.New = &stackKind.NewVM{}
		if choice.New != nil {
			spec.VM.New = choice.New
		}
	}

	admitted := containerKind.Container{
		Kind: containerKind.Name,
		Metadata: kind.Metadata{
			Name:      spec.Name,
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    asked.Metadata.Labels,
			Owners:    []kind.Reference{{Kind: containerKind.Parent, UUID: v.Metadata.UUID}},
			Node:      v.Metadata.Node,
		},
		Spec:   spec,
		Status: containerKind.Status{Status: kind.Status{State: containerKind.Pending, Expected: containerKind.Running}},
	}

	if v.Status.State != vmKind.Running {
		admitted.Status.Reason = fmt.Sprintf("its vm is %s", v.Status.State)
	}

	return admitted, nil, nil
}

// Reconcile is what a container is asked for, given what it was asked to be
// and what it was last seen doing.
func (s *Containers) Reconcile(ctx context.Context, c containerKind.Container) ([]kind.Intent, error) {
	state, expected := c.Status.State, c.Status.Expected

	switch {
	case expected != containerKind.Running && expected != containerKind.Stopped:
		return nil, nil

	case state == containerKind.Pending || state == containerKind.Missing:
		running, err := s.Running(ctx, containerKind.VMOf(c))
		if err != nil || !running {
			return nil, err
		}

		return []kind.Intent{{Action: containerKind.ActionCreate, Reason: "it is not in its vm, which runs"}}, nil

	case expected == containerKind.Stopped && state == containerKind.Running:
		return []kind.Intent{{Action: containerKind.ActionStop, Reason: "it runs while it was expected stopped"}}, nil

	case expected == containerKind.Running && state == containerKind.Stopped:
		return []kind.Intent{{Action: containerKind.ActionStart, Reason: "it stopped while it was expected running, and docker did not start it again"}}, nil
	}

	return attachments(c), nil
}

// attachments are what puts a container that is there back on the networks
// its spec names, one at a time: a network made again after it went missing
// has none of the containers that were on it.
func attachments(c containerKind.Container) []kind.Intent {
	seen := c.Status.Docker

	switch {
	case seen == nil, c.Status.State != containerKind.Running && c.Status.State != containerKind.Stopped && c.Status.State != containerKind.Completed:
		return nil
	}

	for _, network := range c.Spec.Networks {
		if !slices.Contains(seen.Networks, network) {
			return []kind.Intent{{
				Action:  containerKind.ActionConnect,
				Payload: containerKind.ConnectPayload{Network: network, Aliases: c.Spec.Aliases[network]},
				Reason:  fmt.Sprintf("it is not on %s, which its spec names", network),
			}}
		}
	}

	return nil
}

// Apply refuses every action, since a container has none that runs in the
// control plane.
func (s *Containers) Apply(_ context.Context, _ containerKind.Container, action string, _ any) (containerKind.Container, domain.ValidationErrors, error) {
	return containerKind.Container{}, nil, fmt.Errorf("%w: a container has no %q run in the control plane", kind.ErrUnknownAction, action)
}

// Prepare readies a container's command: one that runs is not removed but by
// force, which is refused as docker would refuse it; and putting it on a
// network, or taking it off one, changes the networks its spec names, which
// are what it is put back on.
func (s *Containers) Prepare(ctx context.Context, c containerKind.Container, action string, payload any) (containerKind.Container, domain.ValidationErrors, error) {
	switch action {
	case containerKind.ActionDelete:
		raw, err := kind.Encode(c)
		if err != nil {
			return containerKind.Container{}, nil, err
		}

		if err := s.Refuse(ctx, raw, action, payload); err != nil {
			return containerKind.Container{}, nil, err
		}

	case containerKind.ActionConnect:
		connect, _ := payload.(containerKind.ConnectPayload)

		network, err := s.connectable(ctx, c, connect.Network)
		if err != nil {
			return containerKind.Container{}, nil, err
		}

		connect.Network = network

		if !slices.Contains(c.Spec.Networks, connect.Network) {
			c.Spec.Networks = append(slices.Clone(c.Spec.Networks), connect.Network)
		}

		aliases := make(map[string][]string, len(c.Spec.Aliases)+1)
		for network, names := range c.Spec.Aliases {
			aliases[network] = names
		}

		delete(aliases, connect.Network)

		if len(connect.Aliases) > 0 {
			aliases[connect.Network] = slices.Clone(connect.Aliases)
		}

		c.Spec.Aliases = nil
		if len(aliases) > 0 {
			c.Spec.Aliases = aliases
		}

	case containerKind.ActionDisconnect:
		disconnect, _ := payload.(containerKind.DisconnectPayload)

		network, found, err := s.networkNamed(ctx, containerKind.VMOf(c), disconnect.Network)
		switch {
		case err != nil:
			return containerKind.Container{}, nil, err
		case !found:
			return containerKind.Container{}, nil, &noderequest.Error{Code: noderequest.CodeNotFound, Message: fmt.Sprintf("network %s not found", disconnect.Network)}
		}

		if seen := c.Status.Docker; seen != nil && !slices.Contains(seen.Networks, network) {
			return containerKind.Container{}, nil, blocks.Refused("container %s is not connected to network %s", seen.Name, disconnect.Network)
		}

		disconnect.Network = network

		c.Spec.Networks = slices.DeleteFunc(slices.Clone(c.Spec.Networks), func(network string) bool { return network == disconnect.Network })

		if _, aliased := c.Spec.Aliases[disconnect.Network]; aliased {
			aliases := make(map[string][]string, len(c.Spec.Aliases))
			for network, names := range c.Spec.Aliases {
				if network != disconnect.Network {
					aliases[network] = names
				}
			}

			c.Spec.Aliases = nil
			if len(aliases) > 0 {
				c.Spec.Aliases = aliases
			}
		}
	}

	return c, nil, nil
}

// connectable is the name of the network a kept container is put on, by
// its name or its Docker id, or why it is not put on it, as docker would
// refuse it, before its spec names it: a network its VM does not have is not
// found, and one it is on already has it.
func (s *Containers) connectable(ctx context.Context, c containerKind.Container, network string) (string, error) {
	name, found, err := s.networkNamed(ctx, containerKind.VMOf(c), network)

	switch {
	case err != nil:
		return "", err
	case !found:
		return "", &noderequest.Error{Code: noderequest.CodeNotFound, Message: fmt.Sprintf("network %s not found", network)}
	}

	if seen := c.Status.Docker; seen != nil && slices.Contains(seen.Networks, name) {
		return "", blocks.Refused("endpoint with name %s already exists in network %s", seen.Name, name)
	}

	return name, nil
}

// networkNamed is the name a Docker VM has the network network names, by
// its name or its Docker id, and whether it has one: what a container's
// spec, and docker's listing of a container, name a network by.
func (s *Containers) networkNamed(ctx context.Context, vmUUID string, network string) (string, bool, error) {
	if networkKind.Default(network) {
		return network, true, nil
	}

	kept, _, err := s.Resources.GetAll(ctx, networkKind.Name, resource.Filter{Parent: kind.Reference{Kind: networkKind.Parent, UUID: vmUUID}}, 0, 0)
	if err != nil {
		return "", false, err
	}

	networks := make([]kind.Raw, 0, len(kept))
	for i := range kept {
		networks = append(networks, kept[i].Raw)
	}

	networks = append(networks, s.Sightings.In(networkKind.Name, vmUUID)...)

	for _, n := range networks {
		var spec networkKind.Spec
		if len(n.Spec) > 0 {
			_ = json.Unmarshal(n.Spec, &spec)
		}

		seen := blocks.ObservedOf(n.Status).Docker

		switch {
		case spec.Name == network, seen.Name == network:
			return network, true, nil
		case len(seen.ID) > 0 && seen.ID == network:
			return cmp.Or(seen.Name, spec.Name), true, nil
		}
	}

	return "", false, nil
}

// Refuse is why a container, kept or not, is not asked for action as it is:
// one that runs is not removed but by force, in docker's own words.
func (s *Containers) Refuse(_ context.Context, r kind.Raw, action string, payload any) error {
	if action != containerKind.ActionDelete {
		return nil
	}

	if remove, _ := payload.(containerKind.DeletePayload); remove.Force {
		return nil
	}

	seen := blocks.ObservedOf(r.Status).Docker

	switch seen.State {
	case "running", "restarting", "paused":
	default:
		return nil
	}

	return blocks.Refused("cannot remove container %q: container is running: stop the container before removing or force remove", "/"+seen.Name)
}

// Adopt takes in a container found in a restored VM labelled as the
// platform's with no record: as the spec it was made with, which its labels
// carry, expected to be doing what it is doing, unless that is having been
// stopped.
func (s *Containers) Adopt(observed kind.Observation) (blocks.Adoption, bool) {
	var status containerKind.Status
	if err := json.Unmarshal(observed.Status, &status); err != nil || status.Docker == nil {
		return blocks.Adoption{}, false
	}

	spec, labelled := containerKind.SpecFromLabels(status.Docker.Labels)
	if !labelled {
		return blocks.Adoption{}, false
	}

	if parent, in := vmOf(observed.Owners); in {
		spec.VM = stackKind.VMChoice{UUID: parent}
	}

	encoded, err := json.Marshal(spec)
	if err != nil {
		return blocks.Adoption{}, false
	}

	expected := containerKind.Running
	if status.State == containerKind.Stopped {
		expected = containerKind.Stopped
	}

	name := spec.Name
	if len(name) == 0 {
		name = status.Docker.Name
	}

	return blocks.Adoption{Name: name, Spec: encoded, Expected: expected}, true
}

// Named reports whether name is the name a container was asked for with.
func (s *Containers) Named(r kind.Raw, name string) bool {
	var spec containerKind.Spec
	if len(r.Spec) == 0 || json.Unmarshal(r.Spec, &spec) != nil {
		return false
	}

	return len(spec.Name) > 0 && spec.Name == name
}

// validate is what is wrong with a container as it was asked for, of what
// the control plane can tell: everything else is docker's to refuse.
func validate(spec containerKind.Spec, choice stackKind.VMChoice) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	if len(strings.TrimSpace(spec.Image)) == 0 {
		invalid["image"] = "required_field"
	}

	switch spec.RestartPolicy {
	case "", containerKind.RestartNo, containerKind.RestartAlways, containerKind.RestartUnlessStopped, containerKind.RestartOnFailure:
	default:
		invalid["restart_policy"] = "invalid_restart_policy"
	}

	for i, p := range spec.Ports {
		if p.ContainerPort == 0 {
			invalid[fmt.Sprintf("ports[%d].container_port", i)] = "invalid_port"
		}
	}

	if len(choice.UUID) > 0 && choice.New != nil {
		invalid["vm"] = "vm_or_new_vm"
	}

	return invalid
}

// choiceOf is a container's choice of VM as the chooser reads one.
func choiceOf(choice stackKind.VMChoice) dockervm.Choice {
	chosen := dockervm.Choice{UUID: choice.UUID}

	if made := choice.New; made != nil {
		chosen.New = &dockervm.New{Name: made.Name, Ports: made.Ports}

		if made.Resources != nil {
			chosen.New.Resources = &vmKind.Resources{CPUs: made.Resources.CPUs, Memory: made.Resources.Memory, Disk: made.Resources.Disk}
		}

		if made.Network != nil {
			chosen.New.Network = &vmKind.Network{Ingress: vm.Access(made.Network.Ingress), Egress: vm.Access(made.Network.Egress)}
		}
	}

	return chosen
}

// vmOf is the Docker VM owners name, if they name one.
func vmOf(owners []kind.Reference) (string, bool) {
	for _, owner := range owners {
		if owner.Kind == containerKind.Parent {
			return owner.UUID, true
		}
	}

	return "", false
}
