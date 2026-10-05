// Package vmhost serves a node's engine to its orchestrator: an HTTP API, on
// the unix socket the two share, over any vm.Engine.
//
// It is the whole of a vmhost's surface. microsandbox has no server of its
// own and its SDK may be held by one process only, so the engine lives in the
// vmhost, in the microsandbox container, and the orchestrator reaches it here.
// The orchestrator is redeployed on every push; the vmhost restarts only when
// its own image changes, since restarting it stops every VM it holds. Nothing
// of it is on a network: the socket is the only way in.
//
// What is said is in infrastructure/workload/vm/vmhost/wire: JSON for every
// call, an archive streamed as the body of a snapshot's response and of a
// restore's request, and an exec session carried in frames once its request
// has upgraded the connection. Errors are {code, message}, the domain's
// errors under the names both sides know.
package vmhost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraHttp "github.com/khanzadimahdi/testproject/infrastructure/http"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

const (
	// tracerName names the spans this server starts.
	tracerName = "github.com/khanzadimahdi/testproject/presentation/http/workload/vmhost"

	// vmIDKey is the attribute a span names its VM by.
	vmIDKey = "workload.vm.id"

	// maxBody is the most a JSON body may be: a spec, or an exec's options.
	maxBody = 1 << 20
)

// Server is an engine served over HTTP.
type Server struct {
	engine vm.Engine
	logger *slog.Logger
	tracer trace.Tracer
	mux    *http.ServeMux

	// propagator reads the trace a request is part of off its headers. Nil
	// is whatever the process set globally.
	propagator propagation.TextMapPropagator

	// sessions are the exec sessions being carried. Their connections are no
	// longer HTTP once upgraded, so the http.Server serving this knows
	// nothing of them: Close ends them.
	lock     sync.Mutex
	sessions map[*session]struct{}
	closed   bool
}

var _ http.Handler = &Server{}

// Option configures a Server.
type Option func(*Server)

// WithPropagator is what reads the trace a request is part of off its
// headers, in place of the process's global one.
func WithPropagator(propagator propagation.TextMapPropagator) Option {
	return func(s *Server) {
		s.propagator = propagator
	}
}

// NewServer serves engine.
func NewServer(engine vm.Engine, logger *slog.Logger, options ...Option) *Server {
	s := &Server{
		engine:   engine,
		logger:   logger,
		tracer:   otel.Tracer(tracerName),
		mux:      http.NewServeMux(),
		sessions: make(map[*session]struct{}),
	}

	for _, option := range options {
		option(s)
	}

	s.route()

	return s
}

func (s *Server) route() {
	vmPath := wire.PathVMs + "/{id}"

	s.handle("GET "+wire.PathInfo, s.info)

	s.handle("GET "+wire.PathVMs, s.list)
	s.handle("POST "+wire.PathVMs, s.create)
	s.handle("GET "+vmPath, s.inspect)
	s.handle("PUT "+vmPath, s.reconfigure)
	s.handle("DELETE "+vmPath, s.delete)

	s.handle("POST "+vmPath+"/"+wire.ActionStart, s.start)
	s.handle("POST "+vmPath+"/"+wire.ActionStop, s.stop)
	s.handle("POST "+vmPath+"/"+wire.ActionRestart, s.restart)

	s.handle("GET "+vmPath+"/"+wire.ActionStats, s.stats)
	s.handle("GET "+vmPath+"/"+wire.ActionLogs, s.logs)

	s.handle("POST "+vmPath+"/"+wire.ActionExec, s.exec)
	s.handle("POST "+vmPath+"/"+wire.ActionSnapshot, s.snapshot)
	s.handle("POST "+wire.PathRestore, s.restore)

	// anything else is a client that does not speak this API, which is told
	// so in the shape it reads errors in rather than the mux's own words.
	s.handle("/", func(rw http.ResponseWriter, r *http.Request) {
		s.fail(rw, r, fmt.Errorf("%w: there is no %s %s", wire.ErrInvalid, r.Method, r.URL.Path))
	})
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(rw, r)
}

// Close ends every exec session still being carried, and refuses new ones.
// Shutting the http.Server down does not, since upgraded connections are not
// its any more.
func (s *Server) Close() error {
	s.lock.Lock()
	s.closed = true
	sessions := slices.Collect(maps.Keys(s.sessions))
	s.lock.Unlock()

	for _, session := range sessions {
		session.stop()
	}

	return nil
}

// track keeps a session until it is over, unless the server is closed.
func (s *Server) track(session *session) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return false
	}

	s.sessions[session] = struct{}{}

	return true
}

func (s *Server) untrack(session *session) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.sessions, session)
}

// handle routes pattern to handler inside a span of its own, continuing the
// trace the request says it is part of.
func (s *Server) handle(pattern string, handler http.HandlerFunc) {
	name := "vmhost " + pattern

	s.mux.HandleFunc(pattern, func(rw http.ResponseWriter, r *http.Request) {
		ctx := s.propagation().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		ctx, span := s.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindServer))
		if id := r.PathValue("id"); len(id) > 0 {
			span.SetAttributes(attribute.String(vmIDKey, id))
		}

		recorder := infraHttp.NewResponseWriter(rw, 0, false)

		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					span.End()
					panic(v)
				}

				err := fmt.Errorf("the engine panicked: %v", v)
				_ = infraTrace.RecordError(span, err)
				s.logger.ErrorContext(ctx, "a vmhost request panicked", "panic", v, "stack", string(debug.Stack()))
				s.fail(recorder, r.WithContext(ctx), err)
			}

			status := recorder.Status()
			span.SetAttributes(semconv.HTTPResponseStatusCode(status))
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}

			span.End()
		}()

		handler(recorder, r.WithContext(ctx))
	})
}

func (s *Server) propagation() propagation.TextMapPropagator {
	if s.propagator != nil {
		return s.propagator
	}

	return otel.GetTextMapPropagator()
}

// lasting is a request's context with its cancellation taken off, for a
// change to a VM.
//
// A change the vmhost has begun is carried through even when whoever asked
// for it stops waiting. An engine stopped halfway leaves the VM halfway — a
// create given up on leaves a sandbox that never booted (microsandbox #1687),
// a reconfigure given up on one that is neither the old VM nor the new — and
// what the VM became is in the next heartbeat either way.
func lasting(r *http.Request) context.Context {
	return context.WithoutCancel(r.Context())
}

func (s *Server) info(rw http.ResponseWriter, r *http.Request) {
	info, err := s.engine.Info(r.Context())
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusOK, wire.NewInfo(info))
}

func (s *Server) list(rw http.ResponseWriter, r *http.Request) {
	instances, err := s.engine.List(r.Context())
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusOK, wire.NewInstances(instances))
}

func (s *Server) create(rw http.ResponseWriter, r *http.Request) {
	var spec wire.Spec
	if err := decode(r, &spec); err != nil {
		s.fail(rw, r, err)

		return
	}

	if len(spec.ID) == 0 {
		s.fail(rw, r, fmt.Errorf("%w: a vm is created under an id", wire.ErrInvalid))

		return
	}

	instance, err := s.engine.Create(lasting(r), spec.ToVM())
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusCreated, wire.NewInstance(instance))
}

func (s *Server) inspect(rw http.ResponseWriter, r *http.Request) {
	instance, err := s.engine.Inspect(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusOK, wire.NewInstance(instance))
}

// reconfigure applies a spec to the VM its path names. A spec that names
// another VM is a mistake rather than a way to reach one.
func (s *Server) reconfigure(rw http.ResponseWriter, r *http.Request) {
	var spec wire.Spec
	if err := decode(r, &spec); err != nil {
		s.fail(rw, r, err)

		return
	}

	id := r.PathValue("id")

	switch {
	case len(spec.ID) == 0:
		spec.ID = id
	case spec.ID != id:
		s.fail(rw, r, fmt.Errorf("%w: the spec is %q's and the path %q's", wire.ErrInvalid, spec.ID, id))

		return
	}

	instance, err := s.engine.Reconfigure(lasting(r), spec.ToVM())
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusOK, wire.NewInstance(instance))
}

func (s *Server) delete(rw http.ResponseWriter, r *http.Request) {
	s.change(rw, r, s.engine.Delete)
}

func (s *Server) start(rw http.ResponseWriter, r *http.Request) {
	s.change(rw, r, s.engine.Start)
}

func (s *Server) stop(rw http.ResponseWriter, r *http.Request) {
	s.change(rw, r, s.engine.Stop)
}

func (s *Server) restart(rw http.ResponseWriter, r *http.Request) {
	s.change(rw, r, s.engine.Restart)
}

// change does something to the VM the path names, which answers nothing but
// whether it was done.
func (s *Server) change(rw http.ResponseWriter, r *http.Request, do func(ctx context.Context, id string) error) {
	if err := do(lasting(r), r.PathValue("id")); err != nil {
		s.fail(rw, r, err)

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (s *Server) stats(rw http.ResponseWriter, r *http.Request) {
	stats, err := s.engine.Stats(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusOK, wire.NewStats(stats))
}

// logs is a VM's log, narrowed by since (RFC 3339) and tail (a count of
// lines), either of which may be left out.
func (s *Server) logs(rw http.ResponseWriter, r *http.Request) {
	var options vm.LogOptions

	query := r.URL.Query()

	if since := query.Get(wire.QuerySince); len(since) > 0 {
		at, err := time.Parse(time.RFC3339Nano, since)
		if err != nil {
			s.fail(rw, r, fmt.Errorf("%w: since is a time in RFC 3339: %v", wire.ErrInvalid, err))

			return
		}

		options.Since = at
	}

	if tail := query.Get(wire.QueryTail); len(tail) > 0 {
		lines, err := strconv.ParseUint(tail, 10, 0)
		if err != nil {
			s.fail(rw, r, fmt.Errorf("%w: tail is a number of lines: %v", wire.ErrInvalid, err))

			return
		}

		options.Tail = uint(lines)
	}

	lines, err := s.engine.Logs(r.Context(), r.PathValue("id"), options)
	if err != nil {
		s.fail(rw, r, err)

		return
	}

	respond(rw, http.StatusOK, wire.NewLogLines(lines))
}

// fail answers with what err stands for. Only what is the vmhost's own
// failure marks the request's span as failed: a VM that is not there is an
// answer.
func (s *Server) fail(rw http.ResponseWriter, r *http.Request, err error) {
	answered := wire.ErrorOf(err)

	status := answered.Status()
	if status >= http.StatusInternalServerError {
		_ = infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
	}

	respond(rw, status, answered)
}

func respond(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)

	_ = json.NewEncoder(rw).Encode(body)
}

// decode reads a request's JSON body into v, and the rest of the body with
// it, so nothing of it is left on the connection.
func decode(r *http.Request, v any) error {
	body := http.MaxBytesReader(nil, r.Body, maxBody)

	if err := json.NewDecoder(body).Decode(v); err != nil {
		return fmt.Errorf("%w: the body is not what this API speaks: %v", wire.ErrInvalid, err)
	}

	if _, err := io.Copy(io.Discard, body); err != nil {
		return fmt.Errorf("%w: %v", wire.ErrInvalid, err)
	}

	return nil
}
