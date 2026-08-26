package profiler

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config controls collection cadence, resource limits, redaction and export.
// Zero values are replaced with the defaults below by normalize, so a
// zero-initialized Config is usable.
type Config struct {
	// Enabled switches the whole profiler on/off (PROFILING_ENABLED).
	Enabled bool

	// CPUInterval is the base pause between CPU profile collections and
	// CPUDuration the length of each sampling window.
	CPUInterval time.Duration
	CPUDuration time.Duration

	// MemoryInterval is the base pause between heap profile collections.
	MemoryInterval time.Duration

	// GoroutineInterval is the base pause between goroutine profile
	// snapshots.
	GoroutineInterval time.Duration

	// MutexInterval is the base pause between mutex contention profile
	// collections. MutexFraction is passed to
	// runtime.SetMutexProfileFraction: 1/n contention events are sampled;
	// 0 disables mutex profiling entirely (the runtime default).
	MutexInterval time.Duration
	MutexFraction int

	// BlockInterval is the base pause between block contention profile
	// collections (waiting on channels, selects and sync primitives).
	// BlockRate is passed to runtime.SetBlockProfileRate: on average one
	// blocking event is sampled per rate nanoseconds spent blocked; 0
	// disables block profiling entirely (the runtime default).
	BlockInterval time.Duration
	BlockRate     int

	// CPULoadThreshold is the process CPU usage fraction (0..1) above which
	// the adaptive sampler starts skipping collections; MinSamplingRate is
	// the floor it never drops below so some visibility always remains.
	CPULoadThreshold float64
	MinSamplingRate  float64

	// MaxCPUPercent is the acceptable profiling overhead in percent of the
	// collection interval; the overhead monitor stretches the interval when
	// processing a profile costs more than this.
	MaxCPUPercent float64

	// MaxProfilesPerMinute, MaxBufferBytes and ProfileTimeout are the hard
	// resource limits enforced by the guard.
	MaxProfilesPerMinute int
	MaxBufferBytes       int64
	ProfileTimeout       time.Duration

	// Endpoint is the full OTLP/HTTP profiles URL. Headers are added to every
	// export request. Insecure permits a plain-text http endpoint.
	Endpoint string
	Headers  map[string]string
	Insecure bool

	// ExportTimeout bounds a single HTTP attempt, MaxRetries the number of
	// re-sends of a retryable failure. A batch is flushed once it holds
	// BatchSize collections or FlushInterval elapsed, whichever comes first.
	// QueueSize bounds the number of collections waiting to be exported.
	ExportTimeout time.Duration
	MaxRetries    int
	BatchSize     int
	FlushInterval time.Duration
	QueueSize     int

	// RedactPatterns are extra regular expressions (RE2) redacted from
	// profiles in addition to the built-in secret/e-mail patterns.
	// RedactIPs additionally redacts IPv4 addresses.
	RedactPatterns []string
	RedactIPs      bool
}

const (
	defaultCPUInterval       = 60 * time.Second
	defaultCPUDuration       = 10 * time.Second
	defaultMemoryInterval    = 30 * time.Second
	defaultGoroutineInterval = 60 * time.Second
	defaultMutexInterval     = 60 * time.Second
	defaultMutexFraction     = 10
	defaultBlockInterval     = 60 * time.Second
	defaultBlockRate         = 1_000_000 // 1ms of cumulative blocking per sample

	defaultCPULoadThreshold = 0.7
	defaultMinSamplingRate  = 0.1
	defaultMaxCPUPercent    = 5.0

	defaultMaxProfilesPerMinute = 10
	defaultMaxBufferBytes       = 100 << 20 // 100MB
	defaultProfileTimeout       = 30 * time.Second

	defaultExportTimeout = 10 * time.Second
	defaultMaxRetries    = 3
	defaultBatchSize     = 40
	defaultFlushInterval = 5 * time.Minute
	defaultQueueSize     = 64

	// profilesURLPath is the OTLP/HTTP path of the profiles signal. While the
	// signal is in development the collector serves it under this path (the
	// bundled collector v0.151.0 responds 404 on the eventual /v1/profiles).
	profilesURLPath = "/v1development/profiles"

	defaultEndpoint = "http://localhost:4318" + profilesURLPath
)

// DefaultConfig returns the configuration the profiler runs with when nothing
// overrides it. It is the single source of the defaults: the configuration
// layer seeds its flags from here, so the help of a flag and the value the
// profiler falls back to can never drift apart.
func DefaultConfig() Config {
	return Config{
		Enabled: true,

		CPUInterval:       defaultCPUInterval,
		CPUDuration:       defaultCPUDuration,
		MemoryInterval:    defaultMemoryInterval,
		GoroutineInterval: defaultGoroutineInterval,
		MutexInterval:     defaultMutexInterval,
		MutexFraction:     defaultMutexFraction,
		BlockInterval:     defaultBlockInterval,
		BlockRate:         defaultBlockRate,

		CPULoadThreshold: defaultCPULoadThreshold,
		MinSamplingRate:  defaultMinSamplingRate,
		MaxCPUPercent:    defaultMaxCPUPercent,

		MaxProfilesPerMinute: defaultMaxProfilesPerMinute,
		MaxBufferBytes:       defaultMaxBufferBytes,
		ProfileTimeout:       defaultProfileTimeout,

		Endpoint: defaultEndpoint,

		ExportTimeout: defaultExportTimeout,
		MaxRetries:    defaultMaxRetries,
		BatchSize:     defaultBatchSize,
		FlushInterval: defaultFlushInterval,
		QueueSize:     defaultQueueSize,
	}
}

// ResolveEndpoint resolves the profiles endpoint the same way the OTLP
// exporters do: the signal-specific endpoint is used verbatim while the generic
// one gets the signal path appended. It falls back to the default endpoint when
// neither is configured.
func ResolveEndpoint(profilesEndpoint, otlpEndpoint string) string {
	if v := strings.TrimSpace(profilesEndpoint); v != "" {
		return v
	}

	if v := strings.TrimSpace(otlpEndpoint); v != "" {
		return strings.TrimRight(v, "/") + profilesURLPath
	}

	return defaultEndpoint
}

// ParseHeaders parses the W3C Correlation-Context style "k=v,k2=v2" list the
// OTLP headers are configured as. The signal-specific list wins over the
// generic one, and an empty list yields no headers at all.
func ParseHeaders(profilesHeaders, otlpHeaders string) map[string]string {
	raw := strings.TrimSpace(profilesHeaders)
	if len(raw) == 0 {
		raw = strings.TrimSpace(otlpHeaders)
	}

	if len(raw) == 0 {
		return nil
	}

	headers := make(map[string]string)
	for pair := range strings.SplitSeq(raw, ",") {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		if unescaped, err := url.QueryUnescape(strings.TrimSpace(value)); err == nil {
			value = unescaped
		}
		if key != "" {
			headers[key] = value
		}
	}

	return headers
}

// normalize fills unset fields with defaults and validates the endpoint,
// refusing plain-text HTTP unless Insecure is set (security consideration:
// profile payloads travel over TLS by default).
func (c *Config) normalize() error {
	if c.CPUInterval <= 0 {
		c.CPUInterval = defaultCPUInterval
	}
	if c.CPUDuration <= 0 {
		c.CPUDuration = defaultCPUDuration
	}
	if c.ProfileTimeout <= 0 {
		c.ProfileTimeout = defaultProfileTimeout
	}
	// the sampling window itself is bounded by the per-profile timeout
	if c.CPUDuration > c.ProfileTimeout {
		c.CPUDuration = c.ProfileTimeout
	}
	if c.CPUDuration >= c.CPUInterval {
		return fmt.Errorf("profiler: CPU duration (%s) must be shorter than the interval (%s)", c.CPUDuration, c.CPUInterval)
	}
	if c.MemoryInterval <= 0 {
		c.MemoryInterval = defaultMemoryInterval
	}
	if c.GoroutineInterval <= 0 {
		c.GoroutineInterval = defaultGoroutineInterval
	}
	if c.MutexInterval <= 0 {
		c.MutexInterval = defaultMutexInterval
	}
	// MutexFraction stays as-is: 0 (the zero value) keeps mutex profiling off
	if c.MutexFraction < 0 {
		c.MutexFraction = defaultMutexFraction
	}
	if c.BlockInterval <= 0 {
		c.BlockInterval = defaultBlockInterval
	}
	// BlockRate stays as-is: 0 (the zero value) keeps block profiling off
	if c.BlockRate < 0 {
		c.BlockRate = defaultBlockRate
	}
	if c.CPULoadThreshold <= 0 || c.CPULoadThreshold >= 1 {
		c.CPULoadThreshold = defaultCPULoadThreshold
	}
	if c.MinSamplingRate <= 0 || c.MinSamplingRate > 1 {
		c.MinSamplingRate = defaultMinSamplingRate
	}
	if c.MaxCPUPercent <= 0 {
		c.MaxCPUPercent = defaultMaxCPUPercent
	}
	if c.MaxProfilesPerMinute <= 0 {
		c.MaxProfilesPerMinute = defaultMaxProfilesPerMinute
	}
	if c.MaxBufferBytes <= 0 {
		c.MaxBufferBytes = defaultMaxBufferBytes
	}
	if c.ExportTimeout <= 0 {
		c.ExportTimeout = defaultExportTimeout
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = defaultMaxRetries
	}
	if c.BatchSize <= 0 {
		c.BatchSize = defaultBatchSize
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = defaultFlushInterval
	}
	if c.QueueSize <= 0 {
		c.QueueSize = defaultQueueSize
	}
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}

	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return fmt.Errorf("profiler: invalid endpoint %q: %w", c.Endpoint, err)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !c.Insecure {
			return fmt.Errorf("profiler: refusing plain-text endpoint %q; use https or set OTEL_EXPORTER_OTLP_INSECURE=true", c.Endpoint)
		}
	default:
		return fmt.Errorf("profiler: unsupported endpoint scheme %q", u.Scheme)
	}

	return nil
}
