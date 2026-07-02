package profiler

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofileotlp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func testExporter(t *testing.T, endpoint string) *exporter {
	t.Helper()

	guard := newResourceGuard(1000, 1<<30)
	inst, err := newInstruments(sdkmetric.NewMeterProvider(), func() int64 { return 0 }, guard.bufferedBytes)
	require.NoError(t, err)

	cfg := Config{Endpoint: endpoint, Insecure: true}
	require.NoError(t, cfg.normalize())

	e := newExporter(cfg, guard, inst, slog.New(slog.DiscardHandler))
	e.backoff = func(int) time.Duration { return time.Millisecond }

	return e
}

// testEnvelope converts the shared test profile into an export envelope.
func testEnvelope(t *testing.T) envelope {
	t.Helper()

	profiles, err := toOTLP(testPprofProfile(), ProfileTypeCPU, testResource(t))
	require.NoError(t, err)

	return envelope{profiles: profiles, records: int64(profiles.ProfileCount()), rawBytes: 10}
}

func TestExporterFlush(t *testing.T) {
	t.Run("posts gzipped protobuf the backend can decode", func(t *testing.T) {
		var received atomic.Pointer[http.Request]
		var body atomic.Pointer[[]byte]

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received.Store(r)
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			body.Store(&raw)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		e := testExporter(t, server.URL+"/v1/profiles")
		require.NoError(t, e.guard.reserve(10))

		e.flush([]envelope{testEnvelope(t)})

		req := received.Load()
		require.NotNil(t, req)
		assert.Equal(t, "/v1/profiles", req.URL.Path)
		assert.Equal(t, "application/x-protobuf", req.Header.Get("Content-Type"))
		assert.Equal(t, "gzip", req.Header.Get("Content-Encoding"))

		zr, err := gzip.NewReader(bytes.NewReader(*body.Load()))
		require.NoError(t, err)
		payload, err := io.ReadAll(zr)
		require.NoError(t, err)

		decoded := pprofileotlp.NewExportRequest()
		require.NoError(t, decoded.UnmarshalProto(payload))
		assert.Equal(t, 2, decoded.Profiles().ProfileCount())

		assert.Equal(t, int64(2), e.inst.sentTotal.Load())
		assert.Equal(t, int64(2), e.inst.deliveredTotal.Load())
		assert.Zero(t, e.guard.bufferedBytes(), "reservation must be released after export")
	})

	t.Run("sends configured headers", func(t *testing.T) {
		var got atomic.Value

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got.Store(r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		e := testExporter(t, server.URL)
		e.headers = map[string]string{"Authorization": "Bearer test-key"}
		require.NoError(t, e.guard.reserve(10))

		e.flush([]envelope{testEnvelope(t)})

		assert.Equal(t, "Bearer test-key", got.Load())
	})

	t.Run("retries retryable statuses until success", func(t *testing.T) {
		var calls atomic.Int64

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		e := testExporter(t, server.URL)
		require.NoError(t, e.guard.reserve(10))

		e.flush([]envelope{testEnvelope(t)})

		assert.Equal(t, int64(3), calls.Load())
		assert.Equal(t, int64(2), e.inst.deliveredTotal.Load())
		assert.Zero(t, e.inst.failedTotal.Load())
	})

	t.Run("does not retry permanent failures", func(t *testing.T) {
		var calls atomic.Int64

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		e := testExporter(t, server.URL)
		require.NoError(t, e.guard.reserve(10))

		e.flush([]envelope{testEnvelope(t)})

		assert.Equal(t, int64(1), calls.Load())
		assert.Zero(t, e.inst.deliveredTotal.Load())
		assert.Equal(t, int64(2), e.inst.failedTotal.Load())
	})

	t.Run("counts partial-success rejections as failed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := pprofileotlp.NewExportResponse()
			response.PartialSuccess().SetRejectedProfiles(1)
			response.PartialSuccess().SetErrorMessage("bad sample")

			raw, err := response.MarshalProto()
			require.NoError(t, err)

			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(raw)
		}))
		defer server.Close()

		e := testExporter(t, server.URL)
		require.NoError(t, e.guard.reserve(10))

		e.flush([]envelope{testEnvelope(t)})

		assert.Equal(t, int64(1), e.inst.deliveredTotal.Load())
		assert.Equal(t, int64(1), e.inst.failedTotal.Load())
	})
}

func TestExporterQueue(t *testing.T) {
	t.Run("enqueue does not block when the queue is full", func(t *testing.T) {
		e := testExporter(t, "http://localhost:1")
		e.queue = make(chan envelope, 1)

		assert.True(t, e.enqueue(envelope{}))
		assert.False(t, e.enqueue(envelope{}))
		assert.Equal(t, int64(1), e.queueLength())
	})

	t.Run("shutdown flushes queued envelopes", func(t *testing.T) {
		var calls atomic.Int64

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		e := testExporter(t, server.URL)
		require.NoError(t, e.guard.reserve(10))
		require.True(t, e.enqueue(testEnvelope(t)))

		go e.run()
		require.NoError(t, e.shutdown(t.Context()))

		assert.Equal(t, int64(1), calls.Load())
	})
}
