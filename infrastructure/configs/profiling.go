package configs

import (
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
)

// Profiling holds the continuous profiling settings and the OTLP export
// settings the profiles signal is delivered with.
//
// The fields the profiler exposes as a map or as a byte count are flattened
// into the scalar forms an operator sets: headers as the "k=v,k2=v2" list the
// OTLP specification defines, and the buffer ceiling in whole megabytes.
type Profiling struct {
	Enabled bool `usage:"Whether continuous profiling runs." env:"PROFILING_ENABLED" long:"profiling-enabled"`

	CPUInterval       time.Duration `usage:"Pause between CPU profile collections." env:"PROFILING_CPU_INTERVAL" long:"profiling-cpu-interval"`
	CPUDuration       time.Duration `usage:"Length of each CPU sampling window." env:"PROFILING_CPU_DURATION" long:"profiling-cpu-duration"`
	MemoryInterval    time.Duration `usage:"Pause between heap profile collections." env:"PROFILING_MEMORY_INTERVAL" long:"profiling-memory-interval"`
	GoroutineInterval time.Duration `usage:"Pause between goroutine profile snapshots." env:"PROFILING_GOROUTINE_INTERVAL" long:"profiling-goroutine-interval"`
	MutexInterval     time.Duration `usage:"Pause between mutex contention profile collections." env:"PROFILING_MUTEX_INTERVAL" long:"profiling-mutex-interval"`
	MutexFraction     int           `usage:"Fraction of mutex contention events sampled: 1/n, or 0 to disable." env:"PROFILING_MUTEX_FRACTION" long:"profiling-mutex-fraction"`
	BlockInterval     time.Duration `usage:"Pause between block contention profile collections." env:"PROFILING_BLOCK_INTERVAL" long:"profiling-block-interval"`
	BlockRate         int           `usage:"Nanoseconds blocked per sampled blocking event, or 0 to disable." env:"PROFILING_BLOCK_RATE" long:"profiling-block-rate"`

	CPULoadThreshold float64 `usage:"Process CPU usage fraction (0..1) above which collections start being skipped." env:"PROFILING_CPU_LOAD_THRESHOLD" long:"profiling-cpu-load-threshold"`
	MinSamplingRate  float64 `usage:"Sampling rate floor the adaptive sampler never drops below." env:"PROFILING_MIN_SAMPLING_RATE" long:"profiling-min-sampling-rate"`
	MaxCPUPercent    float64 `usage:"Acceptable profiling overhead, in percent of the collection interval." env:"PROFILING_MAX_CPU_PERCENT" long:"profiling-max-cpu-percent"`

	MaxProfilesPerMinute int           `usage:"Hard ceiling on the profiles collected per minute." env:"PROFILING_MAX_PROFILES_PER_MINUTE" long:"profiling-max-profiles-per-minute"`
	MaxBufferMB          int           `usage:"Hard ceiling on the buffered profile bytes, in megabytes. 0 keeps the profiler's own limit." env:"PROFILING_MAX_BUFFER_MB" long:"profiling-max-buffer-mb"`
	Timeout              time.Duration `usage:"Bound on a single profile collection." env:"PROFILING_TIMEOUT" long:"profiling-timeout"`

	RedactIPs bool `usage:"Whether IPv4 addresses are redacted from profiles." env:"PROFILING_REDACT_IPS" long:"profiling-redact-ips"`

	ExportBatchSize     int           `usage:"Collections a batch holds before it is exported." env:"PROFILING_EXPORT_BATCH_SIZE" long:"profiling-export-batch-size"`
	ExportFlushInterval time.Duration `usage:"Time after which a partial batch is exported anyway." env:"PROFILING_EXPORT_FLUSH_INTERVAL" long:"profiling-export-flush-interval"`

	Insecure         bool   `usage:"Whether a plain-text OTLP endpoint is accepted." env:"OTEL_EXPORTER_OTLP_INSECURE" long:"otlp-insecure"`
	ProfilesEndpoint string `usage:"OTLP profiles endpoint, used verbatim. Takes precedence over the generic endpoint." env:"OTEL_EXPORTER_OTLP_PROFILES_ENDPOINT" long:"otlp-profiles-endpoint"`
	OTLPEndpoint     string `usage:"Generic OTLP endpoint, which the profiles signal path is appended to." env:"OTEL_EXPORTER_OTLP_ENDPOINT" long:"otlp-endpoint"`
	ProfilesHeaders  string `usage:"Headers added to profiles export requests, as \"k=v,k2=v2\". Takes precedence over the generic headers." env:"OTEL_EXPORTER_OTLP_PROFILES_HEADERS" long:"otlp-profiles-headers"`
	OTLPHeaders      string `usage:"Generic OTLP headers, as \"k=v,k2=v2\"." env:"OTEL_EXPORTER_OTLP_HEADERS" long:"otlp-headers"`
}

// defaultProfiling seeds the flags from the profiler's own defaults, so the
// value a flag reports in the help is the one the profiler would fall back to.
func defaultProfiling() Profiling {
	defaults := profiler.DefaultConfig()

	return Profiling{
		Enabled: defaults.Enabled,

		CPUInterval:       defaults.CPUInterval,
		CPUDuration:       defaults.CPUDuration,
		MemoryInterval:    defaults.MemoryInterval,
		GoroutineInterval: defaults.GoroutineInterval,
		MutexInterval:     defaults.MutexInterval,
		MutexFraction:     defaults.MutexFraction,
		BlockInterval:     defaults.BlockInterval,
		BlockRate:         defaults.BlockRate,

		CPULoadThreshold: defaults.CPULoadThreshold,
		MinSamplingRate:  defaults.MinSamplingRate,
		MaxCPUPercent:    defaults.MaxCPUPercent,

		MaxProfilesPerMinute: defaults.MaxProfilesPerMinute,
		Timeout:              defaults.ProfileTimeout,

		ExportBatchSize:     defaults.BatchSize,
		ExportFlushInterval: defaults.FlushInterval,
	}
}

// ProfilerConfig builds the profiler's configuration from the configured
// values. It starts from the profiler's defaults, so the settings which are not
// exposed as flags keep the values the profiler chose for them.
func (c *Profiling) ProfilerConfig() profiler.Config {
	config := profiler.DefaultConfig()

	config.Enabled = c.Enabled

	config.CPUInterval = c.CPUInterval
	config.CPUDuration = c.CPUDuration
	config.MemoryInterval = c.MemoryInterval
	config.GoroutineInterval = c.GoroutineInterval
	config.MutexInterval = c.MutexInterval
	config.MutexFraction = c.MutexFraction
	config.BlockInterval = c.BlockInterval
	config.BlockRate = c.BlockRate

	config.CPULoadThreshold = c.CPULoadThreshold
	config.MinSamplingRate = c.MinSamplingRate
	config.MaxCPUPercent = c.MaxCPUPercent

	config.MaxProfilesPerMinute = c.MaxProfilesPerMinute
	config.ProfileTimeout = c.Timeout

	// a buffer ceiling is only overridden when one is configured, so an unset
	// value keeps the profiler's own limit rather than dropping it to zero.
	if c.MaxBufferMB > 0 {
		config.MaxBufferBytes = int64(c.MaxBufferMB) << 20
	}

	config.RedactIPs = c.RedactIPs

	config.BatchSize = c.ExportBatchSize
	config.FlushInterval = c.ExportFlushInterval

	config.Insecure = c.Insecure
	config.Endpoint = profiler.ResolveEndpoint(c.ProfilesEndpoint, c.OTLPEndpoint)
	config.Headers = profiler.ParseHeaders(c.ProfilesHeaders, c.OTLPHeaders)

	return config
}
