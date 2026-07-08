package profiler

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofileotlp"
)

// errPermanentStatus marks HTTP responses that must not be retried (wrong
// endpoint, malformed payload, ...); they are reported as permanent_error
// instead of retries_exhausted.
var errPermanentStatus = errors.New("permanent status")

// envelope is one collection queued for export.
type envelope struct {
	profiles pprofile.Profiles
	// records is the number of OTLP profile records inside (delivery
	// accounting), rawBytes the guard reservation to release once done.
	records  int64
	rawBytes int64
}

// exporter ships OTLP profiles over OTLP/HTTP protobuf. Envelopes are queued
// without blocking the collectors, batched by size or time, gzip-compressed
// and retried with exponential backoff on retryable failures. Every stage
// feeds the delivery-verification instruments.
type exporter struct {
	endpoint      string
	headers       map[string]string
	client        *http.Client
	timeout       time.Duration
	maxRetries    int
	batchSize     int
	flushInterval time.Duration

	queue chan envelope
	stop  chan struct{}
	done  chan struct{}

	guard  *resourceGuard
	inst   *instruments
	logger *slog.Logger

	backoff func(attempt int) time.Duration
}

func newExporter(cfg Config, guard *resourceGuard, inst *instruments, logger *slog.Logger) *exporter {
	return &exporter{
		endpoint:      cfg.Endpoint,
		headers:       cfg.Headers,
		client:        &http.Client{Timeout: cfg.ExportTimeout},
		timeout:       cfg.ExportTimeout,
		maxRetries:    cfg.MaxRetries,
		batchSize:     cfg.BatchSize,
		flushInterval: cfg.FlushInterval,
		queue:         make(chan envelope, cfg.QueueSize),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
		guard:         guard,
		inst:          inst,
		logger:        logger,
		backoff: func(attempt int) time.Duration {
			return min(500*time.Millisecond<<attempt, 5*time.Second)
		},
	}
}

// queueLength reports the number of queued collections (metrics callback).
func (e *exporter) queueLength() int64 {
	return int64(len(e.queue))
}

// enqueue hands a collection to the exporter without blocking; it reports
// false when the queue is full, in which case the caller keeps ownership of
// the guard reservation.
func (e *exporter) enqueue(env envelope) bool {
	select {
	case e.queue <- env:
		return true
	default:
		return false
	}
}

// run batches and exports until shutdown is called. It is meant to be
// started once in its own goroutine.
func (e *exporter) run() {
	defer close(e.done)

	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()

	batch := make([]envelope, 0, e.batchSize)
	for {
		select {
		case env := <-e.queue:
			batch = append(batch, env)
			if len(batch) >= e.batchSize {
				e.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				e.flush(batch)
				batch = batch[:0]
			}
		case <-e.stop:
			// drain whatever is queued, then a final flush
			for {
				select {
				case env := <-e.queue:
					batch = append(batch, env)
					continue
				default:
				}
				break
			}
			if len(batch) > 0 {
				e.flush(batch)
			}
			return
		}
	}
}

// shutdown stops the run loop after a final drain+flush; it returns early
// when ctx expires.
func (e *exporter) shutdown(ctx context.Context) error {
	close(e.stop)

	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// flush merges the batch into a single OTLP request and sends it.
func (e *exporter) flush(batch []envelope) {
	ctx := context.Background()

	merged := pprofile.NewProfiles()
	var records, rawBytes int64
	for _, env := range batch {
		rawBytes += env.rawBytes
		if err := env.profiles.MergeTo(merged); err != nil {
			e.inst.recordFailed(ctx, env.records, reasonEncodeError)
			e.logger.Warn("profiler: merging profiles for export", slog.String("error", err.Error()))
			continue
		}
		records += env.records
	}
	defer e.guard.release(rawBytes)

	if records == 0 {
		return
	}

	body, err := pprofileotlp.NewExportRequestFromProfiles(merged).MarshalProto()
	if err != nil {
		e.inst.recordFailed(ctx, records, reasonEncodeError)
		e.logger.Warn("profiler: encoding export request", slog.String("error", err.Error()))
		return
	}

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err = zw.Write(body); err == nil {
		err = zw.Close()
	}
	if err != nil {
		e.inst.recordFailed(ctx, records, reasonEncodeError)
		e.logger.Warn("profiler: compressing export request", slog.String("error", err.Error()))
		return
	}

	e.inst.recordSent(ctx, records)

	started := time.Now()
	err = e.send(ctx, compressed.Bytes(), records)
	e.inst.recordExport(ctx, time.Since(started), compressed.Len())

	if err != nil {
		reason := reasonExhausted
		if errors.Is(err, errPermanentStatus) {
			reason = reasonPermanent
		}

		e.inst.recordFailed(ctx, records, reason)
		e.logger.Warn("profiler: export failed",
			slog.String("endpoint", e.endpoint),
			slog.Int64("profiles", records),
			slog.String("error", err.Error()),
		)
	}
}

// send performs the HTTP POST, retrying retryable failures with exponential
// backoff (honouring Retry-After). On success it records delivered/rejected
// counts from the partial-success response.
func (e *exporter) send(ctx context.Context, payload []byte, records int64) error {
	var lastErr error

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if attempt > 0 {
			e.inst.retries.Add(ctx, 1)
		}

		retryAfter, err := e.attempt(ctx, payload, records)
		if err == nil {
			return nil
		}
		lastErr = err

		if retryAfter < 0 {
			return err // not retryable
		}

		delay := e.backoff(attempt)
		if retryAfter > 0 {
			delay = retryAfter
		}

		select {
		case <-time.After(delay):
		case <-e.stop:
			// shutting down: skip the backoff, remaining attempts run
			// back to back so the final flush stays bounded
		}
	}

	return lastErr
}

// attempt sends the payload once. The returned duration is negative when the
// failure is permanent, zero when retryable without a server hint, and
// positive when the server asked to wait (Retry-After).
func (e *exporter) attempt(ctx context.Context, payload []byte, records int64) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(payload))
	if err != nil {
		return -1, err
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "gzip")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return 0, err // network errors are retryable
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		e.confirmDelivery(ctx, resp.Body, records)
		return 0, nil
	case resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusBadGateway ||
		resp.StatusCode == http.StatusServiceUnavailable ||
		resp.StatusCode == http.StatusGatewayTimeout:
		return retryAfter(resp), fmt.Errorf("profiler: export got status %s", resp.Status)
	default:
		return -1, fmt.Errorf("profiler: export got status %s: %w", resp.Status, errPermanentStatus)
	}
}

// confirmDelivery interprets the OTLP partial-success response: rejected
// records count as failed, the remainder as delivered.
func (e *exporter) confirmDelivery(ctx context.Context, body io.Reader, records int64) {
	delivered := records

	raw, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err == nil && len(raw) > 0 {
		response := pprofileotlp.NewExportResponse()
		if err := response.UnmarshalProto(raw); err == nil {
			partial := response.PartialSuccess()
			if rejected := partial.RejectedProfiles(); rejected > 0 {
				rejected = min(rejected, records)
				delivered = records - rejected
				e.inst.recordFailed(ctx, rejected, reasonRejected)
				e.logger.Warn("profiler: backend rejected profiles",
					slog.Int64("rejected", rejected),
					slog.String("message", partial.ErrorMessage()),
				)
			}
		}
	}

	e.inst.recordDelivered(ctx, delivered)
}

func retryAfter(resp *http.Response) time.Duration {
	seconds, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || seconds <= 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}
