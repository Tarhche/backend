package vmhost

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// What vmhost measures, for the dashboard and the alerts that watch it (the
// infrastructure repository's monitoring/README.md, "Workload vmhost", fixes
// these names, units, attributes and buckets, and its dashboard and alert
// queries depend on them): how many VMs are in each state, how much of the
// memory budget they hold, how long boots, image preparations and stops take
// and how many boots fail, how often a VM is refused for want of room, how
// often an agent fails to answer, how many machines no VM accounted for were
// found, and how much disk images take. How often the firewall had to be put
// back (workload.vmhost.firewall.repairs) is the fabric's to count, since only
// it can tell a rule it put back from one that was there. Exported to Prometheus through Alloy they read workload_vmhost_vms,
// workload_vmhost_boot_duration_seconds_bucket and so on.

// the attributes vmhost's measurements carry, and their values.
const (
	stateKey     = "state"
	resultKey    = "result"
	resourceKey  = "resource"
	operationKey = "operation"
	kindKey      = "kind"

	resultOK    = "ok"
	resultError = "error"

	orphanUnit      = "unit"
	orphanDirectory = "directory"
)

// the resources admission refuses VMs for, and the operations asked of agents,
// which are counted from zero from the start so that the first of each is
// seen as an increase.
var (
	refusalResources = []string{"memory", "cpu", "disk", "uids"}
	orphanKinds      = []string{orphanUnit, orphanDirectory}
	agentOperations  = []string{"ready", "configure", "start", "status", "wait", "logs", "stop", "hosts", "exec", "end_exec", "dial", "stats"}
	vmStates         = []vm.State{vm.StateCreated, vm.StateRunning, vm.StateRestarting, vm.StateExited, vm.StateDead, vm.StateRemoving}
)

// the histograms' buckets, in seconds: a boot takes a second or two, pulling
// an image minutes, and a stop up to its timeout.
var (
	bootBuckets    = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}
	prepareBuckets = []float64{1, 5, 10, 30, 60, 120, 300, 600}
	stopBuckets    = []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60}
)

type instruments struct {
	boots     metric.Float64Histogram
	prepares  metric.Float64Histogram
	stops     metric.Float64Histogram
	refusals  metric.Int64Counter
	agent     metric.Int64Counter
	orphans   metric.Int64Counter
	adoptions metric.Int64Counter
	restarts  metric.Int64Counter

	// imageBytes is what images took when they were last counted.
	imageBytes atomic.Int64

	registration metric.Registration
}

func newInstruments(meter metric.Meter, e *Engine) (*instruments, error) {
	if meter == nil {
		meter = noop.NewMeterProvider().Meter("")
	}

	inst := &instruments{}

	var err, errs error

	inst.boots, err = meter.Float64Histogram("workload.vmhost.boot.duration",
		metric.WithUnit("s"),
		metric.WithDescription("From asking the hypervisor to boot a machine until its agent answers, by result."),
		metric.WithExplicitBucketBoundaries(bootBuckets...),
	)
	errs = errors.Join(errs, err)

	inst.prepares, err = meter.Float64Histogram("workload.vmhost.prepare.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Making an image ready to boot: pulling it and making its disk, by result."),
		metric.WithExplicitBucketBoundaries(prepareBuckets...),
	)
	errs = errors.Join(errs, err)

	inst.stops, err = meter.Float64Histogram("workload.vmhost.stop.duration",
		metric.WithUnit("s"),
		metric.WithDescription("From asking a VM to stop until its machine is gone, by result."),
		metric.WithExplicitBucketBoundaries(stopBuckets...),
	)
	errs = errors.Join(errs, err)

	inst.refusals, err = meter.Int64Counter("workload.vmhost.admission.refusals",
		metric.WithUnit("{refusal}"),
		metric.WithDescription("VMs refused for want of room, by what ran out: memory, cpu, disk or uids."),
	)
	errs = errors.Join(errs, err)

	inst.agent, err = meter.Int64Counter("workload.vmhost.agent.errors",
		metric.WithUnit("{error}"),
		metric.WithDescription("Requests to a VM's agent that failed, by what was asked."),
	)
	errs = errors.Join(errs, err)

	inst.orphans, err = meter.Int64Counter("workload.vmhost.orphans",
		metric.WithUnit("{orphan}"),
		metric.WithDescription("Machines found with no VM record, and ended, by how they were found: as a unit, or as a directory."),
	)
	errs = errors.Join(errs, err)

	inst.adoptions, err = meter.Int64Counter("workload.vmhost.adoptions",
		metric.WithUnit("{vm}"),
		metric.WithDescription("Running VMs an earlier vmhost booted that were taken back."),
	)
	errs = errors.Join(errs, err)

	inst.restarts, err = meter.Int64Counter("workload.vmhost.restarts",
		metric.WithUnit("{restart}"),
		metric.WithDescription("Times a restart policy started a VM's task again, by kind: task, in its machine, or machine, booted again."),
	)
	errs = errors.Join(errs, err)

	vms, err := meter.Int64ObservableGauge("workload.vmhost.vms",
		metric.WithUnit("{vm}"),
		metric.WithDescription("VMs held, by state."),
	)
	errs = errors.Join(errs, err)

	reserved, err := meter.Int64ObservableGauge("workload.vmhost.memory.reserved",
		metric.WithUnit("By"),
		metric.WithDescription("Memory the VMs hold, their VMMs' overhead included, as admission counts it."),
	)
	errs = errors.Join(errs, err)

	budget, err := meter.Int64ObservableGauge("workload.vmhost.memory.budget",
		metric.WithUnit("By"),
		metric.WithDescription("Memory every VM together may be given."),
	)
	errs = errors.Join(errs, err)

	images, err := meter.Int64ObservableGauge("workload.vmhost.image.cache.size",
		metric.WithUnit("By"),
		metric.WithDescription("Disk the images made ready to boot take."),
	)
	errs = errors.Join(errs, err)

	imageLimit, err := meter.Int64ObservableGauge("workload.vmhost.image.cache.limit",
		metric.WithUnit("By"),
		metric.WithDescription("Disk images may take before the least recently used that no VM boots are let go of."),
	)
	errs = errors.Join(errs, err)

	healthy, err := meter.Int64ObservableGauge("workload.vmhost.healthy",
		metric.WithDescription("1 while vmhost can make and boot VMs, 0 while it cannot."),
	)
	errs = errors.Join(errs, err)

	if errs != nil {
		return nil, errs
	}

	inst.registration, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		all, err := e.states.All(ctx)
		if err != nil {
			return err
		}

		counts := make(map[vm.State]int64, len(vmStates))
		for _, v := range all {
			counts[v.State]++
		}

		// every state, the ones no VM is in included.
		for _, state := range vmStates {
			observer.ObserveInt64(vms, counts[state], metric.WithAttributes(attribute.String(stateKey, string(state))))
		}

		observer.ObserveInt64(reserved, bytes(e.reserved(all)))
		observer.ObserveInt64(budget, bytes(e.config.MaxMemory))
		observer.ObserveInt64(images, inst.imageBytes.Load())
		observer.ObserveInt64(imageLimit, bytes(e.config.ImageCacheMax))

		up, _ := e.Health()
		if up {
			observer.ObserveInt64(healthy, 1)
		} else {
			observer.ObserveInt64(healthy, 0)
		}

		return nil
	}, vms, reserved, budget, images, imageLimit, healthy)
	if err != nil {
		return nil, err
	}

	inst.zero()

	return inst, nil
}

// zero counts nothing of every counter series vmhost knows of, so that the
// first of each is seen as an increase rather than as a series appearing.
func (i *instruments) zero() {
	ctx := context.Background()

	for _, resource := range refusalResources {
		i.refusals.Add(ctx, 0, metric.WithAttributes(attribute.String(resourceKey, resource)))
	}

	for _, kind := range orphanKinds {
		i.orphans.Add(ctx, 0, metric.WithAttributes(attribute.String(kindKey, kind)))
	}

	for _, operation := range agentOperations {
		i.agent.Add(ctx, 0, metric.WithAttributes(attribute.String(operationKey, operation)))
	}

	for _, kind := range []string{"task", "machine"} {
		i.restarts.Add(ctx, 0, metric.WithAttributes(attribute.String(kindKey, kind)))
	}

	i.adoptions.Add(ctx, 0)
}

// bytes is a size as a gauge takes it.
func bytes(size uint64) int64 {
	return int64(min(size, 1<<62))
}

func outcome(err error) metric.MeasurementOption {
	if err != nil {
		return metric.WithAttributes(attribute.String(resultKey, resultError))
	}

	return metric.WithAttributes(attribute.String(resultKey, resultOK))
}

func (i *instruments) booted(ctx context.Context, took time.Duration, err error) {
	i.boots.Record(context.WithoutCancel(ctx), took.Seconds(), outcome(err))
}

func (i *instruments) prepared(ctx context.Context, took time.Duration, err error) {
	i.prepares.Record(context.WithoutCancel(ctx), took.Seconds(), outcome(err))
}

func (i *instruments) stopped(ctx context.Context, took time.Duration, err error) {
	i.stops.Record(context.WithoutCancel(ctx), took.Seconds(), outcome(err))
}

func (i *instruments) refused(ctx context.Context, resource string) {
	i.refusals.Add(context.WithoutCancel(ctx), 1, metric.WithAttributes(attribute.String(resourceKey, resource)))
}

func (i *instruments) agentFailed(ctx context.Context, operation string) {
	i.agent.Add(context.WithoutCancel(ctx), 1, metric.WithAttributes(attribute.String(operationKey, operation)))
}

// orphaned counts a machine found with no VM record: as a host unit, or as a
// machine directory where its hypervisor keeps no units.
func (i *instruments) orphaned(ctx context.Context, machine vm.Machine) {
	kind := orphanDirectory
	if len(machine.Unit) > 0 {
		kind = orphanUnit
	}

	i.orphans.Add(context.WithoutCancel(ctx), 1, metric.WithAttributes(attribute.String(kindKey, kind)))
}

func (i *instruments) adopted(ctx context.Context) {
	i.adoptions.Add(context.WithoutCancel(ctx), 1)
}

func (i *instruments) restarted(ctx context.Context, kind string) {
	i.restarts.Add(context.WithoutCancel(ctx), 1, metric.WithAttributes(attribute.String(kindKey, kind)))
}

func (i *instruments) imageCache(size int64) {
	i.imageBytes.Store(size)
}

func (i *instruments) close() error {
	if i.registration == nil {
		return nil
	}

	return i.registration.Unregister()
}
