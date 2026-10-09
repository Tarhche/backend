package domain

import "context"

// Migrations says how far what is stored has been brought to the shape this
// version reads: which of the migrations it knows of are recorded as applied.
// A service that must not read what is stored in another shape waits for
// none to be pending.
type Migrations interface {
	// Pending is the names of the migrations not recorded as applied yet, in
	// the order they are applied: none once everything stored is in the
	// shape this version reads.
	Pending(ctx context.Context) ([]string, error)
}
