package profiler

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
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

// ConfigFromEnv builds a Config from the PROFILING_* and standard
// OTEL_EXPORTER_OTLP_* environment variables.
func ConfigFromEnv() (Config, error) {
	cfg := Config{Enabled: true}

	var err error
	if cfg.Enabled, err = envBool("PROFILING_ENABLED", true); err != nil {
		return cfg, err
	}
	if cfg.CPUInterval, err = envDuration("PROFILING_CPU_INTERVAL", defaultCPUInterval); err != nil {
		return cfg, err
	}
	if cfg.CPUDuration, err = envDuration("PROFILING_CPU_DURATION", defaultCPUDuration); err != nil {
		return cfg, err
	}
	if cfg.MemoryInterval, err = envDuration("PROFILING_MEMORY_INTERVAL", defaultMemoryInterval); err != nil {
		return cfg, err
	}
	if cfg.GoroutineInterval, err = envDuration("PROFILING_GOROUTINE_INTERVAL", defaultGoroutineInterval); err != nil {
		return cfg, err
	}
	if cfg.MutexInterval, err = envDuration("PROFILING_MUTEX_INTERVAL", defaultMutexInterval); err != nil {
		return cfg, err
	}
	if cfg.MutexFraction, err = envInt("PROFILING_MUTEX_FRACTION", defaultMutexFraction); err != nil {
		return cfg, err
	}
	if cfg.BlockInterval, err = envDuration("PROFILING_BLOCK_INTERVAL", defaultBlockInterval); err != nil {
		return cfg, err
	}
	if cfg.BlockRate, err = envInt("PROFILING_BLOCK_RATE", defaultBlockRate); err != nil {
		return cfg, err
	}
	if cfg.CPULoadThreshold, err = envFloat("PROFILING_CPU_LOAD_THRESHOLD", defaultCPULoadThreshold); err != nil {
		return cfg, err
	}
	if cfg.MinSamplingRate, err = envFloat("PROFILING_MIN_SAMPLING_RATE", defaultMinSamplingRate); err != nil {
		return cfg, err
	}
	if cfg.MaxCPUPercent, err = envFloat("PROFILING_MAX_CPU_PERCENT", defaultMaxCPUPercent); err != nil {
		return cfg, err
	}
	if cfg.MaxProfilesPerMinute, err = envInt("PROFILING_MAX_PROFILES_PER_MINUTE", defaultMaxProfilesPerMinute); err != nil {
		return cfg, err
	}
	maxBufferMB, err := envInt("PROFILING_MAX_BUFFER_MB", 0)
	if err != nil {
		return cfg, err
	}
	if maxBufferMB > 0 {
		cfg.MaxBufferBytes = int64(maxBufferMB) << 20
	}
	if cfg.ProfileTimeout, err = envDuration("PROFILING_TIMEOUT", defaultProfileTimeout); err != nil {
		return cfg, err
	}
	if cfg.RedactIPs, err = envBool("PROFILING_REDACT_IPS", false); err != nil {
		return cfg, err
	}
	if cfg.BatchSize, err = envInt("PROFILING_EXPORT_BATCH_SIZE", defaultBatchSize); err != nil {
		return cfg, err
	}
	if cfg.FlushInterval, err = envDuration("PROFILING_EXPORT_FLUSH_INTERVAL", defaultFlushInterval); err != nil {
		return cfg, err
	}
	if cfg.Insecure, err = envBool("OTEL_EXPORTER_OTLP_INSECURE", false); err != nil {
		return cfg, err
	}

	cfg.Endpoint = endpointFromEnv()
	cfg.Headers = headersFromEnv()

	return cfg, cfg.normalize()
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

// endpointFromEnv resolves the profiles endpoint the same way the OTLP
// exporters do: the signal-specific variable is used verbatim while the
// generic one gets the signal path appended.
func endpointFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROFILES_ENDPOINT")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); v != "" {
		return strings.TrimRight(v, "/") + profilesURLPath
	}
	return defaultEndpoint
}

// headersFromEnv parses the W3C Correlation-Context style "k=v,k2=v2" list
// used by OTEL_EXPORTER_OTLP_HEADERS.
func headersFromEnv() map[string]string {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROFILES_HEADERS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	}
	if raw == "" {
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

func envBool(name string, fallback bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback, fmt.Errorf("profiler: %s: %w", name, err)
	}

	return parsed, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback, fmt.Errorf("profiler: %s: %w", name, err)
	}

	return parsed, nil
}

func envInt(name string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback, fmt.Errorf("profiler: %s: %w", name, err)
	}

	return parsed, nil
}

func envFloat(name string, fallback float64) (float64, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback, fmt.Errorf("profiler: %s: %w", name, err)
	}

	return parsed, nil
}
