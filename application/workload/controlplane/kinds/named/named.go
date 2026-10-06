// Package named finds what a request to the control plane's resource API
// names: a record by its uuid, or, for a kind that names its resources by
// more than their uuids inside the parent they live in (kind.Resolver), by
// whatever else names one in the parent the request names, as a container is
// named by its Docker id or its name in its Docker VM.
//
// A request that names a parent is held to it: what lives anywhere else is
// not there, a record or an extra alike. And an extra is held to whose it is,
// as a record is: somebody asking for their own sees none of anybody else's.
package named

import (
	"context"
	"errors"
	"slices"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Resource is the record name names, as ownerUUID's own, or as anybody's
// when ownerUUID is empty, inside parent when one is given; and the uuid it
// was looked for under, which is what the kind's extras are asked for when
// none of its records is it. Nothing there is domain.ErrNotExists.
func Resource(ctx context.Context, resources owner.Resources, binding kind.ControlPlaneBinding, ownerUUID string, parent string, name string) (resource.Record, string, error) {
	d := binding.Descriptor()

	uuid, err := resolve(ctx, binding, parent, name)
	if err != nil {
		return resource.Record{}, uuid, err
	}

	r, err := owner.Resource(ctx, resources, d.Name, ownerUUID, uuid)
	if err != nil {
		return resource.Record{}, uuid, err
	}

	if !Lives(r.Metadata, d, parent) {
		return resource.Record{}, uuid, domain.ErrNotExists
	}

	return r, uuid, nil
}

// Extra is the kind's extra uuid names, and its extras: one that is
// ownerUUID's, or anybody's when ownerUUID is empty, inside parent when one is
// given. A kind with no extras, and an extra that is not there to whoever
// asks, is notThere, which is what looking for a record came to.
func Extra(ctx context.Context, binding kind.ControlPlaneBinding, ownerUUID string, parent string, uuid string, notThere error) (kind.Extras, kind.Raw, error) {
	extras, extended := binding.Extras()
	if !extended {
		return nil, kind.Raw{}, notThere
	}

	r, err := extras.One(ctx, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, kind.Raw{}, notThere
	} else if err != nil {
		return nil, kind.Raw{}, err
	}

	if len(ownerUUID) > 0 && r.Metadata.OwnerUUID != ownerUUID {
		return nil, kind.Raw{}, notThere
	}

	if !Lives(r.Metadata, binding.Descriptor(), parent) {
		return nil, kind.Raw{}, notThere
	}

	return extras, r, nil
}

// Lives reports whether what m describes lives in parent, a resource of d's
// parent kind, as one of its owners. A request that names no parent is about
// whatever it names, wherever it lives.
func Lives(m kind.Metadata, d kind.Descriptor, parent string) bool {
	if len(parent) == 0 {
		return true
	}

	return slices.ContainsFunc(m.Owners, func(o kind.Reference) bool {
		return o.UUID == parent && (len(d.Parent) == 0 || o.Kind == d.Parent)
	})
}

// resolve is the uuid name stands for inside parent: what the kind resolves
// it to, when the request names a parent and the kind resolves names, and
// name itself otherwise, a uuid naming itself.
func resolve(ctx context.Context, binding kind.ControlPlaneBinding, parent string, name string) (string, error) {
	d := binding.Descriptor()

	resolver, resolves := binding.Resolver()
	if !resolves || len(parent) == 0 || len(d.Parent) == 0 {
		return name, nil
	}

	uuid, err := resolver.Resolve(ctx, kind.Reference{Kind: d.Parent, UUID: parent}, name)
	if err != nil {
		return name, err
	}

	return uuid, nil
}
