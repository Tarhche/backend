package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

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
// The stack is made last of all, once each of its containers is there: its
// record says, of each of its services, the container its graph made of it,
// by its uuid, and what it is doing, and the stack is what they make it
// (stackKind.StateOf). It is made while it says that of them as they are,
// at rest; written again, it is written only when what they make it changed.
// It has nothing external. Deleted, its record says it is being removed, and
// goes last of all, once what it owns is removed (graph.Ownership.Forget).
type Graph struct {
	records *requirements.Records
}

var _ graph.RootKind = &Graph{}

// NewGraph is the stack kind as a graph needs it, reading and writing stacks,
// and reading the containers their graphs made, through records.
func NewGraph(records *requirements.Records) *Graph {
	return &Graph{records: records}
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
// whatever else its spec says of it, made of what its file says. One in no VM
// named by its uuid, with no slug, or whose file cannot be read, is
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
// stack's and as the service it is, and the stack owns it from its admission
// on.
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
// containers, handed among the records of what it needs: its record says of
// each of its services the container made of it, and what it is doing, and
// it is what they make it, written only when that changed. A stack whose
// record is gone is gone, with whatever was making it: a root is never
// admitted again. One on its way out waits.
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
// deps, make it, once.
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

	if err := g.settle(ctx, r, s, servicesOf(w, containers)); err != nil {
		return graph.Ref{}, err
	}

	return requirements.RefOf(r), nil
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
