package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// tries is how many times a stack's record is read and written again, when
// something else wrote it in the meantime.
const tries = 3

// Graph is the stack kind as a graph needs it (graph.RootKind): a stack, the
// root of a graph of its own, known by its record's uuid, whose graph makes
// what its file says in its Docker VM, each named under its slug:
//
//	its Docker VM                      vm/<uuid>, as the stack wants it
//	each network it declares, and      network/<slug>_<name>
//	the default one a service joins    network/<slug>_default
//	by naming none
//	each volume it declares            volume/<slug>_<name>
//	each service                       container/<slug>_<service>, after
//	                                   those it depends on
//
// A service's container is asked for as the service says, on the stack's
// networks and with its volumes, each by the name the stack made it under,
// and reached by the service's name on each of its networks; it is labelled
// as the stack's and as the service it is, and owned by the stack from its
// admission on. Each container needs its VM, its networks and its volumes as
// the stack does, by the same nodes, so that the stack's graph makes each
// once, for every service that needs it.
//
// The stack is made last of all, once each of its containers is there: what
// it was asked is carried out first, its containers started a level of its
// services at a time, as they depend on one another, each level once the one
// before it runs, or stopped the other way round, or both, through each
// container's own commands; and then its record says, of each of its
// services, the container its graph made of it, by its uuid, and what it is
// doing, and the stack is what they make it (stackKind.StateOf). It is made
// while it is at rest and says that of them as they are; written again, it is
// written only when what they make it changed. What it was asked is said on
// its record before it is carried out, so a control plane that stops halfway
// through leaves it to the next to carry out where it was left. It has
// nothing external. Deleted, its record says it is being removed, and goes
// last of all, once what it owns is removed (graph.Ownership.Forget).
//
// A restore of its VM's disk resets it, as it resets what lives in the VM
// (graph.ResetKind): once the restored disk was looked into, and every word
// of that look heard, it is carried out again when the disk holds any of its
// containers, and forgotten, with what it owns, once it is known to hold none
// of them, no record of one left (Restored).
type Graph struct {
	records *requirements.Records

	// silence is how long what lives in a VM may go unheard while its node
	// beats before it is taken to be gone from it: what a stack its VM's
	// restore reset waits for, from the first look into the restored disk,
	// before it is found not to be there (Restored).
	silence time.Duration
}

var _ graph.ResetKind = &Graph{}

// NewGraph is the stack kind as a graph needs it, reading and writing stacks,
// and reading the containers their graphs made, through records; a stack its
// VM's restore reset is found not to be in the restored disk once what lives
// in the VM has gone unheard for silence since it was first looked into, as
// the reconcile loop takes what goes unheard for as long to be gone
// (reconcileResources.Config.ResourceSilentAfter).
func NewGraph(records *requirements.Records, silence time.Duration) *Graph {
	return &Graph{records: records, silence: silence}
}

// Root is the stack r is the record of, as the root of its graph needs it
// (WantOf). A record of another kind is kind.ErrUnknownKind.
func (g *Graph) Root(r kind.Raw) (graph.Requirement, error) {
	if r.Kind != stackKind.Name {
		return graph.Requirement{}, fmt.Errorf("%w: a %s is not a stack", kind.ErrUnknownKind, r.Kind)
	}

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r)
	if err != nil {
		return graph.Requirement{}, err
	}

	w, err := WantOf(s)
	if err != nil {
		return graph.Requirement{}, err
	}

	return w.Requirement(), nil
}

// WantOf is a stack as its graph needs it: known by its record's uuid, its
// owner's, under its slug, in the Docker VM its spec names by its uuid and
// whatever else its spec says of it, made of what its file says, and stopped
// when it is expected to be, as it was last asked. One in no VM named by its
// uuid, with no slug, or whose file cannot be read, is
// kind.ErrInvalidPayload.
func WantOf(s stackKind.Stack) (stackKind.Want, error) {
	switch {
	case len(s.Spec.VM.UUID) == 0:
		return stackKind.Want{}, fmt.Errorf("%w: the stack %s is in no vm named by its uuid", kind.ErrInvalidPayload, s.Metadata.UUID)
	case len(s.Metadata.Slug) == 0:
		return stackKind.Want{}, fmt.Errorf("%w: the stack %s has no slug to name what it makes under", kind.ErrInvalidPayload, s.Metadata.UUID)
	}

	file, invalid := stackKind.Parse(s.Spec.Compose)
	if len(invalid) > 0 {
		return stackKind.Want{}, fmt.Errorf("%w: the file of the stack %s cannot be read: %s", kind.ErrInvalidPayload, s.Metadata.UUID, said(invalid))
	}

	return stackKind.Want{
		UUID:      s.Metadata.UUID,
		OwnerUUID: s.Metadata.OwnerUUID,
		Slug:      s.Metadata.Slug,
		VM:        s.Spec.VM,
		File:      file,
		Stopped:   s.Status.Expected == stackKind.Stopped,
	}, nil
}

// Requires is what a stack needs directly: its Docker VM, wanted as it is
// wanted; each network it makes; each volume it declares; and a container
// for each of its services, made after those its service depends on.
func (g *Graph) Requires(spec any) ([]graph.Requirement, error) {
	w, err := wantOf(spec)
	if err != nil {
		return nil, err
	}

	required := []graph.Requirement{vmKind.Want{OwnerUUID: w.OwnerUUID, VM: w.VM}.Requirement()}

	for _, network := range w.File.NetworksMade() {
		required = append(required, networkKind.Want{OwnerUUID: w.OwnerUUID, VM: w.VM, Spec: networkKind.Spec{Name: w.Named(network)}}.Requirement())
	}

	for _, volume := range w.File.Volumes {
		required = append(required, volumeKind.Want{OwnerUUID: w.OwnerUUID, VM: w.VM, Spec: volumeKind.Spec{Name: w.Named(volume)}}.Requirement())
	}

	for _, service := range w.File.ServiceNames() {
		r := containerOf(w, service).Requirement()

		for _, dependency := range w.File.Services[service].DependsOn {
			r.After = append(r.After, containerID(w, dependency))
		}

		required = append(required, r)
	}

	return required, nil
}

// containerOf is the container a stack's graph makes of one of its services,
// as the service asks for it: on the stack's networks the service joins, and
// with its volumes, each named under the stack's slug, binds as they are; and
// reached by the service's name on each of its networks. It is labelled as the
// stack's and as the service it is, the stack owns it from its admission on,
// and it is made stopped while the stack is expected stopped, as the stack's
// stop holds each of its containers, so that one made again then does not
// start.
func containerOf(w stackKind.Want, service string) containerKind.Want {
	s := w.File.Services[service]

	spec := containerKind.Spec{
		Name:          w.Named(service),
		Image:         s.Image,
		Command:       slices.Clone(s.Command),
		Entrypoint:    slices.Clone(s.Entrypoint),
		Env:           slices.Clone(s.Environment),
		RestartPolicy: s.Restart,
		CPUs:          s.CPUs,
		Memory:        s.Memory,
		PidsLimit:     s.PidsLimit,
		Privileged:    s.Privileged,
		User:          s.User,
		Runtime:       s.Runtime,
	}

	for _, p := range s.Ports {
		spec.Ports = append(spec.Ports, containerKind.PortBinding{ContainerPort: p.Container, HostPort: p.Host, HostIP: p.HostIP, Protocol: p.Protocol})
	}

	for _, m := range s.Volumes {
		source := m.Source
		if m.Type == stackKind.MountVolume {
			source = w.Named(m.Source)
		}

		spec.Mounts = append(spec.Mounts, containerKind.Mount{Type: m.Type, Source: source, Target: m.Target, ReadOnly: m.ReadOnly})
	}

	for _, network := range s.Networks {
		named := w.Named(network)

		spec.Networks = append(spec.Networks, named)

		if spec.Aliases == nil {
			spec.Aliases = make(map[string][]string, len(s.Networks))
		}

		spec.Aliases[named] = []string{service}
	}

	return containerKind.Want{
		OwnerUUID: w.OwnerUUID,
		Labels:    map[string]string{stackKind.LabelStack: w.UUID, stackKind.LabelService: service},
		Owners:    []kind.Reference{{Kind: stackKind.Name, UUID: w.UUID}},
		VM:        w.VM,
		Spec:      spec,
		Stopped:   w.Stopped,
	}
}

// containerID is the node of the container a stack's graph makes of one of
// its services.
func containerID(w stackKind.Want, service string) graph.NodeID {
	return graph.ID(containerKind.Name, w.Named(service))
}

// Exists is the record of the stack the want names, whenever there is one,
// and whether it is made: not on its way out, at rest, running, degraded or
// stopped, and saying of its services what the records of the containers it
// names say of them now, and what they make it.
func (g *Graph) Exists(ctx context.Context, spec any) (graph.Ref, bool, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, false, err
	}

	r, there, err := g.records.Read(ctx, stackKind.Name, w.UUID)
	if err != nil || !there {
		return graph.Ref{}, false, err
	}

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r.Raw)
	if err != nil {
		return graph.Ref{}, false, err
	}

	if going(s) || !atRest(s.Status.State) {
		return requirements.RefOf(r), false, nil
	}

	containers, err := g.named(ctx, w, s.Status.Services)
	if err != nil {
		return graph.Ref{}, false, err
	}

	services := servicesOf(w, containers)

	return requirements.RefOf(r), sameServices(services, s.Status.Services) && stackKind.StateOf(services) == s.Status.State, nil
}

// Create makes the stack the want names, once its graph made each of its
// containers, handed among the records of what it needs: what it was asked,
// a start, a stop or a restart, is carried out (act), and so is what it was
// asked meanwhile, as its record says once it is read again; and then its
// record says of each of its services the container made of it, and what it
// is doing, and it is what they make it, written only when that changed. A
// stack whose record is gone is gone, with whatever was making it: a root is
// never admitted again. One on its way out waits.
func (g *Graph) Create(ctx context.Context, spec any, deps map[graph.NodeID]graph.Ref) (graph.Ref, error) {
	w, err := wantOf(spec)
	if err != nil {
		return graph.Ref{}, err
	}

	// what wrote its record in the meantime is read again.
	for try := 1; ; try++ {
		made, err := g.finish(ctx, w, deps)
		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		}

		return made, err
	}
}

// finish has the record of the stack w names say what its containers, among
// deps, make it, once what it was asked is carried out. Read again once that
// is done, the stack may have been asked something else meanwhile, a person's
// stop say, which is carried out then: what its containers make it is said
// only of the state it was last acted on as, and never over what it was asked
// after it was read, which would be lost. Asked something else each of tries
// times, it waits, and is carried out again.
func (g *Graph) finish(ctx context.Context, w stackKind.Want, deps map[graph.NodeID]graph.Ref) (graph.Ref, error) {
	r, s, err := g.stack(ctx, w)
	if err != nil {
		return graph.Ref{}, err
	}

	if going(s) {
		return graph.Ref{}, &graph.Waiting{Reason: "it is being removed"}
	}

	containers, err := g.made(ctx, w, deps)
	if err != nil {
		return graph.Ref{}, err
	}

	for round := 1; ; round++ {
		acted, err := g.act(ctx, w, r, s, containers)
		if err != nil {
			return graph.Ref{}, err
		}

		// what carrying it out took, the stack may have been asked meanwhile.
		if r, s, err = g.stack(ctx, w); err != nil {
			return graph.Ref{}, err
		}

		if going(s) {
			return graph.Ref{}, &graph.Waiting{Reason: "it is being removed"}
		}

		if s.Status.State == acted {
			break
		}

		if round == tries {
			return graph.Ref{}, &graph.Waiting{Reason: "it was asked something else each time what it was asked was carried out"}
		}
	}

	if err := g.settle(ctx, r, s, servicesOf(w, containers)); err != nil {
		return graph.Ref{}, err
	}

	return requirements.RefOf(r), nil
}

// act carries out what the stack s, whose record is r, was asked, on its
// containers, by their services, which are as it left them once it is
// carried out: started, stopped, or stopped and started again. It is the
// state it leaves the stack's record in: restarted, its record says it is
// starting once its containers are stopped, so that a restart cut short
// after that is taken up as the start it is then.
func (g *Graph) act(ctx context.Context, w stackKind.Want, r resource.Record, s stackKind.Stack, containers map[string]resource.Record) (kind.State, error) {
	switch s.Status.State {
	case stackKind.Starting:
		return s.Status.State, g.start(ctx, w, containers)
	case stackKind.Stopping:
		return s.Status.State, g.stop(ctx, w, containers)
	case stackKind.Restarting:
		if err := g.stop(ctx, w, containers); err != nil {
			return s.Status.State, err
		}

		if to, moved := stackKind.Machine().Next(s.Status.State, kind.OnObserved(stackKind.Starting)); moved && to != s.Status.State {
			s.Status.State = to
			s.Status.Since = g.records.Dispatcher.Now()

			if err := g.write(ctx, r, s); err != nil {
				return s.Status.State, err
			}
		}

		return s.Status.State, g.start(ctx, w, containers)
	}

	return s.Status.State, nil
}

// start starts the containers of the stack w names, by their services, that
// do not run, a level of its services at a time, in the order they depend on
// one another: each level once the one before it runs, each container through
// its own start, waited on until it is answered. One that runs already is
// expected to run. Those of every level after the first that do not run are
// held stopped first, expected so, so that what keeps a container as it was
// last asked, the reconcile loop, starts none of them before its turn. What a
// start fails with stops it there.
func (g *Graph) start(ctx context.Context, w stackKind.Want, containers map[string]resource.Record) error {
	levels := w.File.Levels()

	for _, level := range levels[min(1, len(levels)):] {
		for _, service := range level {
			if err := g.hold(ctx, containers, service, containerKind.Stopped, func(c containerKind.Container) bool { return !runs(c) }); err != nil {
				return err
			}
		}
	}

	for _, level := range levels {
		for _, service := range level {
			c, err := g.settled(ctx, containers, service)
			if err != nil {
				return err
			}

			if runs(c) {
				if err := g.hold(ctx, containers, service, containerKind.Running, func(containerKind.Container) bool { return true }); err != nil {
					return err
				}

				continue
			}

			if err := g.command(ctx, containers, service, containerKind.ActionStart, "started"); err != nil {
				return err
			}
		}
	}

	return nil
}

// stop stops the containers of the stack w names, by their services, that
// run, a level of its services at a time, in the reverse of the order they
// depend on one another, each through its own stop, waited on until it is
// answered; and has each that does not run expected stopped, so that nothing
// starts it again but its stack. What a stop fails with stops it there.
func (g *Graph) stop(ctx context.Context, w stackKind.Want, containers map[string]resource.Record) error {
	levels := w.File.Levels()
	slices.Reverse(levels)

	stoppable := containerKind.Descriptor()

	for _, level := range levels {
		for _, service := range level {
			c, err := g.settled(ctx, containers, service)
			if err != nil {
				return err
			}

			if !stoppable.Allows(containerKind.ActionStop, c.Status.Status.State) {
				if err := g.hold(ctx, containers, service, containerKind.Stopped, func(containerKind.Container) bool { return true }); err != nil {
					return err
				}

				continue
			}

			if err := g.command(ctx, containers, service, containerKind.ActionStop, "stopped"); err != nil {
				return err
			}
		}
	}

	return nil
}

// settled is the container of service, among containers, as its record is
// now, once what came of the command it waits on, when it waits on one, is
// heard: one being started or stopped is waited on. Its record among
// containers is as it is then.
func (g *Graph) settled(ctx context.Context, containers map[string]resource.Record, service string) (containerKind.Container, error) {
	r, there, err := g.records.Read(ctx, containerKind.Name, containers[service].Metadata.UUID)
	switch {
	case err != nil:
		return containerKind.Container{}, err
	case !there:
		return containerKind.Container{}, requirements.Gone(containers[service])
	}

	if r.Pending != nil {
		settled, went, err := g.records.Settle(ctx, r)
		switch {
		case err != nil:
			return containerKind.Container{}, err
		case went:
			return containerKind.Container{}, requirements.Gone(r)
		}

		r = settled
	}

	containers[service] = r

	return kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
}

// command asks the container of service, among containers, for one of its
// commands, and waits until it is answered: one that is not done, as done
// says, once it is, is an error saying what it is, and why. What wrote its
// record in the meantime is read again, and asked again.
func (g *Graph) command(ctx context.Context, containers map[string]resource.Record, service string, action string, done string) error {
	for try := 1; ; try++ {
		r, err := g.records.Ask(ctx, containers[service], action)
		if errors.Is(err, resource.ErrConflict) && try < tries {
			if _, err := g.settled(ctx, containers, service); err != nil {
				return err
			}

			continue
		}

		if err != nil {
			return err
		}

		containers[service] = r

		c, err := kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
		if err != nil {
			return err
		}

		if c.Status.Status.State == containerKind.Failed {
			return requirements.NotDone(r, done)
		}

		return nil
	}
}

// hold has the container of service, among containers, expected to be state,
// asking it for nothing, when it is not and which says it is to be: so that
// what keeps it as it was last asked keeps it so. What wrote it in the
// meantime is read again.
func (g *Graph) hold(ctx context.Context, containers map[string]resource.Record, service string, state kind.State, which func(c containerKind.Container) bool) error {
	for try := 1; ; try++ {
		r := containers[service]

		c, err := kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
		if err != nil {
			return err
		}

		if c.Status.Expected == state || !which(c) {
			return nil
		}

		held, err := g.records.Dispatcher.Desire(ctx, r, state, true)
		if err == nil {
			containers[service] = held

			return nil
		}

		if !errors.Is(err, resource.ErrConflict) || try >= tries {
			return err
		}

		latest, there, err := g.records.Read(ctx, r.Kind, r.Metadata.UUID)
		switch {
		case err != nil:
			return err
		case !there:
			return requirements.Gone(r)
		}

		containers[service] = latest
	}
}

// runs reports whether a container runs, as its node last said.
func runs(c containerKind.Container) bool {
	return c.Status.Status.State == containerKind.Running
}

// stack is the record of the stack w names, and the stack it is: one that is
// gone is gone, and never admitted again.
func (g *Graph) stack(ctx context.Context, w stackKind.Want) (resource.Record, stackKind.Stack, error) {
	r, there, err := g.records.Read(ctx, stackKind.Name, w.UUID)
	switch {
	case err != nil:
		return resource.Record{}, stackKind.Stack{}, err
	case !there:
		return resource.Record{}, stackKind.Stack{}, requirements.Gone(resource.Record{Raw: kind.Raw{Kind: stackKind.Name, Metadata: kind.Metadata{UUID: w.UUID}}})
	}

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r.Raw)
	if err != nil {
		return resource.Record{}, stackKind.Stack{}, err
	}

	return r, s, nil
}

// settle writes on the record r of the stack s what services make it: what
// each is doing, by the container made of it, and the state they make it, as
// its machine takes it from where it is; and nothing, when it says that
// already. What wrote it in the meantime is resource.ErrConflict.
func (g *Graph) settle(ctx context.Context, r resource.Record, s stackKind.Stack, services []stackKind.ServiceStatus) error {
	state := s.Status.State
	if to, moved := stackKind.Machine().Next(state, kind.OnObserved(stackKind.StateOf(services))); moved {
		state = to
	}

	if state == s.Status.State && sameServices(services, s.Status.Services) && (len(s.Status.Reason) == 0 || !atRest(state)) {
		return nil
	}

	now := g.records.Dispatcher.Now()

	if state != s.Status.State {
		s.Status.State = state
		s.Status.Since = now
	}

	if atRest(state) {
		s.Status.Reason = ""
	}

	s.Status.Services = services

	return g.write(ctx, r, s)
}

// write writes s, the stack r is the record of, over the version r was read
// at.
func (g *Graph) write(ctx context.Context, r resource.Record, s stackKind.Stack) error {
	raw, err := kind.Encode(s)
	if err != nil {
		return err
	}

	raw.Kind = r.Kind
	raw.Metadata.UUID = r.Metadata.UUID

	r.Raw = raw
	r.Metadata.UpdatedAt = g.records.Dispatcher.Now()

	_, err = g.records.Resources.Update(ctx, r)

	return err
}

// made are the records of the containers made of the services of the stack
// w names, by their services, as deps, what its graph made in this run, names
// them. One its graph has made nothing of, or whose record went since, is an
// error, and the stack is not made.
func (g *Graph) made(ctx context.Context, w stackKind.Want, deps map[graph.NodeID]graph.Ref) (map[string]resource.Record, error) {
	containers := make(map[string]resource.Record, len(w.File.Services))

	for _, service := range w.File.ServiceNames() {
		ref, made := deps[containerID(w, service)]
		if !made || ref.Kind != containerKind.Name {
			return nil, fmt.Errorf("no container of the service %s was made for it", service)
		}

		r, there, err := g.records.Read(ctx, containerKind.Name, ref.UUID)
		switch {
		case err != nil:
			return nil, err
		case !there:
			return nil, requirements.Gone(resource.Record{Raw: kind.Raw{Kind: containerKind.Name, Metadata: kind.Metadata{UUID: ref.UUID, Name: w.Named(service)}}})
		}

		containers[service] = r
	}

	return containers, nil
}

// named are the records of the containers services say are those of the
// services of the stack w names, by their services: each that is there, and
// is the container of the service its graph makes, by its name.
func (g *Graph) named(ctx context.Context, w stackKind.Want, services []stackKind.ServiceStatus) (map[string]resource.Record, error) {
	containers := make(map[string]resource.Record, len(services))

	for _, service := range services {
		if _, defined := w.File.Services[service.Name]; !defined || len(service.Containers) != 1 {
			continue
		}

		r, there, err := g.records.Read(ctx, containerKind.Name, service.Containers[0].ID)
		if err != nil {
			return nil, err
		}

		if there && r.Metadata.Name == w.Named(service.Name) {
			containers[service.Name] = r
		}
	}

	return containers, nil
}

// Restored is what the restored Docker VM of the stack the record r is was
// found to hold of it, once the VM's restore reset it (graph.ResetKind): what
// the restored disk holds of the stack's containers, which is known once the
// VM, running, was looked into since its restore, and was heard of for longer
// than what lives in it may go unheard since (silence), so that every
// heartbeat of that look has been heard, and what it found, or took in, is
// kept:
//
//   - held, when a container of one of the services of its file it owns is
//     no longer reset: one that look found in the restored disk, or one
//     taken in from it, which is the stack's again (kinds/container's
//     Adopt). A container of no service of its file says nothing of it: a
//     stack's spec is immutable, there being no update of a stack but a new
//     one, so its file never had that service, no run of its graph made
//     such a container, and none is taken in as its;
//   - not known still while one is reset: what the disk holds of a container
//     its record says, kept as it is found there, or forgotten once its node
//     goes on beating for as long as what lives in a VM may go unheard
//     without a word of it, while its VM runs (observe's Unheard), and a
//     heartbeat of it heard late keeps it reset as long;
//   - and lost once none is left: each forgotten by its own silence, none
//     found, none taken in. The restored disk is known not to hold the stack
//     then, and only then: a stack is forgotten on nothing less, since a
//     wait, which it shows as one, loses nothing of it.
//
// Until then it is not known, whatever its containers' records say: a stack
// none of whose containers was heard of since its VM's restore is never lost
// before its VM is looked into, a dockerd slow to come up after the restore
// keeping them unheard; nor is it carried out before every heartbeat of that
// look is heard, which would make again, beside what the restored disk holds,
// what was yet to be taken in from it. A container made since the restore
// would say the stack is held too, which is why nothing is made for a stack
// its VM's restore reset until this says, but for a create on its way as the
// restore landed, below. Nor is it known while its VM does
// not run, restoring, stopped or failed: nothing in it can be heard of then,
// and what lives in it waits on it, unheard and kept.
//
// Nor is it known while its VM's record says a restore before the one the
// stack was reset for, resetAt, the moment the VM's node says it carried that
// restore out (kind.Restore): what lives in the VM is reset before the
// restore is taken onto the VM's record (recordResult), and until then the
// record says what the restore before it said, nothing when it is the VM's
// first, which would hold the stack, its graph making again all of it on a
// disk that holds none of it; or, the restore before looked into long ago,
// would have the stack decided on a look into the disk before this one. The
// record takes that restore in the end, the result heard again, whatever its
// VM was asked, or failed as, meanwhile.
//
// One whose VM is gone, which nothing will look into again, or whose VM's
// record and reset say nothing of a restore, the reset made before resets
// said which, is held: what lives in it is its graph's to make again, as when
// its VM is deleted under it.
//
// What it cannot tell from what the restored disk held is a container of the
// stack whose create was sent before its VM's restore was asked, while the VM
// ran, and was carried out on the restored disk once its dockerd came up, its
// node waiting for that as it waits for any create: found there, it holds the
// stack, though the snapshot may hold none of it, so that a stack made since
// the snapshot lives on when one of its creates was on its way as the restore
// landed. Nothing its node says of the container tells the two apart but when
// docker says it was made, by the VM's own clock, which says nothing beside
// the restore's moment, by its node's; and no command says which disk it was
// sent for. Its graph sends no create while its VM is restoring, nor while
// it is reset, which narrows it to a create sent before the restore was asked
// and carried out only once the restore was.
func (g *Graph) Restored(ctx context.Context, r kind.Raw, resetAt time.Time) (graph.Restoration, error) {
	if r.Kind != stackKind.Name {
		return graph.RestorationUnknown, fmt.Errorf("%w: a %s is not a stack", kind.ErrUnknownKind, r.Kind)
	}

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r)
	if err != nil {
		return graph.RestorationUnknown, err
	}

	vr, there, err := g.records.Read(ctx, vmKind.Name, stackKind.VMOf(s))
	switch {
	case err != nil:
		return graph.RestorationUnknown, err
	case !there:
		return graph.RestorationHeld, nil
	}

	v, err := kind.Decode[vmKind.Spec, vmKind.Status](vr.Raw)
	if err != nil {
		return graph.RestorationUnknown, err
	}

	switch looked := v.Status.LookedAt; {
	case v.Status.RestoredAt.Before(resetAt):
		// the restore it was reset for is yet to be taken onto its VM's
		// record, which says the one before, if any.
		return graph.RestorationUnknown, nil
	case v.Status.State != vmKind.Running:
		return graph.RestorationUnknown, nil
	case v.Status.RestoredAt.IsZero():
		return graph.RestorationHeld, nil
	case looked.Before(v.Status.RestoredAt), v.Status.ObservedAt.Sub(looked) <= g.silence:
		return graph.RestorationUnknown, nil
	}

	containers, _, err := g.records.Resources.GetAll(ctx, containerKind.Name, resource.Filter{Parent: kind.Reference{Kind: stackKind.Name, UUID: s.Metadata.UUID}}, 0, 0)
	if err != nil {
		return graph.RestorationUnknown, err
	}

	// a container of none of its file's services says nothing of it, its
	// file never having had that service, which never changes; and of a
	// file that cannot be read, which says of none, each says something.
	services := make(map[string]bool)
	if w, err := WantOf(s); err == nil {
		for _, service := range w.File.ServiceNames() {
			services[w.Named(service)] = true
		}
	}

	left := false

	for _, c := range containers {
		switch {
		case len(services) > 0 && !services[c.Metadata.Name]:
		case !c.Reset:
			return graph.RestorationHeld, nil
		default:
			left = true
		}
	}

	if left {
		return graph.RestorationUnknown, nil
	}

	return graph.RestorationLost, nil
}

// Delete has the record of the stack ref names say it is being removed, as
// its machine takes its delete, and leaves it: what it owns is removed after
// it, and its record goes last of all, once that is done
// (graph.Ownership.Forget). One gone already is removed.
func (g *Graph) Delete(ctx context.Context, ref graph.Ref) error {
	if ref.Kind != stackKind.Name {
		return fmt.Errorf("%w: a %s is not a stack", kind.ErrUnknownKind, ref.Kind)
	}

	for try := 1; ; try++ {
		err := g.removing(ctx, ref)
		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		}

		return err
	}
}

// removing has the record of the stack ref names say it is being removed,
// once.
func (g *Graph) removing(ctx context.Context, ref graph.Ref) error {
	r, there, err := g.records.Read(ctx, ref.Kind, ref.UUID)
	if err != nil || !there {
		return err
	}

	s, err := kind.Decode[stackKind.Spec, stackKind.Status](r.Raw)
	if err != nil {
		return err
	}

	to, moved := stackKind.Machine().Next(s.Status.State, kind.OnAction(stackKind.ActionDelete))
	if !moved || to == s.Status.State {
		return nil
	}

	s.Status.State = to
	s.Status.Since = g.records.Dispatcher.Now()
	s.Status.Reason = ""

	err = g.write(ctx, r, s)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}

// servicesOf is what each service of the stack w names is doing, in the
// order of their names, as the record of the container made of it, among
// containers by their services, says: running when it runs; completed when
// it ran to its end, as it was to, and exited with nothing; stopped when it
// is there and does neither; and missing when there is none, or it is not
// made.
func servicesOf(w stackKind.Want, containers map[string]resource.Record) []stackKind.ServiceStatus {
	services := make([]stackKind.ServiceStatus, 0, len(w.File.Services))

	for _, name := range w.File.ServiceNames() {
		r, there := containers[name]
		if !there {
			services = append(services, stackKind.ServiceStatus{Name: name, State: stackKind.ServiceMissing})

			continue
		}

		services = append(services, serviceOf(name, r))
	}

	return services
}

// serviceOf is what the service name is doing, as the record r of the
// container made of it says.
func serviceOf(name string, r resource.Record) stackKind.ServiceStatus {
	service := stackKind.ServiceStatus{Name: name, State: stackKind.ServiceMissing}

	c, err := kind.Decode[containerKind.Spec, containerKind.Status](r.Raw)
	if err != nil {
		return service
	}

	seen := stackKind.ContainerStatus{ID: r.Metadata.UUID, Name: c.Spec.Name}
	if c.Status.Docker != nil {
		seen.State = c.Status.Docker.State
	}

	service.Containers = []stackKind.ContainerStatus{seen}

	switch c.Status.Status.State {
	case containerKind.Running, containerKind.Starting, containerKind.Restarting:
		service.State = stackKind.ServiceRunning
	case containerKind.Completed:
		service.State = stackKind.ServiceStopped

		if c.Status.Docker != nil && c.Status.Expected == containerKind.Running {
			if code, said := stackKind.ExitCode(c.Status.Docker.Status); said && code == 0 {
				service.State = stackKind.ServiceCompleted
			}
		}
	case containerKind.Stopped, containerKind.Stopping, containerKind.Waiting:
		service.State = stackKind.ServiceStopped
	case containerKind.Failed:
		if c.Status.Docker != nil {
			service.State = stackKind.ServiceStopped
		}
	}

	return service
}

// sameServices reports whether two say the same of a stack's services.
func sameServices(a []stackKind.ServiceStatus, b []stackKind.ServiceStatus) bool {
	return slices.EqualFunc(a, b, func(x stackKind.ServiceStatus, y stackKind.ServiceStatus) bool {
		return x.Name == y.Name && x.State == y.State && slices.Equal(x.Containers, y.Containers)
	})
}

// atRest reports whether a stack in state is at rest: what its containers
// make it, and nothing it is on its way to.
func atRest(state kind.State) bool {
	return state == stackKind.Running || state == stackKind.Degraded || state == stackKind.Stopped
}

// going reports whether a stack is on its way out: asked to go, expected
// deleted, or deleted.
func going(s stackKind.Stack) bool {
	return requirements.Going(s.Status.Status) || !s.Metadata.DeletionRequestedAt.IsZero()
}

// wantOf is spec as a stack a graph needs, known by its record's uuid, in a
// Docker VM named by its uuid, under its slug.
func wantOf(spec any) (stackKind.Want, error) {
	w, err := requirements.Want[stackKind.Want](stackKind.Name, spec)
	if err != nil {
		return stackKind.Want{}, err
	}

	switch {
	case len(w.UUID) == 0:
		return stackKind.Want{}, fmt.Errorf("%w: a stack is required by its uuid", kind.ErrInvalidPayload)
	case len(w.VM.UUID) == 0:
		return stackKind.Want{}, fmt.Errorf("%w: a stack is required in a vm named by its uuid", kind.ErrInvalidPayload)
	case len(w.Slug) == 0:
		return stackKind.Want{}, fmt.Errorf("%w: a stack is required under a slug", kind.ErrInvalidPayload)
	}

	return w, nil
}

// said is what is wrong with a stack's file, each field and its code, in the
// order of the fields.
func said(invalid domain.ValidationErrors) string {
	fields := slices.Sorted(maps.Keys(invalid))

	each := make([]string, len(fields))
	for i, field := range fields {
		each[i] = field + ": " + invalid[field]
	}

	return strings.Join(each, ", ")
}
