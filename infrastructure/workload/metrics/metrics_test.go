package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// collect records with a recorder on a reader of its own and reads back what
// would have been exported, by instrument name.
func collect(t *testing.T, record func(r *Recorder)) map[string]metricdata.Metrics {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	recorder, err := New(provider)
	require.NoError(t, err)

	record(recorder)

	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &collected))

	byName := make(map[string]metricdata.Metrics)
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			byName[m.Name] = m
		}
	}

	return byName
}

func gaugePoints(t *testing.T, m metricdata.Metrics) []metricdata.DataPoint[int64] {
	t.Helper()

	gauge, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "%s is a gauge of whole numbers", m.Name)

	return gauge.DataPoints
}

func value(t *testing.T, points []metricdata.DataPoint[int64], attributes ...attribute.KeyValue) int64 {
	t.Helper()

	want := attribute.NewSet(attributes...)
	for _, point := range points {
		if point.Attributes.Equals(&want) {
			return point.Value
		}
	}

	t.Fatalf("no point for %v", attributes)

	return 0
}

func TestRecorder_Node(t *testing.T) {
	t.Parallel()

	info := vm.Info{
		CPUs:      8,
		Memory:    16 << 30,
		Disk:      200 << 30,
		Allocated: vm.Resources{CPUs: 3, Memory: 4 << 30, Disk: 40 << 30},
	}

	collected := collect(t, func(r *Recorder) {
		r.Node(t.Context(), "workload-orchestrator-01", info, map[vm.State]int{vm.Running: 2, vm.Stopped: 1})
	})

	node := attribute.String("node", "workload-orchestrator-01")

	t.Run("every state is counted, zeros included", func(t *testing.T) {
		t.Parallel()

		count := collected["workload.vm.count"]
		assert.Equal(t, "{vm}", count.Unit)

		points := gaugePoints(t, count)
		assert.Len(t, points, 10)

		for _, state := range []string{"created", "scheduled", "starting", "running", "stopping", "stopped", "restarting", "restoring", "failed", "deleting"} {
			want := int64(0)
			switch state {
			case "running":
				want = 2
			case "stopped":
				want = 1
			}

			assert.Equal(t, want, value(t, points, node, attribute.String("state", state)), state)
		}
	})

	t.Run("capacity and allocation are per node, in vCPUs and bytes", func(t *testing.T) {
		t.Parallel()

		for name, want := range map[string]struct {
			unit  string
			value int64
		}{
			"workload.vm.capacity.cpus":    {unit: "{cpu}", value: 8},
			"workload.vm.capacity.memory":  {unit: "By", value: 16 << 30},
			"workload.vm.capacity.disk":    {unit: "By", value: 200 << 30},
			"workload.vm.allocated.cpus":   {unit: "{cpu}", value: 3},
			"workload.vm.allocated.memory": {unit: "By", value: 4 << 30},
			"workload.vm.allocated.disk":   {unit: "By", value: 40 << 30},
		} {
			m, ok := collected[name]
			require.True(t, ok, name)
			assert.Equal(t, want.unit, m.Unit, name)
			assert.Equal(t, want.value, value(t, gaugePoints(t, m), node), name)
		}
	})
}

func TestRecorder_VMHost(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		up   bool
		want int64
	}{
		{up: true, want: 1},
		{up: false, want: 0},
	} {
		collected := collect(t, func(r *Recorder) {
			r.VMHost(t.Context(), "workload-orchestrator-01", tt.up)
		})

		up := collected["workload.vmhost.up"]
		assert.Empty(t, up.Unit, "a unit would rename it in Prometheus")
		assert.Equal(t, tt.want, value(t, gaugePoints(t, up), attribute.String("node", "workload-orchestrator-01")))
	}
}

func TestRecorder_Histograms(t *testing.T) {
	t.Parallel()

	collected := collect(t, func(r *Recorder) {
		r.Snapshot(t.Context(), 90*time.Second, 3<<30)
		r.DockerRequest(t.Context(), "docker.images.pull", 42*time.Second)
	})

	t.Run("a snapshot's duration is in seconds, in buckets made for minutes", func(t *testing.T) {
		t.Parallel()

		m := collected["workload.snapshot.duration"]
		assert.Equal(t, "s", m.Unit)

		histogram, ok := m.Data.(metricdata.Histogram[float64])
		require.True(t, ok)
		require.Len(t, histogram.DataPoints, 1)

		assert.Equal(t, snapshotDurationBuckets, histogram.DataPoints[0].Bounds)
		assert.Equal(t, 90.0, histogram.DataPoints[0].Sum)
	})

	t.Run("a snapshot's size is in bytes, in buckets made for disks", func(t *testing.T) {
		t.Parallel()

		m := collected["workload.snapshot.size"]
		assert.Equal(t, "By", m.Unit)

		histogram, ok := m.Data.(metricdata.Histogram[int64])
		require.True(t, ok)
		require.Len(t, histogram.DataPoints, 1)

		assert.Equal(t, snapshotSizeBuckets, histogram.DataPoints[0].Bounds)
		assert.Equal(t, int64(3<<30), histogram.DataPoints[0].Sum)
	})

	t.Run("a docker request is timed by its operation", func(t *testing.T) {
		t.Parallel()

		m := collected["workload.docker.request.duration"]
		assert.Equal(t, "s", m.Unit)

		histogram, ok := m.Data.(metricdata.Histogram[float64])
		require.True(t, ok)
		require.Len(t, histogram.DataPoints, 1)

		point := histogram.DataPoints[0]
		assert.Equal(t, dockerRequestBuckets, point.Bounds)
		assert.Equal(t, 42.0, point.Sum)

		op, ok := point.Attributes.Value("op")
		require.True(t, ok)
		assert.Equal(t, "docker.images.pull", op.AsString())
	})
}
