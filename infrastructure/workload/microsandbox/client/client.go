package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	// infoTimeout bounds asking the service which version of the API it
	// speaks, which is asked before anything else.
	infoTimeout = 10 * time.Second

	// answerTimeout bounds a request that is answered rather than streamed.
	// Stopping a run is the slowest of those that pull nothing: its main
	// process is given ten seconds, then killed, and then its VM is stopped.
	answerTimeout = 2 * time.Minute

	// pullTimeout bounds a request that may pull an image: pulling one, and
	// starting or restarting a run, which pulls its image when it is missing.
	// A pull takes as long as the image is large; this only keeps a service
	// that stopped answering from holding a task forever.
	pullTimeout = 30 * time.Minute

	// handshakeTimeout bounds how long the service may take to start an
	// exec: to upgrade the connection, and to say the command started.
	handshakeTimeout = 30 * time.Second

	dialTimeout         = 10 * time.Second
	keepAlive           = 30 * time.Second
	tlsHandshakeTimeout = 10 * time.Second
	idleTimeout         = 90 * time.Second

	// maxIdleConnections is how many connections are kept for the next
	// request. The heartbeats ask several times a second, and every log that
	// is followed holds a connection of its own for as long as it is.
	maxIdleConnections = 16

	// maxAnswer bounds what is read of an answer that is not a stream. A
	// node's runs, listed whole, are the largest of them, at a few kilobytes
	// a run.
	maxAnswer = 16 << 20

	// maxRefusal bounds how much of a refusal is read to say why, and
	// maxQuoted how much of one that is not the service's own is quoted.
	maxRefusal = 64 << 10
	maxQuoted  = 256
)

var (
	// ErrNotAService is a URL the service cannot be reached at.
	ErrNotAService = errors.New("workload-microsandbox is reached at https://host:port")

	// ErrUnreachable is a request that never got an answer.
	ErrUnreachable = errors.New("workload-microsandbox cannot be reached")

	// ErrIncompatible is a service that speaks another major version of the
	// API than this client.
	ErrIncompatible = errors.New("workload-microsandbox speaks another version of its API")
)

// Config is what a client is made from.
type Config struct {
	// URL is where the service answers, as https://host:port. Its host is
	// the name the service's certificate has to answer for.
	URL string

	// Node is the orchestrator the client works for. A run it creates
	// belongs to that node, as a container is labelled with it on docker, and
	// the runs of a task or a slug are looked for among that node's runs.
	Node string

	// Authority, Certificate and PrivateKey are the orchestrator's tunnel
	// credentials, as PEM: the authority the service's certificate is checked
	// against, and the certificate the orchestrator proves itself with, which
	// the authority signed for clientAuth.
	Authority   string
	Certificate string
	PrivateKey  string
}

// Client is the workload-microsandbox service, reached over mutual TLS. It is
// safe for concurrent use, and shared by the Runtime and the NodeManager built
// on it.
//
// Every request names one of the contract's routes, written once in the api
// package and registered by the service as written, so what is asked can be
// read off the contract and nowhere else.
type Client struct {
	base url.URL
	node string

	http *http.Client
	exec *websocket.Dialer

	logger *slog.Logger
	tracer oteltrace.Tracer

	// verified is whether the service was found to speak this client's
	// version of the API since the client last lost it.
	verified atomic.Bool
}

// New makes a client. It reaches nothing yet: an orchestrator starts whether or
// not the service is up, as it does whether or not docker is, and the first
// request is what finds out.
func New(config Config, logger *slog.Logger) (*Client, error) {
	base, err := serviceURL(config.URL)
	if err != nil {
		return nil, err
	}

	if len(config.Node) == 0 {
		return nil, errors.New("workload-microsandbox: a client works for an orchestrator, and was not told which")
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	tlsConfig, err := certificate.ClientTLSConfig(certificate.Credentials{
		Authority:   config.Authority,
		Certificate: config.Certificate,
		PrivateKey:  config.PrivateKey,
		ServerName:  base.Hostname(),
	})
	if err != nil {
		return nil, fmt.Errorf("workload-microsandbox: %w", err)
	}

	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: keepAlive}

	transport := &http.Transport{
		// the service is on a network only orchestrators join, and nothing
		// between them could carry the client's certificate on its behalf.
		Proxy: nil,

		DialContext:         dialer.DialContext,
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: tlsHandshakeTimeout,
		MaxIdleConnsPerHost: maxIdleConnections,
		IdleConnTimeout:     idleTimeout,

		// a log that is followed has to arrive a line at a time, which an
		// answer compressed on the way would not.
		DisableCompression: true,
	}

	return &Client{
		base: base,
		node: config.Node,
		http: &http.Client{
			Transport: transport,

			// the service has nowhere to send anybody: a redirect would be an
			// answer it never meant, so it is taken as the answer it is.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		exec: &websocket.Dialer{
			NetDialContext:   dialer.DialContext,
			TLSClientConfig:  tlsConfig,
			HandshakeTimeout: handshakeTimeout,
			Subprotocols:     []string{api.ExecSubprotocol},
		},
		logger: logger,
		tracer: otel.Tracer("microsandbox"),
	}, nil
}

// serviceURL reads where the service answers. It is https, since nothing else
// carries the client's certificate, and a host with nothing after it, since
// every route names its own path.
func serviceURL(raw string) (url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil ||
		parsed.Scheme != "https" ||
		len(parsed.Hostname()) == 0 ||
		parsed.User != nil ||
		(len(parsed.Path) > 0 && parsed.Path != "/") ||
		len(parsed.RawQuery) > 0 ||
		len(parsed.Fragment) > 0 {
		return url.URL{}, fmt.Errorf("%w, not at %q", ErrNotAService, raw)
	}

	return url.URL{Scheme: parsed.Scheme, Host: parsed.Host}, nil
}

// Close lets go of the connections kept for the next request. What the service
// runs carries on: a run is the service's, not the orchestrator's.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// Info is what the service says about itself.
func (c *Client) Info(ctx context.Context) (api.Info, error) {
	var info api.Info
	if err := c.call(ctx, request{route: api.RouteInfo, timeout: infoTimeout}, &info); err != nil {
		return api.Info{}, err
	}

	return info, nil
}

// verify makes sure the service speaks this client's version of the API before
// it is asked anything else, and refuses one that speaks another rather than
// guess at what it means.
//
// It is asked again after the service could not be reached, since what comes
// back may be another build of it. Two requests that find it unverified at
// once both ask; that costs one request more and holds neither up behind the
// other.
func (c *Client) verify(ctx context.Context) error {
	if c.verified.Load() {
		return nil
	}

	info, err := c.Info(ctx)
	if err != nil {
		return err
	}

	if info.APIVersion != api.Version {
		return fmt.Errorf("%w: it speaks version %q, and this orchestrator version %q", ErrIncompatible, info.APIVersion, api.Version)
	}

	c.verified.Store(true)

	c.logger.InfoContext(ctx, "workload-microsandbox reached",
		"url", c.base.String(),
		"apiVersion", info.APIVersion,
		"serviceVersion", info.ServiceVersion,
		"microsandboxVersion", info.MicrosandboxVersion,
		"architecture", info.Architecture,
		"ready", info.Ready,
		"reason", info.Reason,
	)

	return nil
}

// request is one request to one of the contract's routes.
type request struct {
	route string

	// wildcards fill the route's {id} and {exec}, keyed by api.WildcardRun
	// and api.WildcardExec.
	wildcards map[string]string

	query url.Values

	// body is sent as JSON when there is one.
	body any

	// accept is the content type asked for: JSON, unless it is a stream.
	accept string

	// timeout bounds a request that is answered rather than streamed. Zero
	// is answerTimeout.
	timeout time.Duration
}

// call makes a request and decodes its answer into answer, or reads past it
// when answer is nil.
func (c *Client) call(ctx context.Context, r request, answer any) error {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = answerTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response, err := c.do(ctx, r)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body := io.LimitReader(response.Body, maxAnswer)

	if answer == nil {
		_, err := io.Copy(io.Discard, body)

		return err
	}

	if err := json.NewDecoder(body).Decode(answer); err != nil {
		return fmt.Errorf("workload-microsandbox answered what could not be read: %w", err)
	}

	return nil
}

// do makes a request, once the service is known to speak this client's
// version, and hands back the answer when it is a success. Its body is the
// caller's to close.
func (c *Client) do(ctx context.Context, r request) (*http.Response, error) {
	if r.route != api.RouteInfo {
		if err := c.verify(ctx); err != nil {
			return nil, err
		}
	}

	method, location, err := c.locate(r.route, r.wildcards)
	if err != nil {
		return nil, err
	}

	location.RawQuery = r.query.Encode()

	var body io.Reader
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			return nil, err
		}

		body = bytes.NewReader(encoded)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, method, location.String(), body)
	if err != nil {
		return nil, err
	}

	if r.body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}

	accept := r.accept
	if len(accept) == 0 {
		accept = "application/json"
	}

	httpRequest.Header.Set("Accept", accept)

	// the service's spans continue the orchestrator's, so a task that is slow
	// to start shows where the time went on both sides of the request.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpRequest.Header))

	response, err := c.http.Do(httpRequest)
	if err != nil {
		return nil, c.unreachable(ctx, err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		defer response.Body.Close()

		return nil, refusal(response)
	}

	return response, nil
}

// unreachable is a request that got no answer.
//
// The service may come back as another build of itself, which may speak
// another version of the API, so the version is asked again before the next
// request. A request its caller gave up on says nothing about the service.
func (c *Client) unreachable(ctx context.Context, err error) error {
	if ctx.Err() == nil {
		c.verified.Store(false)
	}

	return fmt.Errorf("%w: %w", ErrUnreachable, err)
}

// locate is where one of the contract's routes is asked for, with its
// wildcards filled in, and with which method. A wildcard left empty is a
// mistake rather than a request: the run or the exec it names is the whole
// point of asking.
func (c *Client) locate(route string, wildcards map[string]string) (string, url.URL, error) {
	method, pattern, found := strings.Cut(route, " ")
	if !found {
		return "", url.URL{}, fmt.Errorf("workload-microsandbox: %q is not one of its routes", route)
	}

	segments := strings.Split(pattern, "/")
	escaped := make([]string, len(segments))

	for i, segment := range segments {
		escaped[i] = segment

		name, isWildcard := strings.CutPrefix(segment, "{")
		if !isWildcard {
			continue
		}

		name = strings.TrimSuffix(name, "}")

		value := wildcards[name]
		if len(value) == 0 {
			return "", url.URL{}, fmt.Errorf("workload-microsandbox: %s was asked for with no %s", route, name)
		}

		segments[i] = value
		escaped[i] = url.PathEscape(value)
	}

	location := c.base
	location.Path = strings.Join(segments, "/")
	location.RawPath = strings.Join(escaped, "/")

	return method, location, nil
}

// refusal reads why the service refused a request.
//
// The service always says so with an ErrorResponse. Anything else came from
// something between the two, or from a service too old to know the route, and
// is passed on as what it is.
func refusal(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxRefusal))

	var answer api.ErrorResponse
	if json.Unmarshal(body, &answer) == nil && len(answer.Error.Code) > 0 {
		return refused(answer.Error)
	}

	code := api.CodeInternal
	if response.StatusCode == http.StatusServiceUnavailable {
		code = api.CodeUnavailable
	}

	message := "workload-microsandbox answered " + response.Status
	if quoted := strings.TrimSpace(string(body)); len(quoted) > 0 {
		message += ": " + truncate(quoted, maxQuoted)
	}

	return &api.Error{Code: code, Message: message}
}

// refused is what the service said, in the workload's own terms: a run or an
// exec that is not there is domain.ErrNotExists, as a container docker does not
// have is, and anything else is the service's own error, whose message is a
// task's failure reason when the task could not be run.
func refused(answer api.Error) error {
	if answer.Code == api.CodeNotFound {
		return domain.ErrNotExists
	}

	return &answer
}

// code is the service's code for why it refused, when it did.
func code(err error) string {
	var refusal *api.Error
	if errors.As(err, &refusal) {
		return refusal.Code
	}

	return ""
}

// truncate shortens what is quoted to size bytes, dropping what a cut leaves of
// a character.
func truncate(text string, size int) string {
	if len(text) <= size {
		return text
	}

	return strings.ToValidUTF8(text[:size], "") + "…"
}

// span starts one of the client's spans, named as docker's are but for
// microsandbox.
func (c *Client) span(ctx context.Context, name string, attributes ...attribute.KeyValue) (context.Context, oteltrace.Span) {
	return c.tracer.Start(ctx, "microsandbox."+name, oteltrace.WithAttributes(attributes...))
}

// runPath names a run in a route.
func runPath(runID string) map[string]string {
	return map[string]string{api.WildcardRun: runID}
}
