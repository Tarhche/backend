// Package guest is vmhost's side of the guest protocol (domain/workload/guest):
// a client for the agent inside one machine, which it reaches through the unix
// socket the machine's firecracker exposes for its vsock, with the
// "CONNECT 1024" handshake firecracker-go-sdk's vsock package speaks.
//
// It trusts nothing that comes back. A guest runs whatever the task put in it,
// so every answer is bounded in size and time, and an agent whose protocol
// version it does not know is refused rather than misread: a machine can
// outlive the vmhost that booted it, and be adopted by a newer one.
//
// The agent itself, which runs inside the machine, is in agent/.
package guest

import (
	"context"
	"errors"
	"net"
	"time"

	guestProtocol "github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// errNotImplemented is what a stub answers.
var errNotImplemented = errors.New("guest client: not implemented yet")

// Connector makes clients for machines' agents.
type Connector struct{}

var _ vm.GuestConnector = Connector{}

// NewConnector makes clients for machines' agents.
func NewConnector() Connector {
	return Connector{}
}

// Connect is the client for the agent of the machine whose vsock is exposed at
// vsockPath. It connects to nothing until it is asked something.
func (Connector) Connect(vsockPath string) vm.GuestClient {
	return NewClient(vsockPath)
}

// Client speaks to the agent inside one machine.
type Client struct {
	socketPath string
}

var _ vm.GuestClient = (*Client)(nil)

// NewClient builds a client for the machine whose vsock is exposed at
// socketPath.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

func (c *Client) Ready(ctx context.Context) error {
	return errNotImplemented
}

func (c *Client) Configure(ctx context.Context, config guestProtocol.Config) error {
	return errNotImplemented
}

func (c *Client) SetHosts(ctx context.Context, hosts []guestProtocol.Host) error {
	return errNotImplemented
}

func (c *Client) Start(ctx context.Context, process guestProtocol.Process) (guestProtocol.Status, error) {
	return guestProtocol.Status{}, errNotImplemented
}

func (c *Client) Status(ctx context.Context) (guestProtocol.Status, error) {
	return guestProtocol.Status{}, errNotImplemented
}

func (c *Client) Wait(ctx context.Context, generation uint64) (guestProtocol.Status, error) {
	return guestProtocol.Status{}, errNotImplemented
}

func (c *Client) Signal(ctx context.Context, signal int) error {
	return errNotImplemented
}

func (c *Client) Stop(ctx context.Context, timeout time.Duration) (guestProtocol.Status, error) {
	return guestProtocol.Status{}, errNotImplemented
}

func (c *Client) Stats(ctx context.Context) (guestProtocol.Stats, error) {
	return guestProtocol.Stats{}, errNotImplemented
}

func (c *Client) PowerOff(ctx context.Context) error {
	return errNotImplemented
}

func (c *Client) Logs(ctx context.Context, after uint64, follow bool, emit func(guestProtocol.LogLine) error) error {
	return errNotImplemented
}

func (c *Client) Exec(ctx context.Context, exec guestProtocol.Exec) (string, net.Conn, error) {
	return "", nil, errNotImplemented
}

func (c *Client) EndExec(ctx context.Context, id string, end guestProtocol.EndExec) (guestProtocol.Ended, error) {
	return guestProtocol.Ended{}, errNotImplemented
}

func (c *Client) Dial(ctx context.Context, port uint16) (net.Conn, error) {
	return nil, errNotImplemented
}

func (c *Client) Close() error {
	return nil
}
