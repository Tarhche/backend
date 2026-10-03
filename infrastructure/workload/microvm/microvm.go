// Package microvm is the orchestrator's driver for the microvm kind: a client
// of vmhost on the same host, which runs a task as a microVM of its own.
//
// It holds no privilege and keeps no state. Everything it knows about a run it
// reads back from vmhost, which keeps the run's labels as docker keeps a
// container's; so an orchestrator that is redeployed — which happens with
// every release — finds every VM where it left it, and nothing has to be
// handed over. It acts only on VMs labelled with its own node's name.
//
// A VM's ports are published nowhere: the driver reaches them through vmhost,
// which reaches them through the agent inside the VM, so it is a task.Dialer
// and the orchestrator's proxy dials through it.
package microvm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	workloadRuntime "github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// infoInterval is how often vmhost is asked what it is, which is how often
	// the node says so in its heartbeat.
	infoInterval = time.Second

	// infoTimeout bounds one asking. vmhost answers from memory, so one that
	// takes longer than this is not answering.
	infoTimeout = 2 * time.Second

	// infoStale is how old the last answer may be before the class is offered
	// unhealthy for want of a newer one.
	infoStale = 5 * time.Second
)

// Driver is one class of microVMs on this node.
type Driver struct {
	class  workloadRuntime.Class
	node   string
	client *Client
	logger *slog.Logger

	tasks    *Runtime
	networks *Networks
	nodes    *Node

	// what vmhost last said about itself, when, and why it said nothing the
	// last time it was asked, if it did not.
	lock     sync.Mutex
	info     vm.Info
	answered time.Time
	err      error

	stop    context.CancelFunc
	stopped chan struct{}
	closing sync.Once
}

var _ driver.Driver = (*Driver)(nil)

// New builds the driver of one class, reaching vmhost at the spec's endpoint
// (vm.DefaultEndpoint, as a rule, and when the spec names none).
//
// What the spec says is checked now, and a spec that is wrong stops the
// orchestrator from starting: configuration does not get better by waiting. A
// vmhost that does not answer does not: the class is offered unhealthy until
// it does, and every other class the node offers runs as it did. An
// orchestrator that refused to start without vmhost would stop reporting the
// sysbox tasks it holds, and the control plane would take them for lost.
func New(ctx context.Context, nodeName string, spec driver.Spec, logger *slog.Logger) (*Driver, error) {
	if spec.Kind != driver.KindMicroVM {
		return nil, fmt.Errorf("the %q runtime class is of the %s kind, not %s", spec.Class, spec.Kind, driver.KindMicroVM)
	}

	if !spec.Class.IsValid() {
		return nil, fmt.Errorf("%q cannot name a runtime class", spec.Class)
	}

	// a VM is found again by the node it is labelled with, so a driver that
	// labelled them with nothing would find them all, whoever's they were.
	if len(nodeName) == 0 {
		return nil, fmt.Errorf("the %q runtime class: a microvm driver is given the name of the node it runs for", spec.Class)
	}

	if err := checkOptions(spec); err != nil {
		return nil, err
	}

	endpoint := spec.Endpoint
	if len(endpoint) == 0 {
		endpoint = vm.DefaultEndpoint
	}

	client, err := NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("the %q runtime class: %w", spec.Class, err)
	}

	logger = logger.With("class", spec.Class.String())
	tracer := otel.Tracer("microvm")

	d := &Driver{
		class:  spec.Class,
		node:   nodeName,
		client: client,
		logger: logger,
		tasks:  &Runtime{client: client, node: nodeName, class: spec.Class, logger: logger, tracer: tracer},
		networks: &Networks{
			client:         client,
			logger:         logger,
			tracer:         tracer,
			detachTimeout:  detachTimeout,
			detachInterval: detachInterval,
		},
		nodes: &Node{client: client, node: nodeName, tracer: tracer},
	}

	// the first look is taken now, so the first heartbeat says how vmhost is
	// rather than that nothing is known about it yet.
	d.refresh(ctx)

	watching, stop := context.WithCancel(context.WithoutCancel(ctx))
	d.stop = stop
	d.stopped = make(chan struct{})

	go d.watch(watching, infoInterval)

	return d, nil
}

// NewFactory is how the registry builds drivers of the microvm kind.
func NewFactory(logger *slog.Logger) driver.Factory {
	return func(ctx context.Context, nodeName string, spec driver.Spec) (driver.Driver, error) {
		built, err := New(ctx, nodeName, spec, logger)
		if err != nil {
			return nil, err
		}

		return built, nil
	}
}

// checkOptions refuses options a microvm driver does not read. One that was
// written and then ignored is a class that runs other than its configuration
// says, with nothing to say so.
//
// node-memory is one of them for now. It is meant for orchestrators sharing a
// vmhost, each held to a share of its memory, and holding one to its share is
// vmhost's to do, which nothing in what the driver may say to vmhost lets it be
// asked to. Until it can, one vmhost serves one orchestrator, which may use all
// of it.
func checkOptions(spec driver.Spec) error {
	for name := range spec.Options {
		if name == driver.OptionNodeMemory {
			return fmt.Errorf("the %q runtime class: %s is not supported yet: one vmhost serves one orchestrator", spec.Class, name)
		}

		return fmt.Errorf("the %q runtime class: a microvm driver has no %q option", spec.Class, name)
	}

	return nil
}

func (d *Driver) Class() workloadRuntime.Class {
	return d.class
}

func (d *Driver) Kind() driver.Kind {
	return driver.KindMicroVM
}

func (d *Driver) Tasks() task.Runtime {
	return d.tasks
}

func (d *Driver) Networks() network.Manager {
	return d.networks
}

func (d *Driver) Node() node.Manager {
	return d.nodes
}

// Offer is the class as vmhost says it runs it, or unhealthy, with the reason,
// when vmhost cannot be reached.
//
// It is asked for on every node heartbeat, once a second, so it is answered
// from what vmhost last said rather than by asking it: a vmhost that hangs
// must not hold up the heartbeat that carries every other class's offer, and
// every task the node holds. What vmhost last said is kept fresh in the
// background (watch).
//
// An unhealthy offer still carries what the class could last do, and how much
// room it had: placement passes it by either way, and what it could do is
// still worth showing to whoever looks at the node.
func (d *Driver) Offer(ctx context.Context) workloadRuntime.Offer {
	d.lock.Lock()
	defer d.lock.Unlock()

	offer := workloadRuntime.Offer{
		Class:        d.class,
		Driver:       driver.KindMicroVM.String(),
		Version:      d.info.Version,
		Capabilities: d.info.Capabilities,
		Capacity:     d.info.Capacity,
	}

	switch {
	case d.err != nil:
		offer.Reason = d.err.Error()
	case d.answered.IsZero():
		offer.Reason = "vmhost has not answered yet"
	case time.Since(d.answered) > infoStale:
		offer.Reason = fmt.Sprintf("vmhost has not answered since %s", d.answered.UTC().Format(time.RFC3339))
	default:
		offer.Healthy = d.info.Healthy
		offer.Reason = d.info.Reason
	}

	return offer
}

// watch keeps what vmhost says about itself fresh, until the driver is
// closed.
func (d *Driver) watch(ctx context.Context, every time.Duration) {
	defer close(d.stopped)

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.refresh(ctx)
		}
	}
}

// refresh asks vmhost what it is, and keeps the answer, or why there was
// none. A change in either is written down once, when it happens, rather than
// on every asking.
func (d *Driver) refresh(ctx context.Context) {
	asking, cancel := context.WithTimeout(ctx, infoTimeout)
	defer cancel()

	info, err := d.client.Info(asking)

	// the driver closing is not vmhost failing.
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if err != nil {
		if d.err == nil {
			d.logger.Warn("vmhost cannot be reached: the class is offered unhealthy until it can", "error", err)
		}

		d.err = err

		return
	}

	switch {
	case d.err != nil || d.answered.IsZero():
		d.logger.Info("vmhost answers", "version", info.Version, "healthy", info.Healthy, "reason", info.Reason)
	case info.Healthy != d.info.Healthy:
		d.logger.Info("vmhost changed its health", "healthy", info.Healthy, "reason", info.Reason)
	}

	d.info, d.answered, d.err = info, time.Now(), nil
}

// Close lets go of what the driver holds. What it runs carries on: a VM is
// vmhost's, and the next orchestrator to run finds it where it was.
func (d *Driver) Close() error {
	d.closing.Do(func() {
		d.stop()
		<-d.stopped

		d.client.Close()
	})

	return nil
}
