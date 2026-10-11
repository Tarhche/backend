// Package container is the container kind's control-plane strategy: what a
// container is admitted as, what it is asked for when what it is doing is
// not what it was asked to be, and what is shown beside the containers kept.
//
// A container is admitted into the Docker VM it names, which has to be its
// owner's and running or on its way up, or, when it names none, into one its
// graph is to make for it: given a uuid now, and held now to what the vm kind
// would admit it as, but made by its graph, before the container. Either way
// it keeps the VM's uuid and what to make the VM with again whenever it is
// gone, and what it names that its VM has already, which is not its own
// (Spec.External). It belongs to that VM and is placed where it is, once the
// VM is placed, and it starts pending.
//
// A container nothing owns is the root of a graph of its own (Graph): its
// graph makes its VM, the networks and volumes it needs and then it, in
// order, makes each again whenever it is gone, but what is not its own, and
// removes it, and the networks it owns, once it is asked to go. What it is
// asked for beside that is decided from what its node last said of it:
//
//	expected  observed                          asked
//	any       pending or missing                nothing: its graph makes it
//	any       waiting                           nothing: its VM's own reconcile
//	stopped   running                           stop
//	running   stopped                           start
//	running   completed                         nothing: a one-off that ended
//	any       off a network its spec names      connect
//
// A command docker refuses fails it, in docker's own words, and it is asked
// again with the loop's backoff. One that runs is not removed but by force.
//
// What a Docker VM's dockerd holds that the platform did not make, the
// containers made from the VM's terminal, is shown beside the records, and
// can be started, stopped, restarted, removed and read, through its node, but
// is never reconciled (kinds/blocks). A stack's containers are records, which
// its graph makes.
package container

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
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
	_ kind.Outliver[containerKind.Spec, containerKind.Status]     = &Containers{}
	_ blocks.Kind                                                 = &Containers{}
)

// New is the strategy over what the building blocks share.
func New(dependencies blocks.Dependencies) *Containers {
	c := &Containers{}
	c.Blocks = blocks.New(containerKind.Descriptor(), c, dependencies)

	return c
}

// Admit takes in a container its owner asked for: its spec checked, in the
// Docker VM it names, which has to be theirs and running or on its way up, or
// in one its graph is to make for it when it names none, held now to what the
// vm kind would admit it as. What it keeps of its VM is the VM's uuid and what
// to make it with again whenever it is gone, as the VM's record has it, or as
// it was asked for with the defaults filled in; and one nothing owns, the
// root of its graph, keeps what it names that its VM has already as not its
// own.
func (s *Containers) Admit(ctx context.Context, asked containerKind.Container) (containerKind.Container, domain.ValidationErrors, error) {
	choice := asked.Spec.VM
	if parent, named := asked.Metadata.Owner(containerKind.Parent); named && len(choice.UUID) == 0 && !choice.Describes() {
		choice.UUID = parent.UUID
	}

	if invalid := validate(asked.Spec, choice); len(invalid) > 0 {
		return containerKind.Container{}, invalid, nil
	}

	v, there, refused, err := s.vmOf(ctx, asked.Metadata.OwnerUUID, choice)
	if err != nil || len(refused) > 0 {
		return containerKind.Container{}, refused, err
	}

	spec := asked.Spec
	spec.Image = strings.TrimSpace(spec.Image)
	spec.VM = vmKind.ChoiceOf(v)

	admitted := containerKind.Container{
		Kind: containerKind.Name,
		Metadata: kind.Metadata{
			Name:      spec.Name,
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    labelled(asked.Metadata),
			Owners:    slices.Clone(asked.Metadata.Owners),
			Refs:      kind.LivingIn(asked.Metadata.Refs, kind.Reference{Kind: containerKind.Parent, UUID: v.Metadata.UUID}),
			Node:      v.Metadata.Node,
		},
		Spec:   spec,
		Status: containerKind.Status{Status: kind.Status{State: containerKind.Pending, Expected: containerKind.Running}},
	}

	switch {
	case !there:
		admitted.Status.Reason = "its vm is being made"
	case v.Status.State != vmKind.Running:
		admitted.Status.Reason = fmt.Sprintf("its vm is %s", v.Status.State)
	}

	// a root keeps what was there before it as not its own, which its graph
	// never makes or removes: nothing is in a VM still to be made.
	if len(asked.Metadata.Owning(containerKind.Parent)) == 0 {
		admitted.Spec.External = nil

		if there {
			if admitted.Spec.External, err = s.external(ctx, v.Metadata.UUID, spec); err != nil {
				return containerKind.Container{}, nil, err
			}
		}
	}

	return admitted, nil, nil
}

// stackLabels are the labels of a container a stack owns that say which
// stack's, and which of its services, it is, which its VM's dockerd labels it
// with too.
var stackLabels = []string{stackKind.LabelStack, stackKind.LabelService}

// labelled are the labels a container is kept with, as m, which it was asked
// for with, has them: but for those that say it is a stack's service, unless
// a stack owns it, which its stack's graph says of the containers it makes.
// Nobody else says a container is a stack's, which its VM's dockerd would
// otherwise label it as.
func labelled(m kind.Metadata) map[string]string {
	if _, stacked := m.Owner(stackKind.Name); stacked {
		return m.Labels
	}

	var labels map[string]string

	for key, value := range m.Labels {
		if slices.Contains(stackLabels, key) {
			continue
		}

		if labels == nil {
			labels = make(map[string]string, len(m.Labels))
		}

		labels[key] = value
	}

	return labels
}

// vmOf is the Docker VM a container of ownerUUID's goes into, as choice says,
// and whether it is there: the one choice names, which has to be theirs and
// running or on its way up; or, when it names none, one its graph is to make
// for it, as choice describes it, planned now under a uuid of its own
// (dockervm.Chooser.Plan), which is not there yet.
func (s *Containers) vmOf(ctx context.Context, ownerUUID string, choice stackKind.VMChoice) (vmKind.VM, bool, domain.ValidationErrors, error) {
	if len(choice.UUID) == 0 {
		v, refused, err := s.Chooser.Plan(ctx, ownerUUID, choiceOf(choice))

		return v, false, refused, err
	}

	v, refused, err := s.Chooser.Choose(ctx, ownerUUID, dockervm.Choice{UUID: choice.UUID})
	switch {
	case err != nil || len(refused) > 0:
		return vmKind.VM{}, false, refused, err
	case !vmKind.Up(v):
		return vmKind.VM{}, false, domain.ValidationErrors{"vm": "not_running"}, nil
	}

	return v, true, nil, nil
}

// external are what of the networks a container joins and the volumes it
// mounts by their names the Docker VM vmUUID names has already, each as the
// node of a graph it is, network/<name> or volume/<name>, once: what was there
// before it, made by somebody else, which its graph never makes or removes
// for it, and which it waits on while one is missing.
func (s *Containers) external(ctx context.Context, vmUUID string, spec containerKind.Spec) ([]string, error) {
	var external []string

	held := func(kindName string, name string) error {
		id := graph.ID(kindName, name).String()
		if len(strings.TrimSpace(name)) == 0 || slices.Contains(external, id) {
			return nil
		}

		there, err := s.holds(ctx, kindName, vmUUID, name)
		if err == nil && there {
			external = append(external, id)
		}

		return err
	}

	for _, network := range spec.Networks {
		if networkKind.Default(network) {
			continue
		}

		if err := held(networkKind.Name, network); err != nil {
			return nil, err
		}
	}

	for _, m := range spec.Mounts {
		if m.Type != containerKind.MountVolume {
			continue
		}

		if err := held(volumeKind.Name, m.Source); err != nil {
			return nil, err
		}
	}

	return external, nil
}

// holds reports whether the Docker VM vmUUID names has what of the named kind
// is called name, its Docker name: kept by a record that is not on its way
// out, which one of the same name may be about to take the place of, or held
// by its dockerd with nobody keeping it, as its node last said.
func (s *Containers) holds(ctx context.Context, kindName string, vmUUID string, name string) (bool, error) {
	kept, _, err := s.Resources.GetAll(ctx, kindName, resource.Filter{Parent: kind.Reference{Kind: containerKind.Parent, UUID: vmUUID}}, 0, 0)
	if err != nil {
		return false, err
	}

	for i := range kept {
		if !called(kept[i].Raw, name) {
			continue
		}

		common, err := kept[i].Common()
		if err != nil {
			return false, err
		}

		if common.Expected != kind.Deleted && common.State != kind.Deleted && kept[i].Metadata.DeletionRequestedAt.IsZero() {
			return true, nil
		}
	}

	return slices.ContainsFunc(s.Sightings.In(kindName, vmUUID), func(r kind.Raw) bool { return called(r, name) }), nil
}

// owns reports whether c owns what of the named kind its Docker VM keeps a
// record of under name, its Docker name: what its graph made for it, which
// names it among its owners.
func (s *Containers) owns(ctx context.Context, c containerKind.Container, kindName string, name string) (bool, error) {
	kept, _, err := s.Resources.GetAll(ctx, kindName, resource.Filter{Parent: kind.Reference{Kind: containerKind.Parent, UUID: containerKind.VMOf(c)}}, 0, 0)
	if err != nil {
		return false, err
	}

	owner := kind.Reference{Kind: containerKind.Name, UUID: c.Metadata.UUID}

	return slices.ContainsFunc(kept, func(r resource.Record) bool {
		return called(r.Raw, name) && slices.Contains(r.Metadata.Owners, owner)
	}), nil
}

// called reports whether a network or a volume of a Docker VM is called name:
// the name it was asked for with, or the one its VM's dockerd last said it
// has.
func called(r kind.Raw, name string) bool {
	var spec struct {
		Name string `json:"name"`
	}

	if len(r.Spec) > 0 && json.Unmarshal(r.Spec, &spec) == nil && spec.Name == name {
		return true
	}

	return blocks.ObservedOf(r.Status).Docker.Name == name
}

// Reconcile is what a container is asked for, given what it was asked to be
// and what it was last seen doing. One that is not made, its graph makes, in
// order, after what it needs.
func (s *Containers) Reconcile(_ context.Context, c containerKind.Container) ([]kind.Intent, error) {
	state, expected := c.Status.Status.State, c.Status.Expected

	switch {
	case expected != containerKind.Running && expected != containerKind.Stopped:
		return nil, nil

	// not made yet, or gone: its graph makes it, after what it needs.
	case state == containerKind.Pending || state == containerKind.Missing:
		return nil, nil

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
	case seen == nil, c.Status.Status.State != containerKind.Running && c.Status.Status.State != containerKind.Stopped && c.Status.Status.State != containerKind.Completed:
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
// are what it is put back on. A network it is put on is there already, and is
// not its own (Spec.External) unless its graph made it for it; one it is
// taken off it names no more, and is one its spec names, or that its VM's
// dockerd last said it is on.
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

		own, err := s.owns(ctx, c, networkKind.Name, connect.Network)
		if err != nil {
			return containerKind.Container{}, nil, err
		}

		if !slices.Contains(c.Spec.Networks, connect.Network) {
			c.Spec.Networks = append(slices.Clone(c.Spec.Networks), connect.Network)
		}

		if external := graph.ID(networkKind.Name, connect.Network).String(); !own && !networkKind.Default(connect.Network) && !slices.Contains(c.Spec.External, external) {
			c.Spec.External = append(slices.Clone(c.Spec.External), external)
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

		// on its way onto it, the spec naming it before its node's word that
		// it is on it is taken onto its record, or named by a spec it could
		// not be put back on, it is taken off it all the same: only one
		// neither says it is on is not.
		if seen := c.Status.Docker; !slices.Contains(c.Spec.Networks, network) && (seen == nil || !slices.Contains(seen.Networks, network)) {
			name := c.Spec.Name
			if seen != nil && len(seen.Name) > 0 {
				name = seen.Name
			}

			return containerKind.Container{}, nil, blocks.Refused("container %s is not connected to network %s", name, disconnect.Network)
		}

		disconnect.Network = network

		c.Spec.Networks = slices.DeleteFunc(slices.Clone(c.Spec.Networks), func(network string) bool { return network == disconnect.Network })

		if external := graph.ID(networkKind.Name, disconnect.Network).String(); slices.Contains(c.Spec.External, external) {
			c.Spec.External = slices.DeleteFunc(slices.Clone(c.Spec.External), func(id string) bool { return id == external })
			if len(c.Spec.External) == 0 {
				c.Spec.External = nil
			}
		}

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

// Outlive is a container kept as gone with its Docker VM, whose graph makes
// the VM again under its uuid: what it keeps of its VM, and makes it again
// with, is the VM as its record had it as it went (vmKind.ChoiceOf), the name,
// resources, ports and network an update or a reconfigure gave it since it
// was named, and the slug it was reached under, which it asks for again. A
// parent that is no VM, or not its VM, says nothing of it.
func (s *Containers) Outlive(c containerKind.Container, parent kind.Raw) (containerKind.Container, error) {
	if parent.Kind != containerKind.Parent || parent.Metadata.UUID != containerKind.VMOf(c) {
		return c, nil
	}

	v, err := kind.Decode[vmKind.Spec, vmKind.Status](parent)
	if err != nil {
		return containerKind.Container{}, err
	}

	c.Spec.VM = vmKind.ChoiceOf(v)

	return c, nil
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
// stopped. One its labels say is a service of a stack whose record is there,
// in the same VM, is the stack's again, as its stack's graph makes the
// container of a service (stackOf): owned by the stack, labelled as the
// stack's and as the service, and named <slug>_<service>, which the stack's
// graph knows it by. One whose labels name a stack that is not there, or is
// in another VM, is nobody's, as any other is: its labels confer nothing.
func (s *Containers) Adopt(ctx context.Context, observed kind.Observation) (blocks.Adoption, bool, error) {
	var status containerKind.Status
	if err := json.Unmarshal(observed.Status, &status); err != nil || status.Docker == nil {
		return blocks.Adoption{}, false, nil
	}

	spec, labelled := containerKind.SpecFromLabels(status.Docker.Labels)
	if !labelled {
		return blocks.Adoption{}, false, nil
	}

	parent, in := vmOf(observed.Owners)
	if in {
		spec.VM = stackKind.VMChoice{UUID: parent}
	}

	expected := containerKind.Running
	if status.Status.State == containerKind.Stopped {
		expected = containerKind.Stopped
	}

	adoption := blocks.Adoption{Expected: expected}

	stacked, err := s.stackOf(ctx, parent, status.Docker.Labels)
	if err != nil {
		return blocks.Adoption{}, false, err
	}

	if stacked != nil {
		spec.Name = stacked.name
		adoption.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: stacked.uuid}}
		adoption.Labels = map[string]string{stackKind.LabelStack: stacked.uuid, stackKind.LabelService: stacked.service}
	}

	encoded, err := json.Marshal(spec)
	if err != nil {
		return blocks.Adoption{}, false, nil
	}

	adoption.Name = cmp.Or(spec.Name, status.Docker.Name)
	adoption.Spec = encoded

	return adoption, true, nil
}

// stacked is the service of a stack a container is.
type stacked struct {
	uuid    string
	service string

	// name is the Docker name the stack's graph knows the container of the
	// service by: <slug>_<service>.
	name string
}

// stackOf is the service of a stack a container in the Docker VM vmUUID
// names is, as its labels say, when they say it of a stack whose record is
// there, in that VM: nothing when they say nothing of a stack, or name one
// that is not there, or is in another VM. What reading the stack fails with
// is an error.
func (s *Containers) stackOf(ctx context.Context, vmUUID string, labels map[string]string) (*stacked, error) {
	uuid, service := labels[stackKind.LabelStack], labels[stackKind.LabelService]
	if len(uuid) == 0 || len(service) == 0 || len(vmUUID) == 0 {
		return nil, nil
	}

	r, err := s.Resources.GetOne(ctx, stackKind.Name, uuid)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return nil, nil
	case err != nil:
		return nil, err
	}

	st, err := kind.Decode[stackKind.Spec, stackKind.Status](r.Raw)
	if err != nil {
		return nil, err
	}

	if stackKind.VMOf(st) != vmUUID || len(st.Metadata.Slug) == 0 {
		return nil, nil
	}

	return &stacked{uuid: uuid, service: service, name: stackKind.Want{Slug: st.Metadata.Slug}.Named(service)}, nil
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

		if len(p.HostIP) > 0 && !isIP(p.HostIP) {
			invalid[fmt.Sprintf("ports[%d].host_ip", i)] = "invalid_ip"
		}
	}

	if spec.PidsLimit < 0 {
		invalid["pids_limit"] = "invalid_value"
	}

	if len(choice.UUID) > 0 && choice.Describes() {
		invalid["vm"] = "vm_or_new_vm"
	}

	return invalid
}

// isIP reports whether address is one a port can be published on: an IPv4
// or an IPv6 address, with no zone.
func isIP(address string) bool {
	parsed, err := netip.ParseAddr(address)

	return err == nil && len(parsed.Zone()) == 0
}

// choiceOf is a container's choice of VM as the chooser reads one.
func choiceOf(choice stackKind.VMChoice) dockervm.Choice {
	chosen := dockervm.Choice{UUID: choice.UUID, Name: choice.Name, Ports: choice.Ports}

	if choice.Resources != nil {
		chosen.Resources = &vmKind.Resources{CPUs: choice.Resources.CPUs, Memory: choice.Resources.Memory, Disk: choice.Resources.Disk}
	}

	if choice.Network != nil {
		chosen.Network = &vmKind.Network{Ingress: vm.Access(choice.Network.Ingress), Egress: vm.Access(choice.Network.Egress)}
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
