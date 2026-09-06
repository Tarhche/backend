package ingress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Lister finds the containers whose ports are worth listening on. It is the
// task repository in production; the forwarder asks for no more than this so
// it can be driven by a double in tests.
type Lister interface {
	GetRunningWithPublicPorts(ctx context.Context) ([]task.Task, error)
}

// udpSessionIdle is how long a udp exchange is kept open with nothing said on
// it. A datagram carries no end, so a session is only ever ended by silence.
const udpSessionIdle = 2 * time.Minute

// Forwarder carries whole connections to containers.
//
// The hostname a request is made to is an http idea: ssh, postgres and
// everything else that is not http arrive with nothing but bytes, so they are
// answered on a port of their own instead. Each of a container's exposed ports
// is given one, and what arrives there is handed to the container as it came —
// tcp and udp both, in both directions, until one end stops.
type Forwarder struct {
	lister Lister
	logger *slog.Logger

	// first and last are the ports the forwarder may listen on. A container is
	// given one of them for each port it exposes, by the manager, which is
	// what keeps two containers from being given the same one.
	first, last port.Port

	mutex     sync.Mutex
	listening map[port.Port]*listener
}

// upstream is where one of a container's ports was published on its node —
// one address per protocol, since docker gives each its own.
type upstream struct {
	host string
	tcp  string
	udp  string
}

func (u upstream) address(port string) string {
	return net.JoinHostPort(u.host, port)
}

type listener struct {
	upstream upstream
	close    func()
}

func NewForwarder(lister Lister, portRange string, logger *slog.Logger) (*Forwarder, error) {
	first, last, err := parseRange(portRange)
	if err != nil {
		return nil, err
	}

	return &Forwarder{
		lister:    lister,
		logger:    logger,
		first:     first,
		last:      last,
		listening: make(map[port.Port]*listener),
	}, nil
}

// Serve keeps the forwarder's listeners matching what is running, until the
// context ends.
func (f *Forwarder) Serve(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		f.reconcile(ctx)

		select {
		case <-ctx.Done():
			f.closeAll()

			return
		case <-ticker.C:
		}
	}
}

// reconcile opens a listener for every port a running container was given, and
// closes the ones whose containers are gone.
func (f *Forwarder) reconcile(ctx context.Context) {
	tasks, err := f.lister.GetRunningWithPublicPorts(ctx)
	if err != nil {
		f.logger.ErrorContext(ctx, "failed to read what is running", "error", err)

		return
	}

	wanted := make(map[port.Port]upstream)

	for i := range tasks {
		for _, endpoint := range tasks[i].Endpoints {
			if endpoint.PublicPort < f.first || endpoint.PublicPort > f.last {
				continue
			}

			where := upstream{host: endpoint.Host}

			if endpoint.HostPort != 0 {
				where.tcp = strconv.FormatUint(uint64(endpoint.HostPort), 10)
			}

			if endpoint.HostPortUDP != 0 {
				where.udp = strconv.FormatUint(uint64(endpoint.HostPortUDP), 10)
			}

			if where.tcp == "" && where.udp == "" {
				continue
			}

			wanted[endpoint.PublicPort] = where
		}
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	for public, existing := range f.listening {
		if where, ok := wanted[public]; ok && where == existing.upstream {
			continue
		}

		existing.close()
		delete(f.listening, public)
	}

	for public, where := range wanted {
		if _, ok := f.listening[public]; ok {
			continue
		}

		opened, err := f.listen(ctx, public, where)
		if err != nil {
			f.logger.WarnContext(ctx, "failed to listen for a container", "port", public, "error", err)

			continue
		}

		f.listening[public] = opened
	}
}

// listen accepts tcp connections and udp datagrams on one port, and carries
// them to the container the port was given to.
func (f *Forwarder) listen(ctx context.Context, public port.Port, where upstream) (*listener, error) {
	address := fmt.Sprintf("0.0.0.0:%d", public)

	closers := make([]func(), 0, 2)

	if where.tcp != "" {
		tcp, err := net.Listen("tcp", address)
		if err != nil {
			return nil, err
		}

		closers = append(closers, func() { _ = tcp.Close() })

		go f.acceptTCP(ctx, tcp, where.address(where.tcp))
	}

	if where.udp != "" {
		udp, err := net.ListenPacket("udp", address)
		if err != nil {
			for _, close := range closers {
				close()
			}

			return nil, err
		}

		closers = append(closers, func() { _ = udp.Close() })

		go f.acceptUDP(ctx, udp, where.address(where.udp))
	}

	f.logger.InfoContext(ctx, "forwarding a container's port", "port", public, "tcp", where.tcp, "udp", where.udp)

	return &listener{
		upstream: where,
		close: func() {
			for _, close := range closers {
				close()
			}
		},
	}, nil
}

func (f *Forwarder) acceptTCP(ctx context.Context, l net.Listener, upstream string) {
	for {
		client, err := l.Accept()
		if err != nil {
			// the listener was closed, which is how a container that has gone
			// stops being served.
			return
		}

		go f.pipeTCP(ctx, client, upstream)
	}
}

// pipeTCP hands a connection to the container and lets the two talk until
// either of them stops.
func (f *Forwarder) pipeTCP(ctx context.Context, client net.Conn, upstream string) {
	defer client.Close()

	container, err := net.DialTimeout("tcp", upstream, 5*time.Second)
	if err != nil {
		f.logger.WarnContext(ctx, "a container did not answer", "upstream", upstream, "error", err)

		return
	}

	defer container.Close()

	done := make(chan struct{}, 2)

	copy := func(to, from net.Conn) {
		_, _ = io.Copy(to, from)

		// what is left to read is worth reading: the other half is told there
		// is nothing more coming rather than being cut off.
		if half, ok := to.(interface{ CloseWrite() error }); ok {
			_ = half.CloseWrite()
		}

		done <- struct{}{}
	}

	go copy(container, client)
	go copy(client, container)

	<-done
	<-done
}

// acceptUDP carries datagrams both ways, keeping one socket to the container
// per client so that what comes back reaches the client that asked.
func (f *Forwarder) acceptUDP(ctx context.Context, l net.PacketConn, upstream string) {
	type session struct {
		container net.Conn
		lastUsed  time.Time
	}

	var (
		mutex    sync.Mutex
		sessions = make(map[string]*session)
	)

	buffer := make([]byte, 64<<10)

	for {
		read, from, err := l.ReadFrom(buffer)
		if err != nil {
			return
		}

		mutex.Lock()
		known, ok := sessions[from.String()]
		mutex.Unlock()

		if !ok {
			container, err := net.DialTimeout("udp", upstream, 5*time.Second)
			if err != nil {
				f.logger.WarnContext(ctx, "a container did not answer", "upstream", upstream, "error", err)

				continue
			}

			known = &session{container: container}

			mutex.Lock()
			sessions[from.String()] = known
			mutex.Unlock()

			// what the container says back goes to whoever spoke first, until
			// it has said nothing for long enough.
			go func(client net.Addr, key string, s *session) {
				answer := make([]byte, 64<<10)

				for {
					_ = s.container.SetReadDeadline(time.Now().Add(udpSessionIdle))

					read, err := s.container.Read(answer)
					if err != nil {
						break
					}

					if _, err := l.WriteTo(answer[:read], client); err != nil {
						break
					}
				}

				s.container.Close()

				mutex.Lock()
				delete(sessions, key)
				mutex.Unlock()
			}(from, from.String(), known)
		}

		known.lastUsed = time.Now()

		if _, err := known.container.Write(buffer[:read]); err != nil {
			f.logger.WarnContext(ctx, "failed to hand a datagram to a container", "upstream", upstream, "error", err)
		}
	}
}

func (f *Forwarder) closeAll() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	for public, opened := range f.listening {
		opened.close()
		delete(f.listening, public)
	}
}

// parseRange reads the ports the forwarder may use, written as "first-last".
func parseRange(value string) (port.Port, port.Port, error) {
	trimmed := strings.TrimSpace(value)

	if trimmed == "" {
		return 0, 0, nil
	}

	first, last, found := strings.Cut(trimmed, "-")
	if !found {
		return 0, 0, errors.New("a port range is written as \"first-last\"")
	}

	from, err := strconv.ParseUint(strings.TrimSpace(first), 10, 16)
	if err != nil {
		return 0, 0, err
	}

	to, err := strconv.ParseUint(strings.TrimSpace(last), 10, 16)
	if err != nil {
		return 0, 0, err
	}

	if from == 0 || to < from {
		return 0, 0, errors.New("a port range runs from a port to a higher one")
	}

	return port.Port(from), port.Port(to), nil
}
