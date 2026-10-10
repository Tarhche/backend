// Package volume is the volume kind's control-plane strategy: what a volume
// is admitted as, and what it is asked for when what it is doing is not what
// it was asked to be.
//
// A volume is admitted into the Docker VM it is asked for in, which has to be
// its owner's, and running or on its way up. What it is asked for after that:
//
//	expected  observed                          asked
//	present   pending or missing, its VM runs   create
//
// A volume made again after it went missing is empty, and its node says so
// as its reason for as long as it is there. A volume a container mounts is
// not removed, as docker would not remove it.
package volume

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
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

// Volumes is the volume kind's control-plane strategy.
type Volumes struct {
	*blocks.Blocks
}

var (
	_ kind.ControlPlane[volumeKind.Spec, volumeKind.Status] = &Volumes{}
	_ kind.Preparer[volumeKind.Spec, volumeKind.Status]     = &Volumes{}
	_ blocks.Kind                                           = &Volumes{}
)

// New is the strategy over what the building blocks share.
func New(dependencies blocks.Dependencies) *Volumes {
	v := &Volumes{}
	v.Blocks = blocks.New(volumeKind.Descriptor(), v, dependencies)

	return v
}

// Admit takes in a volume its owner asked for in one of their Docker VMs.
func (s *Volumes) Admit(ctx context.Context, asked volumeKind.Volume) (volumeKind.Volume, domain.ValidationErrors, error) {
	spec := asked.Spec
	spec.Name = strings.TrimSpace(spec.Name)

	switch {
	case len(spec.Name) == 0:
		return volumeKind.Volume{}, domain.ValidationErrors{"name": "required_field"}, nil
	case len(spec.Driver) > 0 && spec.Driver != "local":
		return volumeKind.Volume{}, domain.ValidationErrors{"driver": "invalid_volume_driver"}, nil
	}

	parent, _ := asked.Metadata.Owner(volumeKind.Parent)

	v, refused, err := s.Into(ctx, asked.Metadata.OwnerUUID, parent.UUID)
	if err != nil || len(refused) > 0 {
		return volumeKind.Volume{}, refused, err
	}

	return volumeKind.Volume{
		Kind: volumeKind.Name,
		Metadata: kind.Metadata{
			Name:      spec.Name,
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    asked.Metadata.Labels,
			Owners:    []kind.Reference{{Kind: volumeKind.Parent, UUID: v.Metadata.UUID}},
			Node:      v.Metadata.Node,
		},
		Spec:   spec,
		Status: volumeKind.Status{Status: kind.Status{State: volumeKind.Pending, Expected: volumeKind.Present}},
	}, nil, nil
}

// Reconcile makes a volume its VM does not have, once its VM runs: one made
// again is empty, and says so.
func (s *Volumes) Reconcile(ctx context.Context, v volumeKind.Volume) ([]kind.Intent, error) {
	return s.Present(ctx, v.Status.State, v.Status.Expected, volumeKind.VMOf(v))
}

// Apply refuses every action, since a volume has none that runs in the
// control plane.
func (s *Volumes) Apply(_ context.Context, _ volumeKind.Volume, action string, _ any) (volumeKind.Volume, domain.ValidationErrors, error) {
	return volumeKind.Volume{}, nil, fmt.Errorf("%w: a volume has no %q run in the control plane", kind.ErrUnknownAction, action)
}

// Prepare readies a volume's command, which Refuse refuses nothing of.
func (s *Volumes) Prepare(ctx context.Context, v volumeKind.Volume, action string, payload any) (volumeKind.Volume, domain.ValidationErrors, error) {
	raw, err := kind.Encode(v)
	if err != nil {
		return volumeKind.Volume{}, nil, err
	}

	if err := s.Refuse(ctx, raw, action, payload); err != nil {
		return volumeKind.Volume{}, nil, err
	}

	return v, nil, nil
}

// Refuse refuses a volume nothing. Whether a container mounts one is its
// dockerd's to say, which refuses its delete in its own words, whatever the
// force: what its node last reported of it may be from before a container
// that mounted it was removed a moment ago, which would refuse a delete
// docker would carry out.
func (s *Volumes) Refuse(context.Context, kind.Raw, string, any) error {
	return nil
}

// Adopt takes in a volume found in a restored VM labelled as the platform's
// with no record, as it is: its name, its driver, and the labels it was given
// beside the platform's own.
func (s *Volumes) Adopt(observed kind.Observation) (blocks.Adoption, bool) {
	var status volumeKind.Status
	if err := json.Unmarshal(observed.Status, &status); err != nil || status.Docker == nil || len(status.Docker.Name) == 0 {
		return blocks.Adoption{}, false
	}

	labels := maps.Clone(status.Docker.Labels)
	delete(labels, blockKinds.LabelManaged)
	delete(labels, blockKinds.Label(volumeKind.Name))
	delete(labels, volumeKind.LabelRecreated)

	if len(labels) == 0 {
		labels = nil
	}

	spec, err := json.Marshal(volumeKind.Spec{Name: status.Docker.Name, Driver: status.Docker.Driver, Labels: labels})
	if err != nil {
		return blocks.Adoption{}, false
	}

	return blocks.Adoption{Name: status.Docker.Name, Spec: spec, Expected: volumeKind.Present}, true
}

// Named reports whether name is the name a volume was asked for with.
func (s *Volumes) Named(r kind.Raw, name string) bool {
	var spec volumeKind.Spec
	if len(r.Spec) == 0 || json.Unmarshal(r.Spec, &spec) != nil {
		return false
	}

	return len(spec.Name) > 0 && spec.Name == name
}
