// Package metrics is what a node says about its VMs to the monitoring stack,
// as OpenTelemetry metrics.
//
// The names, units and attributes are the ones the "Workload VMs" dashboard
// and its alerts read (the infrastructure repository's monitoring README), so
// they are a contract: Alloy turns a dot into an underscore and a unit into a
// suffix, and a metric named or measured differently is one nobody sees. Two
// things follow from that. workload.vmhost.up has no unit, because a gauge in
// "1" becomes workload_vmhost_up_ratio. And the histograms say their buckets,
// because the SDK's own, made for milliseconds, would put every Docker request
// in the first bucket and every snapshot in the last.
package metrics

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// scopeName identifies these instruments on what is exported.
const scopeName = "github.com/khanzadimahdi/testproject/infrastructure/workload/metrics"

// The attributes, as the dashboard reads them.
const (
	nodeKey  = "node"
	stateKey = "state"
	opKey    = "op"
)

const (
	mebibyte = 1 << 20
	gibibyte = 1 << 30
)

var (
	// dockerRequestBuckets are seconds: from a listing answered at once to a
	// pull that takes the ten minutes it is allowed.
	dockerRequestBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}

	// snapshotDurationBuckets are seconds, up to an hour.
	snapshotDurationBuckets = []float64{1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600}

	// snapshotSizeBuckets are bytes, from 16 MiB to the 64 GiB of a large
	// disk.
	snapshotSizeBuckets = []float64{
		16 * mebibyte, 64 * mebibyte, 256 * mebibyte, 512 * mebibyte,
		1 * gibibyte, 2 * gibibyte, 4 * gibibyte, 8 * gibibyte, 16 * gibibyte, 32 * gibibyte, 64 * gibibyte,
	}
)

// Recorder records a node's VM metrics.
type Recorder struct {
	vmCount metric.Int64Gauge

	capacityCPUs   metric.Int64Gauge
	capacityMemory metric.Int64Gauge
	capacityDisk   metric.Int64Gauge

	allocatedCPUs   metric.Int64Gauge
	allocatedMemory metric.Int64Gauge
	allocatedDisk   metric.Int64Gauge

	vmhostUp metric.Int64Gauge

	snapshotDuration metric.Float64Histogram
	snapshotSize     metric.Int64Histogram

	dockerRequestDuration metric.Float64Histogram
}

// New makes the instruments on provider, which is the global meter provider in
// a running service.
func New(provider metric.MeterProvider) (*Recorder, error) {
	meter := provider.Meter(scopeName)

	r := &Recorder{}

	var err, errs error

	r.vmCount, err = meter.Int64Gauge("workload.vm.count",
		metric.WithUnit("{vm}"),
		metric.WithDescription("VMs on a node, by state; every state is reported, zeros included."))
	errs = errors.Join(errs, err)

	r.capacityCPUs, err = meter.Int64Gauge("workload.vm.capacity.cpus",
		metric.WithUnit("{cpu}"),
		metric.WithDescription("vCPUs a node offers to VMs."))
	errs = errors.Join(errs, err)

	r.capacityMemory, err = meter.Int64Gauge("workload.vm.capacity.memory",
		metric.WithUnit("By"),
		metric.WithDescription("Memory a node offers to VMs."))
	errs = errors.Join(errs, err)

	r.capacityDisk, err = meter.Int64Gauge("workload.vm.capacity.disk",
		metric.WithUnit("By"),
		metric.WithDescription("Disk a node offers to VMs."))
	errs = errors.Join(errs, err)

	r.allocatedCPUs, err = meter.Int64Gauge("workload.vm.allocated.cpus",
		metric.WithUnit("{cpu}"),
		metric.WithDescription("vCPUs the VMs on a node have been given."))
	errs = errors.Join(errs, err)

	r.allocatedMemory, err = meter.Int64Gauge("workload.vm.allocated.memory",
		metric.WithUnit("By"),
		metric.WithDescription("Memory the VMs on a node have been given."))
	errs = errors.Join(errs, err)

	r.allocatedDisk, err = meter.Int64Gauge("workload.vm.allocated.disk",
		metric.WithUnit("By"),
		metric.WithDescription("Disk the VMs on a node have been given."))
	errs = errors.Join(errs, err)

	// no unit: one in "1" would be exported as workload_vmhost_up_ratio, which
	// is not what the alert reads.
	r.vmhostUp, err = meter.Int64Gauge("workload.vmhost.up",
		metric.WithDescription("1 when the orchestrator's last call to its vmhost succeeded, 0 when it failed."))
	errs = errors.Join(errs, err)

	r.snapshotDuration, err = meter.Float64Histogram("workload.snapshot.duration",
		metric.WithUnit("s"),
		metric.WithDescription("How long taking a snapshot and storing it took."),
		metric.WithExplicitBucketBoundaries(snapshotDurationBuckets...))
	errs = errors.Join(errs, err)

	r.snapshotSize, err = meter.Int64Histogram("workload.snapshot.size",
		metric.WithUnit("By"),
		metric.WithDescription("How large a stored snapshot is."),
		metric.WithExplicitBucketBoundaries(snapshotSizeBuckets...))
	errs = errors.Join(errs, err)

	r.dockerRequestDuration, err = meter.Float64Histogram("workload.docker.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("How long a node took to answer a request to a Docker VM's dockerd, by operation."),
		metric.WithExplicitBucketBoundaries(dockerRequestBuckets...))
	errs = errors.Join(errs, err)

	if errs != nil {
		return nil, errs
	}

	return r, nil
}

// Node records what a node offers to VMs, what it has given them, and how
// many of them are in each state.
//
// Every state is recorded, at zero when no VM is in it, so that a state that
// empties shows nothing rather than the last count it had.
func (r *Recorder) Node(ctx context.Context, node string, info vm.Info, counts map[vm.State]int) {
	onNode := metric.WithAttributes(attribute.String(nodeKey, node))

	for state := vm.Created; state <= vm.Deleting; state++ {
		r.vmCount.Record(ctx, int64(counts[state]), metric.WithAttributes(
			attribute.String(nodeKey, node),
			attribute.String(stateKey, state.String()),
		))
	}

	r.capacityCPUs.Record(ctx, int64(info.CPUs), onNode)
	r.capacityMemory.Record(ctx, bytes(info.Memory), onNode)
	r.capacityDisk.Record(ctx, bytes(info.Disk), onNode)

	r.allocatedCPUs.Record(ctx, int64(info.Allocated.CPUs), onNode)
	r.allocatedMemory.Record(ctx, bytes(info.Allocated.Memory), onNode)
	r.allocatedDisk.Record(ctx, bytes(info.Allocated.Disk), onNode)
}

// VMHost records whether the orchestrator's last call to its vmhost
// succeeded.
func (r *Recorder) VMHost(ctx context.Context, node string, up bool) {
	var value int64
	if up {
		value = 1
	}

	r.vmhostUp.Record(ctx, value, metric.WithAttributes(attribute.String(nodeKey, node)))
}

// Snapshot records a snapshot that was taken and stored: how long that took,
// and how large it is.
func (r *Recorder) Snapshot(ctx context.Context, took time.Duration, size int64) {
	r.snapshotDuration.Record(ctx, took.Seconds())
	r.snapshotSize.Record(ctx, size)
}

// DockerRequest records how long answering one request to a Docker VM's
// dockerd took.
func (r *Recorder) DockerRequest(ctx context.Context, op string, took time.Duration) {
	r.dockerRequestDuration.Record(ctx, took.Seconds(), metric.WithAttributes(attribute.String(opKey, op)))
}

// bytes is a size as a gauge takes it. No node has 8 EiB, so a size past what
// an int64 holds is a budget that means "all of it" and is reported as the
// most there can be rather than as a negative number.
func bytes(size uint64) int64 {
	return int64(min(size, 1<<63-1))
}
