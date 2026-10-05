// Package owner narrows what the control plane reads to one person's own.
//
// Every route of the control plane's API takes an owner, which the blog passes
// for somebody acting on their own things and leaves out for an administrator.
// Something that is not the owner's is not there as far as they are
// concerned, so it reads exactly as something that is not there at all.
package owner

import "context"

// Repository is what keeps records that belong to somebody.
type Repository[T any] interface {
	GetOne(ctx context.Context, uuid string) (T, error)
	GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (T, error)
}

// One is the record uuid names, as ownerUUID's own, or as anybody's when
// ownerUUID is empty.
func One[T any](ctx context.Context, repository Repository[T], ownerUUID string, uuid string) (T, error) {
	if len(ownerUUID) > 0 {
		return repository.GetOneByOwner(ctx, ownerUUID, uuid)
	}

	return repository.GetOne(ctx, uuid)
}
