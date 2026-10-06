package kindstest

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Witnessing is the fan's control-plane strategy that hears every report of
// fans (kind.Witness), keeping what it heard for a test to read; finds a fan
// inside its house by its name (kind.Resolver), as a container is found in
// its Docker VM by its Docker name; and readies its commands as Refuse says
// (kind.Preparer). The fans on its Shelf, when it has one, are shown beside
// its records, and found by their names too.
type Witnessing struct {
	*Fans

	// Records are where the fans it finds by their names are read from.
	Records resource.Repository

	// Shelf, when there is one, holds the fans shown beside its records.
	Shelf *Shelf

	// Refuse, when set, is what readying a command for a node fails with,
	// by its action: nothing lets it through.
	Refuse func(action string) error

	lock  sync.Mutex
	heard []Heard
}

// Heard is one report a Witnessing was told of.
type Heard struct {
	Node   string
	Report kind.Report[json.RawMessage]
	At     time.Time
}

var (
	_ kind.Witness                    = &Witnessing{}
	_ kind.Resolver                   = &Witnessing{}
	_ kind.Extender                   = &Witnessing{}
	_ kind.Preparer[Spec, Status]     = &Witnessing{}
	_ kind.ControlPlane[Spec, Status] = &Witnessing{}
)

func (w *Witnessing) Witnessed(_ context.Context, nodeName string, report kind.Report[json.RawMessage], at time.Time) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.heard = append(w.heard, Heard{Node: nodeName, Report: report, At: at})

	return nil
}

// Heard is every report it was told of, in turn.
func (w *Witnessing) Heard() []Heard {
	w.lock.Lock()
	defer w.lock.Unlock()

	return slices.Clone(w.heard)
}

func (w *Witnessing) Extras() kind.Extras {
	if w.Shelf == nil {
		return nil
	}

	return w.Shelf
}

// Resolve is the fan in house parent name names: by its uuid, or by its
// name, a record before one on the shelf.
func (w *Witnessing) Resolve(ctx context.Context, parent kind.Reference, name string) (string, error) {
	records, _, err := w.Records.GetAll(ctx, Kind, resource.Filter{Parent: parent}, 0, 0)
	if err != nil {
		return "", err
	}

	for _, r := range records {
		if r.Metadata.UUID == name || r.Metadata.Name == name {
			return r.Metadata.UUID, nil
		}
	}

	if w.Shelf == nil {
		return "", domain.ErrNotExists
	}

	shelved, err := w.Shelf.All(ctx)
	if err != nil {
		return "", err
	}

	for _, r := range shelved {
		if (r.Metadata.UUID == name || r.Metadata.Name == name) && slices.Contains(r.Metadata.Owners, parent) {
			return r.Metadata.UUID, nil
		}
	}

	return "", domain.ErrNotExists
}

// Prepare lets a command through as it is, unless Refuse refuses it.
func (w *Witnessing) Prepare(_ context.Context, r Fan, action string, _ any) (Fan, domain.ValidationErrors, error) {
	if w.Refuse != nil {
		if err := w.Refuse(action); err != nil {
			return Fan{}, nil, err
		}
	}

	return r, nil, nil
}
