// Package slugs gives a VM, a task or a stack a slug nothing else holds.
//
// A slug is what a VM's or a task's ports are served under and a stack's
// compose project, so it is unique, and unique among them all, whatever their
// kinds: the resources of every kind are kept together, under one unique
// index of slugs. It is made from the name and a random suffix, and made
// again in the rare case that one is taken. The unique index is what holds
// two asked for at the same moment apart; this is what keeps that from being
// the common case.
package slugs

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/slug"
)

// attempts is how many slugs are tried before giving up. A five letter suffix
// is nearly twelve million of them for one name, so needing more than one is
// already rare.
const attempts = 8

// ErrExhausted is every slug tried being taken already.
var ErrExhausted = errors.New("no free slug could be found for the name")

// Taken reports whether a slug is held already.
type Taken func(ctx context.Context, slug string) (bool, error)

// Generate is a slug for name that none of taken holds.
func Generate(ctx context.Context, name string, taken ...Taken) (string, error) {
	for range attempts {
		candidate, err := slug.Generate(name)
		if err != nil {
			return "", err
		}

		free, err := isFree(ctx, candidate, taken)
		if err != nil {
			return "", err
		}

		if free {
			return candidate, nil
		}
	}

	return "", ErrExhausted
}

func isFree(ctx context.Context, candidate string, taken []Taken) (bool, error) {
	for _, held := range taken {
		isHeld, err := held(ctx, candidate)
		if err != nil {
			return false, err
		}

		if isHeld {
			return false, nil
		}
	}

	return true, nil
}

// By is a slug held by whatever lookup finds by it.
func By[T any](lookup func(ctx context.Context, slug string) (T, error)) Taken {
	return func(ctx context.Context, slug string) (bool, error) {
		_, err := lookup(ctx, slug)

		switch {
		case errors.Is(err, domain.ErrNotExists):
			return false, nil
		case err != nil:
			return false, err
		default:
			return true, nil
		}
	}
}
