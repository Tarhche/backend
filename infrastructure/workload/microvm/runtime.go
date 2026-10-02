package microvm

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Runtime is a class's microVMs, as task.Runtime: a VM for every run, which
// vmhost holds. IDs are vmhost's own; putting the class in front is the
// multiplexer's.
type Runtime struct {
	client *Client
	node   string
}

var (
	_ task.Runtime = (*Runtime)(nil)
	_ task.Dialer  = (*Runtime)(nil)
)

func (r *Runtime) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	return nil, errNotImplemented
}

func (r *Runtime) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	return nil, errNotImplemented
}

func (r *Runtime) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	return nil, errNotImplemented
}

func (r *Runtime) EnsureImage(ctx context.Context, image string) error {
	return errNotImplemented
}

func (r *Runtime) Create(ctx context.Context, execution *task.Execution) (string, error) {
	return "", errNotImplemented
}

func (r *Runtime) Start(ctx context.Context, executionID string) error {
	return errNotImplemented
}

func (r *Runtime) Stop(ctx context.Context, executionID string) error {
	return errNotImplemented
}

func (r *Runtime) Restart(ctx context.Context, executionID string) error {
	return errNotImplemented
}

func (r *Runtime) Kill(ctx context.Context, executionID string) error {
	return errNotImplemented
}

func (r *Runtime) Delete(ctx context.Context, executionID string) error {
	return errNotImplemented
}

func (r *Runtime) Inspect(ctx context.Context, executionID string) (task.Execution, error) {
	return task.Execution{}, errNotImplemented
}

func (r *Runtime) Stats(ctx context.Context, executionID string) (task.Stats, error) {
	return task.Stats{}, errNotImplemented
}

func (r *Runtime) Logs(ctx context.Context, executionID string, writer io.Writer) error {
	return errNotImplemented
}

func (r *Runtime) StreamLogs(ctx context.Context, executionID string, since time.Time, emit func(task.LogLine) error) error {
	return errNotImplemented
}

func (r *Runtime) Exec(ctx context.Context, executionID string, options task.ExecOptions) (task.ExecSession, error) {
	return nil, errNotImplemented
}

// DialContext connects to one of a VM's ports through vmhost: nothing on the
// host has a route into a VM's network.
func (r *Runtime) DialContext(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	return nil, errNotImplemented
}

// Networks are a class's VM networks, which vmhost makes.
type Networks struct {
	client *Client
}

var _ network.Manager = (*Networks)(nil)

func (n *Networks) EnsureIsolatedNetwork(ctx context.Context) error {
	return errNotImplemented
}

func (n *Networks) EnsureStackNetwork(ctx context.Context, stackSlug string) error {
	return errNotImplemented
}

func (n *Networks) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	return errNotImplemented
}

// Node is what a class's VMs on this node use between them.
type Node struct {
	client *Client
}

var _ node.Manager = (*Node)(nil)

func (n *Node) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	return node.Stats{}, errNotImplemented
}
