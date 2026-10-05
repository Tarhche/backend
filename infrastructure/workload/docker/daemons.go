// Package docker reaches the dockerd of each of a node's Docker VMs, and runs
// docker compose in them, through the VMs' engine.
//
// dockerd is never exposed on a network: the Docker client's connections are
// `docker system dial-stdio` commands exec'd into the VM, and compose is a
// command exec'd the same way. So a Docker VM is reachable whatever its
// network allows, by this node alone.
package docker

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/docker/docker/client"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// idleConnections is how many connections into one VM are kept for the
	// next request. Each is a command running in the VM, so a few is plenty.
	idleConnections = 2

	// idleTimeout is how long one of them is kept unused before it is let go,
	// and the command carrying it with it.
	idleTimeout = 30 * time.Second
)

// dialStdio is the command that carries a connection to dockerd over its
// standard input and output.
var dialStdio = []string{"docker", "system", "dial-stdio"}

// Daemons are the dockerds of this node's Docker VMs.
//
// One Docker client is kept per VM, so its connections are kept for the next
// request rather than a command started for every one. A VM going down takes
// those connections with it, so the client is let go of when the VM stops,
// restarts, is replaced or is removed, and when a request through it fails
// for anything but dockerd's own refusal.
type Daemons struct {
	engine       vm.Engine
	readyTimeout time.Duration
	logger       *slog.Logger

	lock    sync.Mutex
	clients map[string]*client.Client
}

// NewDaemons reaches Docker VMs through engine. A dockerd that does not answer
// is waited for up to readyTimeout, which covers a VM that is still booting.
func NewDaemons(engine vm.Engine, readyTimeout time.Duration, logger *slog.Logger) *Daemons {
	return &Daemons{
		engine:       engine,
		readyTimeout: readyTimeout,
		logger:       logger,
		clients:      make(map[string]*client.Client),
	}
}

// Daemon is the dockerd of one Docker VM.
func (d *Daemons) Daemon(vmUUID string) docker.Daemon {
	return &Daemon{daemons: d, vmUUID: vmUUID}
}

// Compose is docker compose in one Docker VM.
func (d *Daemons) Compose(vmUUID string) docker.Compose {
	return NewCompose(d.engine, vmUUID)
}

// Forget lets go of the client of one VM's dockerd, and closes the
// connections it kept. The next request makes a new one.
func (d *Daemons) Forget(vmUUID string) {
	d.lock.Lock()
	cli, ok := d.clients[vmUUID]
	delete(d.clients, vmUUID)
	d.lock.Unlock()

	if ok {
		_ = cli.Close()
	}
}

// client is the Docker client of one VM's dockerd, made when there is none.
func (d *Daemons) client(vmUUID string) (*client.Client, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	if cli, ok := d.clients[vmUUID]; ok {
		return cli, nil
	}

	// a transport of its own: every connection it opens is a command in this
	// VM, and none goes through a proxy the environment may name.
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			return d.dial(ctx, vmUUID)
		},
		MaxIdleConns:        idleConnections,
		MaxIdleConnsPerHost: idleConnections,
		IdleConnTimeout:     idleTimeout,
	}

	// the client keeps the host it would have dialled, a unix socket, only to
	// address the daemon by its placeholder name; every connection is the
	// dialler's.
	cli, err := client.NewClientWithOpts(
		client.WithHTTPClient(&http.Client{Transport: transport, CheckRedirect: client.CheckRedirect}),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}

	d.clients[vmUUID] = cli

	return cli, nil
}

// dial opens a connection to one VM's dockerd: a dial-stdio command exec'd
// into it.
//
// The connection outlives the request that opened it, since it is kept for the
// next one, so the command is not tied to that request's context. Only the
// dialling is: a request that gives up while the command is being started is
// not kept waiting, and the command is ended once it has started.
func (d *Daemons) dial(ctx context.Context, vmUUID string) (net.Conn, error) {
	type dialled struct {
		session vm.ExecSession
		err     error
	}

	result := make(chan dialled, 1)

	go func() {
		session, err := d.engine.Exec(context.WithoutCancel(ctx), vmUUID, vm.ExecOptions{Command: dialStdio})
		result <- dialled{session: session, err: err}
	}()

	select {
	case r := <-result:
		if r.err != nil {
			return nil, r.err
		}

		return newConn(r.session, vmUUID), nil
	case <-ctx.Done():
		go func() {
			if r := <-result; r.err == nil {
				_ = r.session.Close()
			}
		}()

		return nil, ctx.Err()
	}
}
