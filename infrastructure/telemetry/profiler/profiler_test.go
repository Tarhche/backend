package profiler

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofileotlp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// capturedHeapProfile produces a real pprof payload as the collectors would.
func capturedHeapProfile(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, pprof.Lookup("heap").WriteTo(&buf, 0))

	return buf.Bytes()
}

func TestProfilerEndToEnd(t *testing.T) {
	var requests atomic.Int64
	var lastBody atomic.Pointer[[]byte]

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		lastBody.Store(&raw)
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		Endpoint:      server.URL + "/v1/profiles",
		Insecure:      true,
		BatchSize:     1,
		FlushInterval: 10 * time.Millisecond,
		// keep the collectors quiet during the test; process is driven directly
		CPUInterval:    time.Hour,
		CPUDuration:    time.Second,
		MemoryInterval: time.Hour,
	}

	p, err := New(cfg, testResource(t), sdkmetric.NewMeterProvider(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	require.NoError(t, p.Start(t.Context()))

	p.process(t.Context(), ProfileTypeHeap, capturedHeapProfile(t))

	require.Eventually(t, func() bool {
		return requests.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond, "profile should be exported")

	require.NoError(t, p.Stop(t.Context()))

	t.Run("payload is valid gzipped OTLP", func(t *testing.T) {
		zr, err := gzip.NewReader(bytes.NewReader(*lastBody.Load()))
		require.NoError(t, err)
		payload, err := io.ReadAll(zr)
		require.NoError(t, err)

		decoded := pprofileotlp.NewExportRequest()
		require.NoError(t, decoded.UnmarshalProto(payload))

		// a heap profile carries four sample types
		assert.Equal(t, 4, decoded.Profiles().ProfileCount())

		rp := decoded.Profiles().ResourceProfiles()
		require.Equal(t, 1, rp.Len())
		serviceName, ok := rp.At(0).Resource().Attributes().Get("service.name")
		require.True(t, ok)
		assert.Equal(t, "blog", serviceName.Str())
	})

	t.Run("delivery is verified in metrics", func(t *testing.T) {
		assert.Equal(t, int64(4), p.inst.sentTotal.Load())
		assert.Equal(t, int64(4), p.inst.deliveredTotal.Load())
		assert.Zero(t, p.inst.failedTotal.Load())
		assert.Zero(t, p.guard.bufferedBytes())
	})
}

func TestProfilerProcessDropsOversizedPayloads(t *testing.T) {
	cfg := Config{
		Endpoint:       "http://localhost:1/v1/profiles",
		Insecure:       true,
		MaxBufferBytes: 8, // smaller than any real profile
		CPUInterval:    time.Hour,
		MemoryInterval: time.Hour,
	}

	p, err := New(cfg, testResource(t), sdkmetric.NewMeterProvider(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	p.process(t.Context(), ProfileTypeHeap, capturedHeapProfile(t))

	assert.Zero(t, p.exporter.queueLength())
	assert.Zero(t, p.guard.bufferedBytes())
}

func TestProfilerProcessDropsGarbage(t *testing.T) {
	cfg := Config{
		Endpoint:       "http://localhost:1/v1/profiles",
		Insecure:       true,
		CPUInterval:    time.Hour,
		MemoryInterval: time.Hour,
	}

	p, err := New(cfg, testResource(t), sdkmetric.NewMeterProvider(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	p.process(t.Context(), ProfileTypeHeap, []byte("not a profile"))

	assert.Zero(t, p.exporter.queueLength())
	assert.Zero(t, p.guard.bufferedBytes(), "reservation must be released on parse failure")
}
