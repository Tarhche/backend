package vmhost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
)

// The names, units, attributes and buckets vmhost measures by are fixed by the
// infrastructure repository's monitoring/README.md: its dashboard and its
// alerts query them. This holds vmhost to them.
func TestMetrics(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	w := newWorld(t, nil)

	engine, err := New(testConfig(w.dataDir), Parts{
		Hypervisor: w.hypervisor,
		Images:     w.images,
		Fabric:     w.fabric,
		Guests:     w.guests,
		States:     w.states,
		Logs:       w.logs,
		FreeSpace:  func() (uint64, error) { return 100 << 30, nil },
		Metrics:    provider.Meter("github.com/khanzadimahdi/testproject/application/workload/vmhost"),
	}, discard())
	require.NoError(t, err)

	engine.timing = fastTiming()
	w.engine = engine

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = engine.Close(ctx)
	})

	w.hypervisor.Leave(vm.Machine{ID: "ffffffffffffffff", Running: true, Unit: "workload-vm-ffffffffffffffff.service", VsockPath: "fake://orphan"}, vmMock.NewFakeAgent(nil).Running(1))
	require.NoError(t, engine.Reconcile(w.ctx))

	_, err = engine.PrepareImage(w.ctx, "nginx:alpine")
	require.NoError(t, err)

	id := w.run(spec("web"))
	require.NoError(t, engine.Stop(w.ctx, id, time.Second))

	huge := spec("huge")
	huge.Resources.Memory = 8 << 30

	_, err = engine.Create(w.ctx, huge)
	require.ErrorIs(t, err, vm.ErrCapacity)

	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(w.ctx, &collected))

	found := make(map[string]metricdata.Metrics)
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			found[m.Name] = m
		}
	}

	units := map[string]string{
		"workload.vmhost.vms":                "{vm}",
		"workload.vmhost.boot.duration":      "s",
		"workload.vmhost.prepare.duration":   "s",
		"workload.vmhost.stop.duration":      "s",
		"workload.vmhost.memory.reserved":    "By",
		"workload.vmhost.memory.budget":      "By",
		"workload.vmhost.admission.refusals": "{refusal}",
		"workload.vmhost.agent.errors":       "{error}",
		"workload.vmhost.orphans":            "{orphan}",
		"workload.vmhost.image.cache.size":   "By",
		"workload.vmhost.image.cache.limit":  "By",
	}

	for name, unit := range units {
		m, ok := found[name]
		if assert.True(t, ok, "%s is measured", name) {
			assert.Equal(t, unit, m.Unit, name)
		}
	}

	t.Run("every state is reported, the ones no vm is in included", func(t *testing.T) {
		gauge, ok := found["workload.vmhost.vms"].Data.(metricdata.Gauge[int64])
		require.True(t, ok)

		states := make(map[string]int64)
		for _, point := range gauge.DataPoints {
			state, _ := point.Attributes.Value(stateKey)
			states[state.AsString()] = point.Value
		}

		assert.Equal(t, map[string]int64{"created": 0, "running": 0, "restarting": 0, "exited": 1, "dead": 0, "removing": 0}, states)
	})

	t.Run("counters are there from the start, for every resource and kind", func(t *testing.T) {
		refusals, ok := found["workload.vmhost.admission.refusals"].Data.(metricdata.Sum[int64])
		require.True(t, ok)

		counted := make(map[string]int64)
		for _, point := range refusals.DataPoints {
			resource, _ := point.Attributes.Value(resourceKey)
			counted[resource.AsString()] = point.Value
		}

		assert.Equal(t, map[string]int64{"memory": 1, "cpu": 0, "disk": 0, "uids": 0}, counted)

		orphans, ok := found["workload.vmhost.orphans"].Data.(metricdata.Sum[int64])
		require.True(t, ok)

		kinds := make(map[string]int64)
		for _, point := range orphans.DataPoints {
			kind, _ := point.Attributes.Value(kindKey)
			kinds[kind.AsString()] = point.Value
		}

		assert.Equal(t, map[string]int64{"unit": 1, "directory": 0}, kinds)
	})

	t.Run("durations are in seconds, in the buckets they are read in, by result", func(t *testing.T) {
		buckets := map[string][]float64{
			"workload.vmhost.boot.duration":    {0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
			"workload.vmhost.prepare.duration": {1, 5, 10, 30, 60, 120, 300, 600},
			"workload.vmhost.stop.duration":    {0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
		}

		for name, bounds := range buckets {
			histogram, ok := found[name].Data.(metricdata.Histogram[float64])
			require.True(t, ok, name)
			require.NotEmpty(t, histogram.DataPoints, name)

			for _, point := range histogram.DataPoints {
				assert.Equal(t, bounds, point.Bounds, name)

				result, _ := point.Attributes.Value(resultKey)
				assert.Equal(t, "ok", result.AsString(), name)
			}
		}
	})

	t.Run("memory is the budget, and what the vms hold of it", func(t *testing.T) {
		budget, ok := found["workload.vmhost.memory.budget"].Data.(metricdata.Gauge[int64])
		require.True(t, ok)
		assert.Equal(t, int64(4<<30), budget.DataPoints[0].Value)

		reserved, ok := found["workload.vmhost.memory.reserved"].Data.(metricdata.Gauge[int64])
		require.True(t, ok)
		assert.Zero(t, reserved.DataPoints[0].Value, "a stopped vm holds no memory")
	})
}
