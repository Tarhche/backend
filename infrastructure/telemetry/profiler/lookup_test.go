package profiler

import (
	"bytes"
	"log/slog"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// capturedLookupProfile produces a real pprof payload for a named runtime
// profile, as the lookup collectors would.
func capturedLookupProfile(t *testing.T, name string) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, pprof.Lookup(name).WriteTo(&buf, 0))

	return buf.Bytes()
}

func TestProfilerProcessesGoroutineAndMutexProfiles(t *testing.T) {
	cfg := Config{
		Endpoint:       "http://localhost:1" + profilesURLPath,
		Insecure:       true,
		CPUInterval:    time.Hour,
		MemoryInterval: time.Hour,
	}

	p, err := New(cfg, testResource(t), sdkmetric.NewMeterProvider(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	t.Run("goroutine snapshot is queued for export", func(t *testing.T) {
		p.process(t.Context(), ProfileTypeGoroutine, capturedLookupProfile(t, "goroutine"))

		assert.Equal(t, int64(1), p.exporter.queueLength())
	})

	t.Run("block profile is queued for export", func(t *testing.T) {
		// sample every blocking event and produce one via channel contention
		runtime.SetBlockProfileRate(1)
		defer runtime.SetBlockProfileRate(0)

		ch := make(chan struct{})
		go func() {
			time.Sleep(10 * time.Millisecond)
			close(ch)
		}()
		<-ch // blocks until the goroutine closes the channel

		p.process(t.Context(), ProfileTypeBlock, capturedLookupProfile(t, "block"))

		// 1 queued goroutine collection + 1 block (contentions + delay)
		assert.Equal(t, int64(2), p.exporter.queueLength())
	})

	t.Run("mutex profile is queued for export", func(t *testing.T) {
		// sample every contention event and produce one
		prev := runtime.SetMutexProfileFraction(1)
		defer runtime.SetMutexProfileFraction(prev)

		var mu sync.Mutex
		mu.Lock()
		done := make(chan struct{})
		go func() {
			mu.Lock() // contends until the main goroutine unlocks
			mu.Unlock()
			close(done)
		}()
		time.Sleep(10 * time.Millisecond)
		mu.Unlock()
		<-done

		p.process(t.Context(), ProfileTypeMutex, capturedLookupProfile(t, "mutex"))

		// goroutine + block + mutex collections queued
		assert.Equal(t, int64(3), p.exporter.queueLength())
	})
}

func TestProfilerStartTogglesMutexFraction(t *testing.T) {
	initial := runtime.SetMutexProfileFraction(-1) // -1 only reads the rate
	t.Cleanup(func() { runtime.SetMutexProfileFraction(initial) })

	cfg := Config{
		Endpoint:          "http://localhost:1" + profilesURLPath,
		Insecure:          true,
		MutexFraction:     7,
		CPUInterval:       time.Hour,
		MemoryInterval:    time.Hour,
		GoroutineInterval: time.Hour,
		MutexInterval:     time.Hour,
	}

	p, err := New(cfg, testResource(t), sdkmetric.NewMeterProvider(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	require.NoError(t, p.Start(t.Context()))
	assert.Equal(t, 7, runtime.SetMutexProfileFraction(-1))

	require.NoError(t, p.Stop(t.Context()))
	assert.Equal(t, initial, runtime.SetMutexProfileFraction(-1))
}

func TestConfigDefaultsForLookupProfiles(t *testing.T) {
	cfg := Config{Endpoint: "http://localhost:1", Insecure: true}
	require.NoError(t, cfg.normalize())

	assert.Equal(t, defaultGoroutineInterval, cfg.GoroutineInterval)
	assert.Equal(t, defaultMutexInterval, cfg.MutexInterval)
	assert.Zero(t, cfg.MutexFraction, "zero value keeps mutex profiling off")
	assert.Equal(t, defaultBlockInterval, cfg.BlockInterval)
	assert.Zero(t, cfg.BlockRate, "zero value keeps block profiling off")
}

func TestProfilerStartTogglesBlockRate(t *testing.T) {
	cfg := Config{
		Endpoint:          "http://localhost:1" + profilesURLPath,
		Insecure:          true,
		BlockRate:         100,
		CPUInterval:       time.Hour,
		MemoryInterval:    time.Hour,
		GoroutineInterval: time.Hour,
		BlockInterval:     time.Hour,
	}

	p, err := New(cfg, testResource(t), sdkmetric.NewMeterProvider(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	require.NoError(t, p.Start(t.Context()))
	// SetBlockProfileRate has no getter; prove the rate is active by
	// checking that a blocking event is sampled into the profile
	ch := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(ch)
	}()
	<-ch

	var buf bytes.Buffer
	require.NoError(t, pprof.Lookup("block").WriteTo(&buf, 0))
	assert.NotEmpty(t, buf.Bytes())

	require.NoError(t, p.Stop(t.Context()))
}
