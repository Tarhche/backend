// Package multiplex puts every class a node offers behind the one
// task.Runtime, network.Manager and node.Manager the orchestrator's use cases
// have always asked, so that none of them has to know there is more than one.
//
// A run's ID says which class holds it (runtime.Qualify): a sysbox run keeps
// the bare ID docker gave it, and any other class's carries the class in front.
// So a command about a run goes to the driver holding it without anything
// having to remember which one that is, and what was running before there were
// classes is found where it always was.
package multiplex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// ErrClassRequired is something only one class's driver can do, asked of every
// class at once. Making a run's image or network ready is the class's own
// business, so whatever does it asks the class's driver (driver.Set) instead;
// asked here, it is a mistake to catch rather than a guess to make.
var ErrClassRequired = errors.New("this is asked of one runtime class's driver, not of every class at once")

// Multiplexer is every class a node offers, as one runtime.
//
// What asks about every run — what a node holds, a task's runs, a slug's — is
// asked of every class at once, and what each answers is put together. A class
// whose driver fails is left out rather than failing the rest, so a vmhost that
// is away for a moment does not take the node's containers out of its
// heartbeats; it is the class that is reported unhealthy (Offer), and the
// control plane holds what it was running as unknown rather than lost. Only
// when every class fails does the question fail.
//
// What asks about one run goes to the class its ID names.
//
// What the node's runs use between them is Node, and the node's drivers as the
// use cases see them are Drivers.
type Multiplexer struct {
	drivers driver.Set
	logger  *slog.Logger

	// outages are the classes that could not answer the last time they were
	// asked about their runs, by what they were asked, and why.
	mu      sync.Mutex
	outages map[runtime.Class]outage
}

// question is what a class is asked about its runs.
type question int

const (
	// holding is what runs a class holds: a node's, a task's, a slug's.
	holding question = iota

	// using is what a class's runs use between them.
	using
)

// failing is what a question that went unanswered says about its class.
func (q question) failing() string {
	if q == using {
		return "what its runs use cannot be read"
	}

	return "its runs cannot be listed"
}

// outage is what a class could not answer the last time it was asked, and
// why. Each question is answered on its own, so one answered is not taken
// for the other.
type outage map[question]error

var (
	_ task.Runtime    = &Multiplexer{}
	_ task.Dialer     = &Multiplexer{}
	_ network.Manager = &Multiplexer{}
)

// New puts the drivers of a node behind one runtime.
func New(drivers driver.Set, logger *slog.Logger) *Multiplexer {
	return &Multiplexer{
		drivers: drivers,
		logger:  logger,
		outages: make(map[runtime.Class]outage),
	}
}

// OnNode is every run the node holds, of every class.
func (m *Multiplexer) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	return m.gather(ctx, func(ctx context.Context, tasks task.Runtime) ([]task.Execution, error) {
		return tasks.OnNode(ctx, nodeName)
	})
}

// Of is a task's runs, of whichever class holds them.
//
// A class that cannot be asked is left out, so a command about a task it holds
// finds nothing here and is taken as done. It is not lost for good: the
// control plane goes on seeing the task as it was, and asks again until what it
// asked for is true.
func (m *Multiplexer) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	return m.gather(ctx, func(ctx context.Context, tasks task.Runtime) ([]task.Execution, error) {
		return tasks.Of(ctx, taskUUID)
	})
}

// BySlug is the runs answering to a slug, of whichever class holds them.
func (m *Multiplexer) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	return m.gather(ctx, func(ctx context.Context, tasks task.Runtime) ([]task.Execution, error) {
		return tasks.BySlug(ctx, slug)
	})
}

// gather asks every class's runs at once, and puts what they answer together,
// in the order the classes were configured, each run named by its class.
func (m *Multiplexer) gather(ctx context.Context, ask func(context.Context, task.Runtime) ([]task.Execution, error)) ([]task.Execution, error) {
	drivers := m.drivers.All()

	answers := make([][]task.Execution, len(drivers))
	errs := make([]error, len(drivers))

	var wg sync.WaitGroup
	for i, d := range drivers {
		wg.Go(func() {
			answers[i], errs[i] = ask(ctx, d.Tasks())
		})
	}
	wg.Wait()

	runs := make([]task.Execution, 0)
	failed := make([]error, 0, len(drivers))

	for i, d := range drivers {
		class := d.Class()

		m.answered(ctx, class, holding, errs[i])

		if errs[i] != nil {
			failed = append(failed, fmt.Errorf("the %q runtime class: %w", class, errs[i]))

			continue
		}

		for _, run := range answers[i] {
			runs = append(runs, qualified(class, run))
		}
	}

	if len(drivers) > 0 && len(failed) == len(drivers) {
		return nil, errors.Join(failed...)
	}

	return runs, nil
}

// qualified is a run as the multiplexer hands it out: named by its class, and
// saying which class it is.
func qualified(class runtime.Class, run task.Execution) task.Execution {
	run.ID = runtime.Qualify(class, run.ID)
	run.Runtime = class

	return run
}

// route is the driver holding a run, and what that driver calls it.
func (m *Multiplexer) route(executionID string) (driver.Driver, string, error) {
	class, native := runtime.Split(executionID)

	d, err := m.drivers.For(class)
	if err != nil {
		return nil, "", fmt.Errorf("the run %q: %w", executionID, err)
	}

	return d, native, nil
}

// EnsureImage is the class's own business: whatever runs a task makes its
// image ready on the driver of the task's class.
func (m *Multiplexer) EnsureImage(ctx context.Context, image string) error {
	return fmt.Errorf("making %q ready: %w", image, ErrClassRequired)
}

// Create makes a run on the driver of the class the execution names, which a
// task that names none is sysbox.
func (m *Multiplexer) Create(ctx context.Context, execution *task.Execution) (string, error) {
	d, err := m.drivers.For(execution.Runtime.OrSysbox())
	if err != nil {
		return "", err
	}

	id, err := d.Tasks().Create(ctx, execution)
	if err != nil {
		return "", err
	}

	return runtime.Qualify(d.Class(), id), nil
}

func (m *Multiplexer) Start(ctx context.Context, executionID string) error {
	d, native, err := m.route(executionID)
	if err != nil {
		return err
	}

	return d.Tasks().Start(ctx, native)
}

func (m *Multiplexer) Stop(ctx context.Context, executionID string) error {
	d, native, err := m.route(executionID)
	if err != nil {
		return err
	}

	return d.Tasks().Stop(ctx, native)
}

func (m *Multiplexer) Restart(ctx context.Context, executionID string) error {
	d, native, err := m.route(executionID)
	if err != nil {
		return err
	}

	return d.Tasks().Restart(ctx, native)
}

func (m *Multiplexer) Kill(ctx context.Context, executionID string) error {
	d, native, err := m.route(executionID)
	if err != nil {
		return err
	}

	return d.Tasks().Kill(ctx, native)
}

func (m *Multiplexer) Delete(ctx context.Context, executionID string) error {
	d, native, err := m.route(executionID)
	if err != nil {
		return err
	}

	return d.Tasks().Delete(ctx, native)
}

func (m *Multiplexer) Inspect(ctx context.Context, executionID string) (task.Execution, error) {
	d, native, err := m.route(executionID)
	if err != nil {
		return task.Execution{}, err
	}

	run, err := d.Tasks().Inspect(ctx, native)
	if err != nil {
		return task.Execution{}, err
	}

	return qualified(d.Class(), run), nil
}

func (m *Multiplexer) Stats(ctx context.Context, executionID string) (task.Stats, error) {
	d, native, err := m.route(executionID)
	if err != nil {
		return task.Stats{}, err
	}

	return d.Tasks().Stats(ctx, native)
}

func (m *Multiplexer) Logs(ctx context.Context, executionID string, writer io.Writer) error {
	d, native, err := m.route(executionID)
	if err != nil {
		return err
	}

	return d.Tasks().Logs(ctx, native, writer)
}

func (m *Multiplexer) StreamLogs(ctx context.Context, executionID string, since time.Time, emit func(task.LogLine) error) error {
	d, native, err := m.route(executionID)
	if err != nil {
		return err
	}

	return d.Tasks().StreamLogs(ctx, native, since, emit)
}

func (m *Multiplexer) Exec(ctx context.Context, executionID string, options task.ExecOptions) (task.ExecSession, error) {
	d, native, err := m.route(executionID)
	if err != nil {
		return nil, err
	}

	return d.Tasks().Exec(ctx, native, options)
}

// DialContext connects to a run's port through the class holding it, when
// that class can dial its runs. One that cannot says so with
// task.ErrNotSupported, and its runs are reached where they were published.
func (m *Multiplexer) DialContext(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	d, native, err := m.route(executionID)
	if err != nil {
		return nil, err
	}

	dialer, ok := d.Tasks().(task.Dialer)
	if !ok {
		return nil, task.ErrNotSupported
	}

	return dialer.DialContext(ctx, native, p)
}

// EnsureIsolatedNetwork is the class's own business: a task joins the network
// of its class's driver.
func (m *Multiplexer) EnsureIsolatedNetwork(ctx context.Context) error {
	return fmt.Errorf("making the isolated network: %w", ErrClassRequired)
}

// EnsureStackNetwork is the class's own business: a stack's network belongs
// to the driver of the stack's class.
func (m *Multiplexer) EnsureStackNetwork(ctx context.Context, stackSlug string) error {
	return fmt.Errorf("making the network of the %q stack: %w", stackSlug, ErrClassRequired)
}

// RemoveStackNetwork drops a stack's network whichever class made it, which is
// for a stack whose class is not known: one deleted by a control plane from
// before there were classes. Every class is asked at once, and one that has no
// such network has nothing to drop, which is the outcome asked for.
func (m *Multiplexer) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	drivers := m.drivers.All()
	errs := make([]error, len(drivers))

	var wg sync.WaitGroup
	for i, d := range drivers {
		wg.Go(func() {
			err := d.Networks().RemoveStackNetwork(ctx, stackSlug)
			if err != nil && !errors.Is(err, domain.ErrNotExists) {
				errs[i] = fmt.Errorf("the %q runtime class: %w", d.Class(), err)
			}
		})
	}
	wg.Wait()

	return errors.Join(errs...)
}

// Node is what the node's runs use between them, every class's added up.
func (m *Multiplexer) Node() node.Manager {
	return &nodeStats{multiplexer: m}
}

// nodeStats is the multiplexer as a node.Manager, which cannot be the
// multiplexer itself: a run's stats and a node's are both called Stats.
type nodeStats struct {
	multiplexer *Multiplexer
}

var _ node.Manager = &nodeStats{}

// Stats is what the node's runs use, every class's added up. A class that
// cannot say is left out, as it is from the node's runs, and is offered
// unhealthy until it can; only a node none of whose classes can say has
// nothing to report.
func (n *nodeStats) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	drivers := n.multiplexer.drivers.All()

	stats := make([]node.Stats, len(drivers))
	errs := make([]error, len(drivers))

	var wg sync.WaitGroup
	for i, d := range drivers {
		wg.Go(func() {
			stats[i], errs[i] = d.Node().Stats(ctx, nodeName)
		})
	}
	wg.Wait()

	var (
		total  node.Stats
		failed []error
	)

	for i, d := range drivers {
		n.multiplexer.answered(ctx, d.Class(), using, errs[i])

		if errs[i] != nil {
			failed = append(failed, fmt.Errorf("the %q runtime class: %w", d.Class(), errs[i]))

			continue
		}

		total.PIDs += stats[i].PIDs
		total.CPUPercent += stats[i].CPUPercent
		total.MemoryUsage += stats[i].MemoryUsage
		total.MemoryLimit += stats[i].MemoryLimit
		total.NetworkInput += stats[i].NetworkInput
		total.NetworkOutput += stats[i].NetworkOutput
		total.BlockInput += stats[i].BlockInput
		total.BlockOutput += stats[i].BlockOutput
	}

	if len(drivers) > 0 && len(failed) == len(drivers) {
		return node.Stats{}, errors.Join(failed...)
	}

	if total.MemoryLimit > 0 {
		total.MemoryPercent = float64(total.MemoryUsage) / float64(total.MemoryLimit) * 100.0
	}

	return total, nil
}

// Drivers is the node's drivers as the use cases see them: the same drivers,
// whose offers also say when they could not answer for their runs here.
func (m *Multiplexer) Drivers() driver.Set {
	return &watchedSet{multiplexer: m}
}

// watchedSet is the node's drivers, each watched.
type watchedSet struct {
	multiplexer *Multiplexer
}

var _ driver.Set = &watchedSet{}

func (s *watchedSet) For(class runtime.Class) (driver.Driver, error) {
	d, err := s.multiplexer.drivers.For(class)
	if err != nil {
		return nil, err
	}

	return &watched{Driver: d, multiplexer: s.multiplexer}, nil
}

func (s *watchedSet) All() []driver.Driver {
	drivers := s.multiplexer.drivers.All()

	result := make([]driver.Driver, len(drivers))
	for i, d := range drivers {
		result[i] = &watched{Driver: d, multiplexer: s.multiplexer}
	}

	return result
}

// watched is a driver whose offer also says when it could not answer for its
// runs: what it holds, or what they use.
//
// A driver answers for what stands behind it from what it last heard, and may
// not have heard yet that it is gone; the runs that went missing from a
// heartbeat are what is certain. A class that cannot say what it holds is not
// one to place anything on, and what it was holding is unknown rather than
// lost, which is what an unhealthy offer tells the control plane.
type watched struct {
	driver.Driver

	multiplexer *Multiplexer
}

func (w *watched) Offer(ctx context.Context) runtime.Offer {
	offer := w.Driver.Offer(ctx)

	if reason, failing := w.multiplexer.outage(w.Class()); failing && offer.Healthy {
		offer.Healthy = false
		offer.Reason = reason
	}

	return offer
}

// outage is why a class could not answer for its runs the last time it was
// asked, if it could not.
func (m *Multiplexer) outage(class runtime.Class) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, q := range []question{holding, using} {
		if err, failing := m.outages[class][q]; failing {
			return fmt.Sprintf("%s: %v", q.failing(), err), true
		}
	}

	return "", false
}

// answered writes down how a class answered a question about its runs, and
// says so when that changes rather than every time a heartbeat finds it the
// same.
func (m *Multiplexer) answered(ctx context.Context, class runtime.Class, q question, err error) {
	m.mu.Lock()
	failing := m.outages[class]
	_, was := failing[q]

	switch {
	case err != nil:
		if failing == nil {
			failing = make(outage)
			m.outages[class] = failing
		}

		failing[q] = err
	case was:
		delete(failing, q)

		if len(failing) == 0 {
			delete(m.outages, class)
		}
	}
	m.mu.Unlock()

	switch {
	case err != nil && !was:
		m.logger.WarnContext(ctx, "a runtime class cannot answer for its runs; it is left out, and offered unhealthy, until it can",
			"class", class, "question", q.failing(), "error", err)
	case err == nil && was:
		m.logger.InfoContext(ctx, "a runtime class answers for its runs again", "class", class)
	}
}
