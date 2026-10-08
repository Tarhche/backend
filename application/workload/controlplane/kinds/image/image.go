// Package image is the image kind's control-plane strategy: what an image is
// admitted as, and what it is asked for when what it is doing is not what it
// was asked to be.
//
// An image is admitted into the Docker VM it is asked for in, which has to be
// its owner's, and running or on its way up, under the uuid worked out from
// the VM and its reference, which its node reports it by. What it is asked for
// after that:
//
//	expected  observed                          asked
//	present   pending or missing, its VM runs   create, which pulls it
//
// An image that a container kept in its VM uses is implied present, and is
// not removed, whether it is kept itself or not, while that container is
// kept. One a container nobody keeps uses is removed only by force.
package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Images is the image kind's control-plane strategy.
type Images struct {
	*blocks.Blocks
}

var (
	_ kind.ControlPlane[imageKind.Spec, imageKind.Status] = &Images{}
	_ kind.Preparer[imageKind.Spec, imageKind.Status]     = &Images{}
	_ blocks.Kind                                         = &Images{}
)

// New is the strategy over what the building blocks share.
func New(dependencies blocks.Dependencies) *Images {
	i := &Images{}
	i.Blocks = blocks.New(imageKind.Descriptor(), i, dependencies)

	return i
}

// Admit takes in an image its owner asked for in one of their Docker VMs,
// under the uuid its VM and its reference give it. One that VM keeps already
// is that one, and not another.
func (s *Images) Admit(ctx context.Context, asked imageKind.Image) (imageKind.Image, domain.ValidationErrors, error) {
	reference := strings.TrimSpace(asked.Spec.Reference)
	if len(reference) == 0 || strings.ContainsAny(reference, " \t\n") {
		return imageKind.Image{}, domain.ValidationErrors{"reference": "invalid_image"}, nil
	}

	parent, _ := asked.Metadata.Owner(imageKind.Parent)

	v, refused, err := s.Into(ctx, asked.Metadata.OwnerUUID, parent.UUID)
	if err != nil || len(refused) > 0 {
		return imageKind.Image{}, refused, err
	}

	uuid := imageKind.UUIDOf(v.Metadata.UUID, reference)

	if _, err := s.Resources.GetOne(ctx, imageKind.Name, uuid); err == nil {
		return imageKind.Image{}, domain.ValidationErrors{"reference": "request_already_exists"}, nil
	} else if !errors.Is(err, domain.ErrNotExists) {
		return imageKind.Image{}, nil, err
	}

	admitted := imageKind.Image{
		Kind: imageKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      imageKind.Normalized(reference),
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    asked.Metadata.Labels,
			Owners:    []kind.Reference{{Kind: imageKind.Parent, UUID: v.Metadata.UUID}},
			Node:      v.Metadata.Node,
		},
		Spec:   imageKind.Spec{Reference: reference},
		Status: imageKind.Status{Status: kind.Status{State: imageKind.Pending, Expected: imageKind.Present}},
	}

	return admitted, nil, nil
}

// Reconcile pulls an image its VM does not hold, once its VM runs.
func (s *Images) Reconcile(ctx context.Context, i imageKind.Image) ([]kind.Intent, error) {
	return s.Present(ctx, i.Status.State, i.Status.Expected, imageKind.VMOf(i))
}

// Apply refuses every action, since an image has none that runs in the
// control plane.
func (s *Images) Apply(_ context.Context, _ imageKind.Image, action string, _ any) (imageKind.Image, domain.ValidationErrors, error) {
	return imageKind.Image{}, nil, fmt.Errorf("%w: an image has no %q run in the control plane", kind.ErrUnknownAction, action)
}

// Prepare readies an image's command: one a container kept in its VM uses is
// refused its delete, as Refuse says.
func (s *Images) Prepare(ctx context.Context, i imageKind.Image, action string, payload any) (imageKind.Image, domain.ValidationErrors, error) {
	raw, err := kind.Encode(i)
	if err != nil {
		return imageKind.Image{}, nil, err
	}

	if err := s.Refuse(ctx, raw, action, payload); err != nil {
		return imageKind.Image{}, nil, err
	}

	return i, nil, nil
}

// Refuse is why an image, kept or not, is not removed: a container kept in
// its VM uses it, which it is implied present for. Whether any other
// container uses it is its dockerd's to say, which refuses its delete unless
// it is forced: what its node last reported of it may be from before a
// container using it was removed a moment ago.
func (s *Images) Refuse(ctx context.Context, r kind.Raw, action string, payload any) error {
	if action != imageKind.ActionDelete {
		return nil
	}

	var spec imageKind.Spec
	if len(r.Spec) > 0 {
		_ = json.Unmarshal(r.Spec, &spec)
	}

	seen := blocks.ObservedOf(r.Status).Docker

	references := []string{spec.Reference, seen.Reference}

	parent, _ := r.Metadata.Owner(imageKind.Parent)

	containers, _, err := s.Resources.GetAll(ctx, containerKind.Name, resource.Filter{Parent: kind.Reference{Kind: containerKind.Parent, UUID: parent.UUID}}, 0, 0)
	if err != nil {
		return err
	}

	for _, c := range containers {
		var used containerKind.Spec
		if err := json.Unmarshal(c.Spec, &used); err != nil || len(strings.TrimSpace(used.Image)) == 0 {
			continue
		}

		for _, reference := range references {
			if len(reference) > 0 && imageKind.Normalized(used.Image) == imageKind.Normalized(reference) {
				return blocks.Refused("the image %s is used by the container %s, which is kept: it is not removed while the container is", imageKind.Normalized(reference), cmpOr(c.Metadata.Name, c.Metadata.UUID))
			}
		}
	}

	return nil
}

// Adopt takes in nothing: nothing can label an image, so nothing found in a
// restored VM is known to have been the platform's.
func (s *Images) Adopt(kind.Observation) (blocks.Adoption, bool) {
	return blocks.Adoption{}, false
}

// Named reports whether name is a reference to an image: the one it was
// pulled as, or the one it was seen under, however either is written.
func (s *Images) Named(r kind.Raw, name string) bool {
	var spec imageKind.Spec
	if len(r.Spec) > 0 {
		_ = json.Unmarshal(r.Spec, &spec)
	}

	asked := imageKind.Normalized(name)

	for _, reference := range []string{spec.Reference, blocks.ObservedOf(r.Status).Docker.Reference} {
		if len(reference) > 0 && imageKind.Normalized(reference) == asked {
			return true
		}
	}

	return false
}

func cmpOr(values ...string) string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}

	return ""
}
