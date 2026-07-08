// Package profiler implements continuous CPU and memory profiling with
// OpenTelemetry-native delivery. Profiles are captured with runtime/pprof,
// sanitized, converted to the OTLP profiles signal (development) and pushed
// over OTLP/HTTP protobuf to the endpoint configured through the standard
// OTEL_EXPORTER_OTLP_* environment variables, so any OTLP-capable backend
// (an OpenTelemetry Collector, Grafana Pyroscope behind a collector, ...)
// can receive them; nothing here is vendor specific.
//
// The design follows the practices described in
// https://oneuptime.com/blog/post/2026-01-07-opentelemetry-continuous-profiling/view:
//
//   - Sampling strategy: CPU profiles are captured in short windows
//     (PROFILING_CPU_DURATION, default 10s) on an interval
//     (PROFILING_CPU_INTERVAL, default 60s); heap profiles on
//     PROFILING_MEMORY_INTERVAL (default 30s), goroutine snapshots on
//     PROFILING_GOROUTINE_INTERVAL (default 60s), mutex contention
//     profiles on PROFILING_MUTEX_INTERVAL (default 60s; the runtime samples
//     1/PROFILING_MUTEX_FRACTION contention events, default 10, 0 switches
//     mutex profiling off) and block profiles on PROFILING_BLOCK_INTERVAL
//     (default 60s; the runtime samples one blocking event per
//     PROFILING_BLOCK_RATE nanoseconds spent blocked, default 1e6, 0
//     switches block profiling off). An adaptive sampler reduces the collection
//     probability linearly from 100% down to PROFILING_MIN_SAMPLING_RATE
//     while process CPU usage exceeds PROFILING_CPU_LOAD_THRESHOLD, so
//     profiling backs off exactly when the service can least afford it.
//
//   - Dynamic overhead adjustment: the time spent producing and encoding
//     each profile is compared with the collection interval; when the
//     overhead ratio exceeds PROFILING_MAX_CPU_PERCENT the interval is
//     stretched (x1.5, capped at ten times the base interval) and shrunk
//     again (x0.8, floored at the base interval) once there is headroom.
//
//   - Resource limits: a guard enforces PROFILING_MAX_PROFILES_PER_MINUTE,
//     caps the memory held by in-flight profile payloads at
//     PROFILING_MAX_BUFFER_MB and bounds per-profile processing time with
//     PROFILING_TIMEOUT.
//
//   - Security: profile payloads can contain sensitive information in pprof
//     labels, comments and symbol names. Every profile is parsed and redacted
//     before leaving the process (secrets/tokens/passwords/API keys, e-mail
//     addresses and, optionally with PROFILING_REDACT_IPS, IPv4 addresses).
//     The raw pprof payload is deliberately not attached to the OTLP message
//     (original_payload stays empty) so unredacted bytes never leave the
//     process, profiles are only ever held in memory, and plain-text HTTP
//     export is refused unless OTEL_EXPORTER_OTLP_INSECURE=true. Transport
//     encryption is TLS via an https endpoint plus optional authentication
//     headers (OTEL_EXPORTER_OTLP_HEADERS); payload-level encryption such as
//     AES-GCM does not apply to OTLP because the collector must be able to
//     decode the message.
//
//   - Export batching: collections are merged into one OTLP request per
//     flush window (PROFILING_EXPORT_BATCH_SIZE collections, default 40, or
//     PROFILING_EXPORT_FLUSH_INTERVAL elapsed, default 5m, whichever comes
//     first). Merging deduplicates the profile dictionaries — the symbol
//     tables that make up most of every payload — so wider windows directly
//     reduce egress.
//
//   - Delivery verification: OpenTelemetry metrics count collected, dropped,
//     sent, delivered and failed profiles, observe export duration, payload
//     size, retries, queue length, in-flight profiles, buffered bytes, the
//     delivery success ratio and the current (adaptive) collection
//     intervals, alongside the runtime memory/GC metrics from the blog post
//     (runtime.memory.heap_alloc_bytes, runtime.memory.heap_objects,
//     runtime.gc.pause_duration_seconds).
//
// Trace correlation follows the pprof-label approach: TracedProfiler attaches
// trace_id/span_id goroutine labels around request handling; CPU samples that
// carry these labels are exported as OTLP profile links to the corresponding
// span instead of plain attributes.
package profiler
