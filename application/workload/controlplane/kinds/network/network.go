// Package network is the network kind's control-plane strategy: what a
// network is admitted as, and what it is asked for when what it is doing is
// not what it was asked to be.
//
// A network is admitted into the Docker VM it is asked for in, which has to be
// its owner's, and running or on its way up. What it is asked for after that:
//
//	expected  observed                          asked
//	present   pending or missing, its VM runs   create
//
// Which containers are on it is theirs to say: a container kept names the
// networks it is on, and is put back on them. A network with containers on
// it is not removed, as docker would not remove it.
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
)

// Networks is the network kind's control-plane strategy.
type Networks struct {
	*blocks.Blocks
}

var (
	_ kind.ControlPlane[networkKind.Spec, networkKind.Status] = &Networks{}
	_ kind.Preparer[networkKind.Spec, networkKind.Status]     = &Networks{}
	_ blocks.Kind                                             = &Networks{}
)

// New is the strategy over what the building blocks share.
func New(dependencies blocks.Dependencies) *Networks {
	n := &Networks{}
	n.Blocks = blocks.New(networkKind.Descriptor(), n, dependencies)

	return n
}

// Admit takes in a network its owner asked for in one of their Docker VMs.
func (s *Networks) Admit(ctx context.Context, asked networkKind.Network) (networkKind.Network, domain.ValidationErrors, error) {
	spec := asked.Spec
	spec.Name = strings.TrimSpace(spec.Name)

	switch {
	case len(spec.Name) == 0:
		return networkKind.Network{}, domain.ValidationErrors{"name": "required_field"}, nil
	case len(spec.Driver) > 0 && spec.Driver != "bridge":
		return networkKind.Network{}, domain.ValidationErrors{"driver": "invalid_network_driver"}, nil
	}

	parent, _ := asked.Metadata.Owner(networkKind.Parent)

	v, refused, err := s.Into(ctx, asked.Metadata.OwnerUUID, parent.UUID)
	if err != nil || len(refused) > 0 {
		return networkKind.Network{}, refused, err
	}

	return networkKind.Network{
		Kind: networkKind.Name,
		Metadata: kind.Metadata{
			Name:      spec.Name,
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    asked.Metadata.Labels,
			Owners:    []kind.Reference{{Kind: networkKind.Parent, UUID: v.Metadata.UUID}},
			Node:      v.Metadata.Node,
		},
		Spec:   spec,
		Status: networkKind.Status{Status: kind.Status{State: networkKind.Pending, Expected: networkKind.Present}},
	}, nil, nil
}

// Reconcile makes a network its VM does not have, once its VM runs.
func (s *Networks) Reconcile(ctx context.Context, n networkKind.Network) ([]kind.Intent, error) {
	return s.Present(ctx, n.Status.State, n.Status.Expected, networkKind.VMOf(n))
}

// Apply refuses every action, since a network has none that runs in the
// control plane.
func (s *Networks) Apply(_ context.Context, _ networkKind.Network, action string, _ any) (networkKind.Network, domain.ValidationErrors, error) {
	return networkKind.Network{}, nil, fmt.Errorf("%w: a network has no %q run in the control plane", kind.ErrUnknownAction, action)
}

// Prepare readies a network's command: one every dockerd has of its own is
// refused its delete, as Refuse says.
func (s *Networks) Prepare(ctx context.Context, n networkKind.Network, action string, payload any) (networkKind.Network, domain.ValidationErrors, error) {
	raw, err := kind.Encode(n)
	if err != nil {
		return networkKind.Network{}, nil, err
	}

	if err := s.Refuse(ctx, raw, action, payload); err != nil {
		return networkKind.Network{}, nil, err
	}

	return n, nil, nil
}

// Refuse is why a network, kept or not, is not removed, in docker's own
// words: one every dockerd has of its own. Whether a container is on it is
// its dockerd's to say, which refuses its delete in the same words: what its
// node last reported of it may be from before a container was taken off it a
// moment ago, which would refuse a delete docker would carry out.
func (s *Networks) Refuse(_ context.Context, r kind.Raw, action string, _ any) error {
	if action != networkKind.ActionDelete {
		return nil
	}

	seen := blocks.ObservedOf(r.Status).Docker

	if networkKind.Default(seen.Name) {
		return blocks.Refused("%s is a pre-defined network and cannot be removed", seen.Name)
	}

	return nil
}

// Adopt takes in a network found in a restored VM labelled as the
// platform's with no record, as it is: its name, its driver, whether it is
// internal, and the labels it was given beside the platform's own.
func (s *Networks) Adopt(observed kind.Observation) (blocks.Adoption, bool) {
	var status networkKind.Status
	if err := json.Unmarshal(observed.Status, &status); err != nil || status.Docker == nil || len(status.Docker.Name) == 0 {
		return blocks.Adoption{}, false
	}

	labels := maps.Clone(status.Docker.Labels)
	delete(labels, blockKinds.LabelManaged)
	delete(labels, blockKinds.Label(networkKind.Name))

	if len(labels) == 0 {
		labels = nil
	}

	spec, err := json.Marshal(networkKind.Spec{Name: status.Docker.Name, Driver: status.Docker.Driver, Internal: status.Docker.Internal, Labels: labels})
	if err != nil {
		return blocks.Adoption{}, false
	}

	return blocks.Adoption{Name: status.Docker.Name, Spec: spec, Expected: networkKind.Present}, true
}

// Named reports whether name is the name a network was asked for with.
func (s *Networks) Named(r kind.Raw, name string) bool {
	var spec networkKind.Spec
	if len(r.Spec) == 0 || json.Unmarshal(r.Spec, &spec) != nil {
		return false
	}

	return len(spec.Name) > 0 && spec.Name == name
}
