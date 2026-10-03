package microvm

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Client is vmhost's API (domain/workload/vm), reached over its unix socket.
// What vmhost refuses comes back as the error vmhost had, so
// errors.Is(err, vm.ErrCapacity) holds here as it did there.
type Client struct {
	socketPath string
}

// NewClient builds a client for the vmhost at endpoint, a unix socket written
// unix:///path.
func NewClient(endpoint string) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "unix" || len(parsed.Path) == 0 {
		return nil, fmt.Errorf("%q is not a vmhost: it is reached at unix:///path", endpoint)
	}

	return &Client{socketPath: parsed.Path}, nil
}

func (c *Client) Info(ctx context.Context) (vm.Info, error) {
	return vm.Info{}, errNotImplemented
}

func (c *Client) PrepareImage(ctx context.Context, image string) (vm.Image, error) {
	return vm.Image{}, errNotImplemented
}

func (c *Client) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	return vm.Network{}, errNotImplemented
}

func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	return errNotImplemented
}

func (c *Client) Create(ctx context.Context, spec vm.Spec) (string, error) {
	return "", errNotImplemented
}

// VMs is every VM carrying every label filter given, each key=value.
func (c *Client) VMs(ctx context.Context, labels ...string) ([]vm.VM, error) {
	return nil, errNotImplemented
}

func (c *Client) VM(ctx context.Context, id string) (vm.VM, error) {
	return vm.VM{}, errNotImplemented
}

func (c *Client) Start(ctx context.Context, id string) error {
	return errNotImplemented
}

func (c *Client) Stop(ctx context.Context, id string, timeout time.Duration) error {
	return errNotImplemented
}

func (c *Client) Restart(ctx context.Context, id string) error {
	return errNotImplemented
}

func (c *Client) Kill(ctx context.Context, id string) error {
	return errNotImplemented
}

func (c *Client) Delete(ctx context.Context, id string) error {
	return errNotImplemented
}

// Logs hands a VM's output to emit, after the line numbered after, or from
// since; with follow it keeps waiting for more until ctx is done.
func (c *Client) Logs(ctx context.Context, id string, after uint64, since time.Time, follow bool, emit func(vm.LogLine) error) error {
	return errNotImplemented
}

func (c *Client) Stats(ctx context.Context, id string) (vm.Stats, error) {
	return vm.Stats{}, errNotImplemented
}

// Exec starts a command inside a VM and hands back its ID and the connection
// its input and output travel on, as guest frames.
func (c *Client) Exec(ctx context.Context, id string, exec guest.Exec) (string, net.Conn, error) {
	return "", nil, errNotImplemented
}

func (c *Client) EndExec(ctx context.Context, id string, exec string, end guest.EndExec) (guest.Ended, error) {
	return guest.Ended{}, errNotImplemented
}

// Dial connects to one of a VM's task's ports, through vmhost and its agent.
func (c *Client) Dial(ctx context.Context, id string, port uint16) (net.Conn, error) {
	return nil, errNotImplemented
}
