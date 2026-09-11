package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// ErrMalformedForward is a forwarding rule that could not be read.
var ErrMalformedForward = errors.New("tunnel: malformed forward")

// defaultForwardHost is where a rule that names only a port looks for it. A
// worker publishes its containers' ports on its own loopback, so that is what a
// bare port means.
const defaultForwardHost = "127.0.0.1"

// Forward is a port the ingress listens on and where what arrives there goes.
//
// Nothing is dialled to deliver it. The accepted connection becomes a stream on
// one of the connections the worker itself opened, which is the only way into a
// worker there is: a worker has no address, no open port, and nothing to dial.
type Forward struct {
	// Address is what to listen on, as host:port. An empty host listens on
	// every interface.
	Address string

	// Worker is the worker every connection on this port is carried to. Empty
	// leaves the choice to the router, which is how a service several workers
	// offer is spread across them.
	Worker string

	// Target is what the worker connects the stream to once it arrives.
	Target Target
}

func (f Forward) String() string {
	worker := f.Worker
	if len(worker) == 0 {
		worker = "any worker"
	}

	return fmt.Sprintf("%s -> %s %s", f.Address, worker, f.Target)
}

// Validate reports a rule that cannot be served.
func (f Forward) Validate() error {
	if len(f.Address) == 0 {
		return fmt.Errorf("%w: nothing to listen on", ErrMalformedForward)
	}

	if !f.Target.Valid() {
		return fmt.Errorf("%w: %s names no target", ErrMalformedForward, f.Address)
	}

	return nil
}

// ParseForwards reads a comma-separated list of forwarding rules.
func ParseForwards(rules string) ([]Forward, error) {
	forwards := make([]Forward, 0, 1)

	for _, rule := range strings.Split(rules, ",") {
		if rule = strings.TrimSpace(rule); len(rule) == 0 {
			continue
		}

		forward, err := ParseForward(rule)
		if err != nil {
			return nil, err
		}

		forwards = append(forwards, forward)
	}

	return forwards, nil
}

// ParseForward reads one forwarding rule, which is a port to listen on and
// where what arrives there is carried to:
//
//	8022=worker-a:22              a port on that worker's own loopback
//	8080=worker-a:api             a service that worker resolves for itself
//	5432=worker-a:10.0.0.5:5432   an address that worker has to allow
//	9000=:api                     whichever worker the router picks
//	127.0.0.1:8022=worker-a:22    listening on one interface rather than all
//
// A service is the safer of the two: a worker that offers only names cannot be
// talked into connecting anywhere else, while an address is checked against
// what that worker allows.
func ParseForward(rule string) (Forward, error) {
	left, right, found := strings.Cut(rule, "=")
	if !found {
		return Forward{}, fmt.Errorf("%w: %q says no destination", ErrMalformedForward, rule)
	}

	address, err := listenOn(strings.TrimSpace(left))
	if err != nil {
		return Forward{}, err
	}

	worker, destination, found := strings.Cut(strings.TrimSpace(right), ":")
	if !found {
		return Forward{}, fmt.Errorf("%w: %q says no worker", ErrMalformedForward, rule)
	}

	target, err := forwardTarget(strings.TrimSpace(destination))
	if err != nil {
		return Forward{}, err
	}

	forward := Forward{Address: address, Worker: strings.TrimSpace(worker), Target: target}

	return forward, forward.Validate()
}

// listenOn turns the left of a rule into an address to listen on. A bare port
// listens on every interface, which is what a rule that names no host means.
func listenOn(listen string) (string, error) {
	if len(listen) == 0 {
		return "", fmt.Errorf("%w: nothing to listen on", ErrMalformedForward)
	}

	if !strings.Contains(listen, ":") {
		if _, err := strconv.ParseUint(listen, 10, 16); err != nil {
			return "", fmt.Errorf("%w: %q is not a port", ErrMalformedForward, listen)
		}

		return ":" + listen, nil
	}

	if _, _, err := net.SplitHostPort(listen); err != nil {
		return "", fmt.Errorf("%w: %q is not an address: %s", ErrMalformedForward, listen, err)
	}

	return listen, nil
}

// forwardTarget turns the right of a rule into what the worker is asked for: a
// bare port on its loopback, an address it has to allow, or a service it
// resolves for itself.
func forwardTarget(destination string) (Target, error) {
	if len(destination) == 0 {
		return Target{}, fmt.Errorf("%w: no target", ErrMalformedForward)
	}

	if host, port, err := net.SplitHostPort(destination); err == nil {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return Target{}, fmt.Errorf("%w: %q is not a port", ErrMalformedForward, port)
		}

		return Target{Host: host, Port: uint16(number)}, nil
	}

	if number, err := strconv.ParseUint(destination, 10, 16); err == nil {
		if number == 0 {
			return Target{}, fmt.Errorf("%w: port zero", ErrMalformedForward)
		}

		return Target{Host: defaultForwardHost, Port: uint16(number)}, nil
	}

	return Target{Service: destination}, nil
}

// Forwarder is the ingress's edge for traffic that is not the ingress's own.
//
// It is layer four and nothing more: it accepts a connection and joins it to a
// stream, having read none of it. Which worker it goes to is the port it
// arrived on, because a raw connection carries nothing that could name one —
// there is no host header to read and no path to route on, which is the whole
// reason the mapping is configuration rather than something read off the wire.
//
// A port whose worker is not connected refuses by closing, since at layer four
// there is nothing to say and nowhere to say it.
type Forwarder struct {
	ingress *Ingress
	proxy   StreamProxy
	logger  *slog.Logger

	forwards []Forward

	lock      sync.Mutex
	closing   bool
	opened    bool
	listeners []net.Listener
	carrying  map[net.Conn]struct{}

	closed    chan struct{}
	closeOnce sync.Once
	wait      sync.WaitGroup
}

// NewForwarder builds the edge. It carries onto an ingress rather than dialling
// anything itself, so a forwarder without one has nowhere to put a connection.
func NewForwarder(ingress *Ingress, logger *slog.Logger, forwards ...Forward) (*Forwarder, error) {
	if ingress == nil {
		return nil, errors.New("tunnel: a forwarder needs an ingress to carry onto")
	}

	for _, forward := range forwards {
		if err := forward.Validate(); err != nil {
			return nil, err
		}
	}

	return &Forwarder{
		ingress:  ingress,
		logger:   logger,
		forwards: slices.Clone(forwards),
		carrying: make(map[net.Conn]struct{}),
		closed:   make(chan struct{}),
	}, nil
}

// Forwards is what this was told to listen on.
func (f *Forwarder) Forwards() []Forward { return slices.Clone(f.forwards) }

// Listening is what it actually bound, in the order the forwards were given. A
// rule naming port zero is given one by the operating system, so this is the
// only way to learn which.
func (f *Forwarder) Listening() []net.Addr {
	f.lock.Lock()
	defer f.lock.Unlock()

	addresses := make([]net.Addr, 0, len(f.listeners))
	for _, listener := range f.listeners {
		addresses = append(addresses, listener.Addr())
	}

	return addresses
}

// Listen opens every forwarded port without serving any of them, so that a port
// already taken is a refusal to start rather than something discovered once
// traffic is arriving on the others. Serve opens them itself if this was not
// called.
func (f *Forwarder) Listen() error {
	_, err := f.open()

	return err
}

// Serve carries what arrives on every forwarded port until ctx is done or Close
// is called.
func (f *Forwarder) Serve(ctx context.Context) error {
	listeners, err := f.open()
	if err != nil {
		return err
	}

	if len(listeners) == 0 {
		return nil
	}

	stop := make(chan struct{})
	defer close(stop)

	go func() {
		select {
		case <-ctx.Done():
		case <-f.closed:
		case <-stop:
		}

		f.shut()
	}()

	var (
		serving sync.WaitGroup
		failed  = make(chan error, len(listeners))
	)

	for index, listener := range listeners {
		serving.Add(1)

		go func(forward Forward, listener net.Listener) {
			defer serving.Done()

			failed <- f.serve(ctx, forward, listener)
		}(f.forwards[index], listener)
	}

	serving.Wait()
	close(failed)

	return errors.Join(slices.Collect(func(yield func(error) bool) {
		for err := range failed {
			if err != nil && !yield(err) {
				return
			}
		}
	})...)
}

// open listens on every forwarded port, and lets go of the ones it got if any of
// them could not be had. Opening twice returns what is already open, so Listen
// followed by Serve listens once.
func (f *Forwarder) open() ([]net.Listener, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closing {
		return nil, nil
	}

	if f.opened {
		return slices.Clone(f.listeners), nil
	}

	listeners := make([]net.Listener, 0, len(f.forwards))

	for _, forward := range f.forwards {
		listener, err := net.Listen("tcp", forward.Address)
		if err != nil {
			closeAll(listeners)

			return nil, fmt.Errorf("tunnel: forwarding %s: %w", forward.Address, err)
		}

		listeners = append(listeners, listener)
	}

	f.opened = true
	f.listeners = listeners

	return slices.Clone(listeners), nil
}

// serve carries what arrives on one port for as long as it is open.
func (f *Forwarder) serve(ctx context.Context, forward Forward, listener net.Listener) error {
	f.logger.InfoContext(ctx, "forwarding a port into the tunnel",
		"listening", listener.Addr().String(),
		"forward", forward.String(),
	)

	for {
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || f.isClosed() {
				return nil
			}

			return err
		}

		if !f.starting(client) {
			client.Close()

			return nil
		}

		go func() {
			defer f.wait.Done()
			defer f.finished(client)

			f.carry(ctx, forward, client)
		}()
	}
}

// carry joins one accepted connection to a stream on the worker's own
// connection, and stays with it until it is over.
func (f *Forwarder) carry(ctx context.Context, forward Forward, client net.Conn) {
	stream, err := f.open1(ctx, forward)
	if err != nil {
		// there is no way to refuse at layer four but to close: whatever is on
		// the other side is speaking a protocol this knows nothing about.
		f.logger.WarnContext(ctx, "a forwarded connection reached no worker",
			"forward", forward.String(),
			"client", client.RemoteAddr().String(),
			"error", err,
		)
		client.Close()

		return
	}

	sent, received, err := f.proxy.Copy(client, stream)
	if err != nil {
		f.logger.DebugContext(ctx, "a forwarded connection ended badly",
			"forward", forward.String(),
			"sent", sent,
			"received", received,
			"error", err,
		)
	}
}

// open1 asks the tunnel for a stream, either to the worker the port names or to
// whichever one the router picks.
func (f *Forwarder) open1(ctx context.Context, forward Forward) (net.Conn, error) {
	if len(forward.Worker) == 0 {
		return f.ingress.Route(ctx, forward.Target)
	}

	return f.ingress.Dial(ctx, forward.Worker, forward.Target)
}

func (f *Forwarder) starting(client net.Conn) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closing {
		return false
	}

	f.wait.Add(1)
	f.carrying[client] = struct{}{}

	return true
}

func (f *Forwarder) finished(client net.Conn) {
	f.lock.Lock()
	defer f.lock.Unlock()

	delete(f.carrying, client)
}

func (f *Forwarder) isClosed() bool {
	select {
	case <-f.closed:
		return true
	default:
		return false
	}
}

// shut stops listening and ends what is being carried.
//
// It ends them rather than draining them because what this carries has no
// length: an ssh session lasts as long as someone is typing, and waiting for
// that is a process that never exits. A client sees its connection close, which
// is what a client of any TCP service sees when the service goes away.
func (f *Forwarder) shut() {
	f.lock.Lock()
	f.closing = true
	listeners := f.listeners
	f.listeners = nil
	carrying := slices.Collect(maps.Keys(f.carrying))
	clear(f.carrying)
	f.lock.Unlock()

	closeAll(listeners)

	for _, client := range carrying {
		client.Close()
	}
}

// Close stops listening, ends what is being carried, and waits for the
// goroutines carrying it, so that a closed forwarder leaves nothing running.
func (f *Forwarder) Close() error {
	f.closeOnce.Do(func() {
		close(f.closed)
		f.shut()
	})

	f.wait.Wait()

	return nil
}

func closeAll(listeners []net.Listener) {
	for _, listener := range listeners {
		listener.Close()
	}
}
