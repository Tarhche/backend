// Package vmhost is a node's engine as its orchestrator reaches it: a
// vm.Engine whose every call is a request to the vmhost beside it, over the
// unix socket the two share.
//
// It holds nothing and dials nothing until it is asked something, so an
// orchestrator starts whether its vmhost is there or not, and a vmhost that is
// not answering is an error on each call (ErrUnavailable) rather than an
// orchestrator that cannot start. Everything that can be large is streamed: a
// snapshot's archive goes to the caller's writer as it arrives and a
// restore's is read from the caller's reader as it is sent, since an
// orchestrator has little memory and an archive is as large as a disk. An exec
// session is the connection its request upgraded, carried in frames.
package vmhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

const (
	// tracerName names the spans this client starts.
	tracerName = "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"

	// vmIDKey is the attribute a span names its VM by.
	vmIDKey = "workload.vm.id"

	// base is what requests are addressed to. The socket is what is dialled,
	// so the host is a placeholder.
	base = "http://vmhost"

	// readTimeout bounds a call that only reads, when its caller set no
	// deadline: a vmhost that took the connection and never answers would
	// otherwise hold up whoever asked — the heartbeat first of all, which
	// then could not say the vmhost is down.
	readTimeout = 30 * time.Second

	// changeTimeout bounds a call that changes a VM, when its caller set no
	// deadline. Booting a Docker VM whose image has to be pulled takes
	// minutes; this is only so that a vmhost that stopped answering is given
	// up on eventually. Snapshots, restores and exec sessions are bounded by
	// their callers alone.
	changeTimeout = 30 * time.Minute

	// handshakeTimeout bounds opening an exec session, when its caller set no
	// deadline. The session itself lasts until it is closed.
	handshakeTimeout = 30 * time.Second

	// idleConnections is how many connections are kept for the next call.
	idleConnections = 8

	// idleTimeout is how long one of them is kept unused. It is shorter than
	// the vmhost's own, so the side that would close one is always this one,
	// never a request finding it closed under it.
	idleTimeout = 60 * time.Second
)

// ErrUnavailable is a vmhost that is not answering.
var ErrUnavailable = wire.ErrUnavailable

// Client is a vm.Engine whose VMs are its vmhost's.
type Client struct {
	socket string
	http   *http.Client
	tracer trace.Tracer

	// propagator writes the trace a call is part of into its headers. Nil is
	// whatever the process set globally.
	propagator propagation.TextMapPropagator
}

var _ vm.Engine = &Client{}

// Option configures a Client.
type Option func(*Client)

// WithPropagator is what writes the trace a call is part of into its
// headers, in place of the process's global one.
func WithPropagator(propagator propagation.TextMapPropagator) Option {
	return func(c *Client) {
		c.propagator = propagator
	}
}

// NewClient is the engine its vmhost serves on socket.
func NewClient(socket string, options ...Option) *Client {
	c := &Client{
		socket: socket,
		tracer: otel.Tracer(tracerName),
	}

	for _, option := range options {
		option(c)
	}

	// a transport of its own: every connection is the socket's, none goes
	// through a proxy the environment names, and an archive is already
	// compressed.
	c.http = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			return c.dial(ctx)
		},
		MaxIdleConns:        idleConnections,
		MaxIdleConnsPerHost: idleConnections,
		IdleConnTimeout:     idleTimeout,
		DisableCompression:  true,
	}}

	return c
}

func (c *Client) Info(ctx context.Context) (vm.Info, error) {
	ctx, span := c.start(ctx, "Info", "")
	defer span.End()

	var info wire.Info
	err := c.call(ctx, http.MethodGet, wire.PathInfo, nil, &info, readTimeout)

	return info.ToVM(), record(span, err)
}

func (c *Client) List(ctx context.Context) ([]vm.Instance, error) {
	ctx, span := c.start(ctx, "List", "")
	defer span.End()

	var instances []wire.Instance
	if err := c.call(ctx, http.MethodGet, wire.PathVMs, nil, &instances, readTimeout); err != nil {
		return nil, record(span, err)
	}

	return wire.InstancesToVM(instances), nil
}

func (c *Client) Inspect(ctx context.Context, id string) (vm.Instance, error) {
	ctx, span := c.start(ctx, "Inspect", id)
	defer span.End()

	if len(id) == 0 {
		return vm.Instance{}, notThere(id)
	}

	var instance wire.Instance
	if err := c.call(ctx, http.MethodGet, wire.PathVM(id), nil, &instance, readTimeout); err != nil {
		return vm.Instance{}, record(span, err)
	}

	return instance.ToVM(), nil
}

func (c *Client) Create(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	ctx, span := c.start(ctx, "Create", spec.ID)
	defer span.End()

	var instance wire.Instance
	if err := c.call(ctx, http.MethodPost, wire.PathVMs, wire.NewSpec(spec), &instance, changeTimeout); err != nil {
		return vm.Instance{}, record(span, err)
	}

	return instance.ToVM(), nil
}

func (c *Client) Start(ctx context.Context, id string) error {
	return c.act(ctx, "Start", id, wire.ActionStart)
}

func (c *Client) Stop(ctx context.Context, id string) error {
	return c.act(ctx, "Stop", id, wire.ActionStop)
}

func (c *Client) Restart(ctx context.Context, id string) error {
	return c.act(ctx, "Restart", id, wire.ActionRestart)
}

// act does something to a VM that answers nothing but whether it was done.
func (c *Client) act(ctx context.Context, operation string, id string, action string) error {
	ctx, span := c.start(ctx, operation, id)
	defer span.End()

	if len(id) == 0 {
		return notThere(id)
	}

	return record(span, c.call(ctx, http.MethodPost, wire.PathVM(id, action), nil, nil, changeTimeout))
}

// Delete stops a VM and removes it. Deleting one that is not there is the
// outcome asked for, and so is deleting none.
func (c *Client) Delete(ctx context.Context, id string) error {
	ctx, span := c.start(ctx, "Delete", id)
	defer span.End()

	if len(id) == 0 {
		return nil
	}

	return record(span, c.call(ctx, http.MethodDelete, wire.PathVM(id), nil, nil, changeTimeout))
}

func (c *Client) Reconfigure(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	ctx, span := c.start(ctx, "Reconfigure", spec.ID)
	defer span.End()

	if len(spec.ID) == 0 {
		return vm.Instance{}, notThere(spec.ID)
	}

	var instance wire.Instance
	if err := c.call(ctx, http.MethodPut, wire.PathVM(spec.ID), wire.NewSpec(spec), &instance, changeTimeout); err != nil {
		return vm.Instance{}, record(span, err)
	}

	return instance.ToVM(), nil
}

func (c *Client) Stats(ctx context.Context, id string) (vm.Stats, error) {
	ctx, span := c.start(ctx, "Stats", id)
	defer span.End()

	if len(id) == 0 {
		return vm.Stats{}, notThere(id)
	}

	var stats wire.Stats
	if err := c.call(ctx, http.MethodGet, wire.PathVM(id, wire.ActionStats), nil, &stats, readTimeout); err != nil {
		return vm.Stats{}, record(span, err)
	}

	return stats.ToVM(), nil
}

func (c *Client) Logs(ctx context.Context, id string, options vm.LogOptions) ([]vm.LogLine, error) {
	ctx, span := c.start(ctx, "Logs", id)
	defer span.End()

	if len(id) == 0 {
		return nil, notThere(id)
	}

	query := url.Values{}
	if !options.Since.IsZero() {
		query.Set(wire.QuerySince, options.Since.Format(time.RFC3339Nano))
	}

	if options.Tail > 0 {
		query.Set(wire.QueryTail, strconv.FormatUint(uint64(options.Tail), 10))
	}

	path := wire.PathVM(id, wire.ActionLogs)
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	var lines []wire.LogLine
	if err := c.call(ctx, http.MethodGet, path, nil, &lines, readTimeout); err != nil {
		return nil, record(span, err)
	}

	return wire.LogLinesToVM(lines), nil
}

// call makes one request whose body, when there is one, is JSON, and decodes
// the JSON it is answered with into out, when out is given. A caller that set
// no deadline is given bound.
func (c *Client) call(ctx context.Context, method string, path string, body any, out any, bound time.Duration) error {
	ctx, cancel, bounded := within(ctx, bound)
	defer cancel()

	err := c.exchange(ctx, method, path, body, out)
	if bounded && errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: the vmhost did not answer within %s", err, bound)
	}

	return err
}

// within is ctx bounded by bound when its caller set no deadline, and whether
// it was.
func within(ctx context.Context, bound time.Duration) (context.Context, context.CancelFunc, bool) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}, false
	}

	ctx, cancel := context.WithTimeout(ctx, bound)

	return ctx, cancel, true
}

// exchange is one request and its answer.
func (c *Client) exchange(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}

		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.do(ctx, request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusMultipleChoices {
		return errorOf(response)
	}

	return decodeAnswer(response, out)
}

// decodeAnswer reads the JSON a call was answered with into out, or reads the
// answer to its end when out is nil, so the connection is kept for the next.
func decodeAnswer(response *http.Response, out any) error {
	if out == nil {
		_, _ = io.Copy(io.Discard, response.Body)

		return nil
	}

	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("the vmhost's answer cannot be read: %w", err)
	}

	return nil
}

// do sends a request with the trace it is part of.
//
// A caller that gave up is told so, whatever the transport tripped over first:
// a request streaming a body fails on the closed pipe as often as on the
// context, depending on which of the two noticed first.
func (c *Client) do(ctx context.Context, request *http.Request) (*http.Response, error) {
	c.propagation().Inject(ctx, propagation.HeaderCarrier(request.Header))

	response, err := c.http.Do(request)
	if err != nil {
		if cause := ctx.Err(); cause != nil && !errors.Is(err, cause) {
			return nil, fmt.Errorf("%w: %w", cause, unwrap(err))
		}

		return nil, unwrap(err)
	}

	return response, nil
}

// dial opens a connection to the vmhost. A socket that is not there, or that
// nobody listens on, is a vmhost that is not answering, which is said in as
// many words: an orchestrator whose vmhost is down says so on every VM.
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "unix", c.socket)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		return nil, fmt.Errorf("%w on %s: %w", ErrUnavailable, c.socket, err)
	}

	return conn, nil
}

func (c *Client) start(ctx context.Context, operation string, id string) (context.Context, trace.Span) {
	options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindClient)}
	if len(id) > 0 {
		options = append(options, trace.WithAttributes(attribute.String(vmIDKey, id)))
	}

	return c.tracer.Start(ctx, "vmhost."+operation, options...)
}

func (c *Client) propagation() propagation.TextMapPropagator {
	if c.propagator != nil {
		return c.propagator
	}

	return otel.GetTextMapPropagator()
}

// errorOf is what a response that is not a success says went wrong: the
// vmhost's own error, or, when what answered is not a vmhost, its status.
func errorOf(response *http.Response) error {
	var answered wire.Error

	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answered); err != nil || len(answered.Code) == 0 {
		return &wire.Error{Code: wire.CodeInternal, Message: "the vmhost answered " + response.Status}
	}

	return &answered
}

// unwrap takes off what net/http wraps a failed request in, which says the
// method and the placeholder URL and nothing more of use.
func unwrap(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}

	return err
}

// notThere is what an engine answers for an instance with no name, which is
// not one it could hold.
func notThere(id string) error {
	return fmt.Errorf("%w: no instance %q", domain.ErrNotExists, id)
}

// record marks span as failed with err when err is the vmhost's or the
// connection's failure. An answer such as a VM that is not there is not one.
func record(span trace.Span, err error) error {
	if err == nil {
		return nil
	}

	if wire.ErrorOf(err).Status() >= http.StatusInternalServerError {
		_ = infraTrace.RecordError(span, err)
	}

	return err
}
