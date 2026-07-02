package profiler

import (
	"testing"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

// testPprofProfile builds a small but complete pprof profile: two sample
// types (like a CPU profile), two samples sharing a function, and one sample
// carrying trace correlation labels.
func testPprofProfile() *profile.Profile {
	mapping := &profile.Mapping{ID: 1, Start: 0x1000, Limit: 0x2000, File: "/opt/app/server"}

	fnMain := &profile.Function{ID: 1, Name: "main.main", SystemName: "main.main", Filename: "main.go", StartLine: 10}
	fnWork := &profile.Function{ID: 2, Name: "main.work", SystemName: "main.work", Filename: "main.go", StartLine: 42}

	locMain := &profile.Location{ID: 1, Mapping: mapping, Address: 0x1100, Line: []profile.Line{{Function: fnMain, Line: 12}}}
	locWork := &profile.Location{ID: 2, Mapping: mapping, Address: 0x1200, Line: []profile.Line{{Function: fnWork, Line: 45}}}

	return &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "samples", Unit: "count"},
			{Type: "cpu", Unit: "nanoseconds"},
		},
		PeriodType:    &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Period:        10000000,
		TimeNanos:     1700000000000000000,
		DurationNanos: 10000000000,
		Mapping:       []*profile.Mapping{mapping},
		Function:      []*profile.Function{fnMain, fnWork},
		Location:      []*profile.Location{locMain, locWork},
		Sample: []*profile.Sample{
			{
				Location: []*profile.Location{locWork, locMain}, // leaf first
				Value:    []int64{3, 30000000},
				Label: map[string][]string{
					"trace_id": {"4bf92f3577b34da6a3ce929d0e0e4736"},
					"span_id":  {"00f067aa0ba902b7"},
				},
			},
			{
				Location: []*profile.Location{locMain},
				Value:    []int64{1, 10000000},
				NumLabel: map[string][]int64{"bytes": {512}},
				NumUnit:  map[string][]string{"bytes": {"bytes"}},
			},
		},
	}
}

func testResource(t *testing.T) *resource.Resource {
	t.Helper()

	res, err := resource.New(t.Context(), resource.WithAttributes(
		attribute.String("service.name", "blog"),
	))
	require.NoError(t, err)

	return res
}

// stringAt resolves a string-table index.
func stringAt(t *testing.T, dict pprofile.ProfilesDictionary, idx int32) string {
	t.Helper()
	require.Less(t, int(idx), dict.StringTable().Len())

	return dict.StringTable().At(int(idx))
}

func TestToOTLP(t *testing.T) {
	out, err := toOTLP(testPprofProfile(), ProfileTypeCPU, testResource(t))
	require.NoError(t, err)

	require.Equal(t, 1, out.ResourceProfiles().Len())
	rp := out.ResourceProfiles().At(0)

	t.Run("carries the service resource", func(t *testing.T) {
		serviceName, ok := rp.Resource().Attributes().Get("service.name")
		require.True(t, ok)
		assert.Equal(t, "blog", serviceName.Str())
	})

	require.Equal(t, 1, rp.ScopeProfiles().Len())
	sp := rp.ScopeProfiles().At(0)
	assert.Equal(t, scopeName, sp.Scope().Name())

	dict := out.Dictionary()

	t.Run("emits one profile record per pprof sample type", func(t *testing.T) {
		require.Equal(t, 2, sp.Profiles().Len())

		first := sp.Profiles().At(0)
		assert.Equal(t, "samples", stringAt(t, dict, first.SampleType().TypeStrindex()))
		assert.Equal(t, "count", stringAt(t, dict, first.SampleType().UnitStrindex()))

		second := sp.Profiles().At(1)
		assert.Equal(t, "cpu", stringAt(t, dict, second.SampleType().TypeStrindex()))
		assert.Equal(t, "nanoseconds", stringAt(t, dict, second.SampleType().UnitStrindex()))

		assert.Equal(t, int64(10000000), second.Period())
		assert.Equal(t, uint64(10000000000), second.DurationNano())
		assert.Equal(t, uint64(1700000000000000000), uint64(second.Time()))
	})

	t.Run("splits values per sample type", func(t *testing.T) {
		samples := sp.Profiles().At(0).Samples()
		require.Equal(t, 2, samples.Len())
		assert.Equal(t, int64(3), samples.At(0).Values().At(0))
		assert.Equal(t, int64(1), samples.At(1).Values().At(0))

		cpuSamples := sp.Profiles().At(1).Samples()
		assert.Equal(t, int64(30000000), cpuSamples.At(0).Values().At(0))
		assert.Equal(t, int64(10000000), cpuSamples.At(1).Values().At(0))
	})

	t.Run("resolves stacks leaf first through the dictionary", func(t *testing.T) {
		sample := sp.Profiles().At(0).Samples().At(0)

		stack := dict.StackTable().At(int(sample.StackIndex()))
		require.Equal(t, 2, stack.LocationIndices().Len())

		leaf := dict.LocationTable().At(int(stack.LocationIndices().At(0)))
		require.Equal(t, 1, leaf.Lines().Len())
		leafFn := dict.FunctionTable().At(int(leaf.Lines().At(0).FunctionIndex()))
		assert.Equal(t, "main.work", stringAt(t, dict, leafFn.NameStrindex()))

		root := dict.LocationTable().At(int(stack.LocationIndices().At(1)))
		rootFn := dict.FunctionTable().At(int(root.Lines().At(0).FunctionIndex()))
		assert.Equal(t, "main.main", stringAt(t, dict, rootFn.NameStrindex()))

		mapping := dict.MappingTable().At(int(leaf.MappingIndex()))
		assert.Equal(t, "/opt/app/server", stringAt(t, dict, mapping.FilenameStrindex()))
	})

	t.Run("turns trace labels into links", func(t *testing.T) {
		linked := sp.Profiles().At(0).Samples().At(0)
		require.NotZero(t, linked.LinkIndex())

		link := dict.LinkTable().At(int(linked.LinkIndex()))
		assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", link.TraceID().String())
		assert.Equal(t, "00f067aa0ba902b7", link.SpanID().String())

		unlinked := sp.Profiles().At(0).Samples().At(1)
		assert.Zero(t, unlinked.LinkIndex())
	})

	t.Run("keeps numeric labels as attributes with units", func(t *testing.T) {
		sample := sp.Profiles().At(0).Samples().At(1)
		require.Equal(t, 1, sample.AttributeIndices().Len())

		kv := dict.AttributeTable().At(int(sample.AttributeIndices().At(0)))
		assert.Equal(t, "bytes", stringAt(t, dict, kv.KeyStrindex()))
		assert.Equal(t, int64(512), kv.Value().Int())
		assert.Equal(t, "bytes", stringAt(t, dict, kv.UnitStrindex()))
	})

	t.Run("tags profiles with the profile type", func(t *testing.T) {
		p := sp.Profiles().At(0)
		require.Equal(t, 1, p.AttributeIndices().Len())

		kv := dict.AttributeTable().At(int(p.AttributeIndices().At(0)))
		assert.Equal(t, profileTypeKey, stringAt(t, dict, kv.KeyStrindex()))
		assert.Equal(t, string(ProfileTypeCPU), kv.Value().Str())
	})

	t.Run("does not attach the original payload", func(t *testing.T) {
		p := sp.Profiles().At(0)
		assert.Empty(t, p.OriginalPayloadFormat())
		assert.Zero(t, p.OriginalPayload().Len())
	})

	t.Run("names the period type after ambiguous profile types", func(t *testing.T) {
		// block and mutex share one pprof schema; the period type name is the
		// only place a backend can tell them apart
		contention := &profile.Profile{
			SampleType: []*profile.ValueType{
				{Type: "contentions", Unit: "count"},
				{Type: "delay", Unit: "nanoseconds"},
			},
			PeriodType: &profile.ValueType{Type: "contentions", Unit: "count"},
			Period:     1,
		}

		for _, pt := range []ProfileType{ProfileTypeBlock, ProfileTypeMutex} {
			converted, err := toOTLP(contention, pt, testResource(t))
			require.NoError(t, err)

			p := converted.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
			assert.Equal(t, string(pt), stringAt(t, converted.Dictionary(), p.PeriodType().TypeStrindex()))
			assert.Equal(t, "count", stringAt(t, converted.Dictionary(), p.PeriodType().UnitStrindex()))
			// sample types keep the original pprof schema
			assert.Equal(t, "contentions", stringAt(t, converted.Dictionary(), p.SampleType().TypeStrindex()))
		}

		// unambiguous profiles keep their pprof period type untouched
		cpu, err := toOTLP(testPprofProfile(), ProfileTypeCPU, testResource(t))
		require.NoError(t, err)
		p := cpu.ResourceProfiles().At(0).ScopeProfiles().At(0).Profiles().At(0)
		assert.Equal(t, "cpu", stringAt(t, cpu.Dictionary(), p.PeriodType().TypeStrindex()))
	})

	t.Run("seeds every dictionary table with a zero entry", func(t *testing.T) {
		assert.Equal(t, "", dict.StringTable().At(0))
		assert.Positive(t, dict.MappingTable().Len())
		assert.Positive(t, dict.LocationTable().Len())
		assert.Positive(t, dict.FunctionTable().Len())
		assert.Positive(t, dict.StackTable().Len())
		assert.Positive(t, dict.LinkTable().Len())
		assert.Positive(t, dict.AttributeTable().Len())
	})
}
