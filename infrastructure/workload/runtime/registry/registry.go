// Package registry builds the drivers an orchestrator runs tasks with: one for
// every class it is configured to offer (WORKLOAD_ORCHESTRATOR_RUNTIMES), each
// by the factory registered for its kind.
//
// It is the one place that knows which kinds of driver exist. A new kind is a
// factory handed to New, and nothing that asks for a class has to change.
package registry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// Registry is the drivers of every class an orchestrator offers, in the order
// they were configured.
type Registry struct {
	drivers []driver.Driver
	byClass map[runtime.Class]driver.Driver
}

var _ driver.Set = &Registry{}

// New builds a driver for every spec, for the node they run tasks for.
//
// What is configured wrongly stops the orchestrator from starting, where
// somebody is watching, rather than leaving it offering something it cannot
// run: a kind nobody registered a factory for, a class configured twice, a
// spec a factory refuses — an endpoint it can never use, an option it does
// not know. A driver whose endpoint does not answer yet is built all the same,
// and offered unhealthy until it does: an orchestrator has always waited for a
// docker daemon that came up after it, rather than going down for it. That one
// is written down here, so it does not go unnoticed either.
//
// Nothing is left behind by a failure: whatever was already built is closed.
func New(ctx context.Context, node string, specs []driver.Spec, factories map[driver.Kind]driver.Factory, logger *slog.Logger) (*Registry, error) {
	if len(specs) == 0 {
		return nil, errors.New("no runtime class is configured: an orchestrator that offers none can run nothing")
	}

	r := &Registry{
		drivers: make([]driver.Driver, 0, len(specs)),
		byClass: make(map[runtime.Class]driver.Driver, len(specs)),
	}

	for _, spec := range specs {
		built, err := r.build(ctx, node, spec, factories)
		if err != nil {
			return nil, errors.Join(err, r.Close())
		}

		r.drivers = append(r.drivers, built)
		r.byClass[spec.Class] = built
	}

	for _, d := range r.drivers {
		if offer := d.Offer(ctx); !offer.Healthy {
			logger.WarnContext(ctx, "a runtime class cannot run tasks yet; it is offered unhealthy until it can",
				"class", d.Class(), "kind", d.Kind(), "reason", offer.Reason)
		}
	}

	return r, nil
}

// build is the driver of one spec, which has to be a class not built already,
// of a kind there is a factory for.
func (r *Registry) build(ctx context.Context, node string, spec driver.Spec, factories map[driver.Kind]driver.Factory) (driver.Driver, error) {
	if _, taken := r.byClass[spec.Class]; taken {
		return nil, fmt.Errorf("the %q runtime class is configured more than once", spec.Class)
	}

	factory, found := factories[spec.Kind]
	if !found {
		return nil, fmt.Errorf("the %q runtime class is of the %q kind, which this orchestrator has no driver for: it has %s",
			spec.Class, spec.Kind, kinds(factories))
	}

	built, err := factory(ctx, node, spec)
	if err != nil {
		return nil, fmt.Errorf("the %q runtime class cannot be set up: %w", spec.Class, err)
	}

	// a factory that builds something other than what it was asked for would
	// send one class's tasks to another's driver.
	if built.Class() != spec.Class || built.Kind() != spec.Kind {
		return nil, errors.Join(
			fmt.Errorf("the %q runtime class was built as %q of the %q kind", spec.Class, built.Class(), built.Kind()),
			built.Close(),
		)
	}

	return built, nil
}

// kinds is the kinds there are factories for, as they are written.
func kinds(factories map[driver.Kind]driver.Factory) string {
	names := make([]string, 0, len(factories))
	for _, kind := range slices.Sorted(maps.Keys(factories)) {
		names = append(names, kind.String())
	}

	if len(names) == 0 {
		return "none"
	}

	return strings.Join(names, ", ")
}

// For is the driver of a class. A class that names none is sysbox, which is
// every class from before there were classes.
func (r *Registry) For(class runtime.Class) (driver.Driver, error) {
	found, ok := r.byClass[class.OrSysbox()]
	if !ok {
		return nil, fmt.Errorf("%w: %q", driver.ErrUnknownClass, class.OrSysbox())
	}

	return found, nil
}

// All is every driver, in the order they were configured.
func (r *Registry) All() []driver.Driver {
	return slices.Clone(r.drivers)
}

// Close lets go of every driver. What they run carries on.
func (r *Registry) Close() error {
	errs := make([]error, 0, len(r.drivers))
	for _, d := range r.drivers {
		errs = append(errs, d.Close())
	}

	return errors.Join(errs...)
}
