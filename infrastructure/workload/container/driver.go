package container

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	infraNetwork "github.com/khanzadimahdi/testproject/infrastructure/workload/network"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/workload/node"
)

const (
	// aboutTTL is how long what the daemon said about itself is believed. A
	// heartbeat asks for the class's offer every second, and a daemon's
	// version, CPUs and memory do not change from one second to the next.
	aboutTTL = 5 * time.Second

	// aboutTimeout is how long the daemon is given to say it. A daemon that
	// does not answer in that long is not one to run tasks on right now.
	aboutTimeout = 2 * time.Second
)

// restartPolicies are the restart policies docker applies in place.
var restartPolicies = []string{"no", "always", "on-failure", "unless-stopped"}

// options are what a container driver reads off its endpoint.
var options = []string{driver.OptionOCIRuntime, driver.OptionAdvertiseHost, driver.OptionNetworkPrefix}

// Driver is one class of tasks run as containers on a docker daemon: what
// every task was before there were classes, and what the sysbox class still
// is.
//
// A class is a daemon plus how it is asked to run things. The same daemon can
// stand behind more than one: gvisor would be this driver pointed at a daemon
// with runsc installed, asked for it by its OCI runtime, with networks of its
// own.
type Driver struct {
	class    runtime.Class
	endpoint string
	client   *client.Client

	tasks    *DockerManager
	networks *infraNetwork.Manager
	node     *infraNode.Manager

	ociRuntime string

	// about is what the daemon last said about itself.
	about about
}

var _ driver.Driver = &Driver{}

// New builds the driver of one class: the daemon at the spec's endpoint, or
// the docker client's own default when it names none.
//
// It refuses what can never work — an endpoint the docker client cannot use, an
// option it does not know, a network prefix docker would refuse — and nothing
// else. A daemon that does not answer yet is not refused: it is offered
// unhealthy until it does, as an orchestrator whose daemon came up after it
// always waited for it.
func New(ctx context.Context, nodeName string, spec driver.Spec, logger *slog.Logger) (*Driver, error) {
	if spec.Kind != driver.KindContainer {
		return nil, fmt.Errorf("the %q runtime class is of the %q kind, not %q", spec.Class, spec.Kind, driver.KindContainer)
	}

	for name, values := range spec.Options {
		if !slices.Contains(options, name) {
			return nil, fmt.Errorf("a container runtime has no %q option: it reads %s", name, strings.Join(options, ", "))
		}

		if len(values) > 1 {
			return nil, fmt.Errorf("the %q option of the %q runtime class is given more than once", name, spec.Class)
		}
	}

	names, err := infraNetwork.NewNames(spec.Option(driver.OptionNetworkPrefix))
	if err != nil {
		return nil, fmt.Errorf("the networks of the %q runtime class: %w", spec.Class, err)
	}

	cli, err := newClient(spec.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("the %q runtime class: %w", spec.Class, err)
	}

	tasks := NewDockerManager(cli, Scope{
		Node:          nodeName,
		Class:         spec.Class,
		OCIRuntime:    spec.Option(driver.OptionOCIRuntime),
		AdvertiseHost: spec.Option(driver.OptionAdvertiseHost),
		Networks:      names,
	}, logger)

	return &Driver{
		class:      spec.Class,
		endpoint:   spec.Endpoint,
		client:     cli,
		tasks:      tasks,
		networks:   infraNetwork.NewManager(cli, names, logger),
		node:       infraNode.NewManager(tasks),
		ociRuntime: spec.Option(driver.OptionOCIRuntime),
		about:      about{now: time.Now},
	}, nil
}

// NewFactory is how the registry builds drivers of the container kind.
func NewFactory(logger *slog.Logger) driver.Factory {
	return func(ctx context.Context, nodeName string, spec driver.Spec) (driver.Driver, error) {
		built, err := New(ctx, nodeName, spec, logger)
		if err != nil {
			return nil, err
		}

		return built, nil
	}
}

// newClient is a docker client for an endpoint, which an empty one leaves to
// the client's own default. Nothing is asked of the daemon yet.
func newClient(endpoint string) (*client.Client, error) {
	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if len(endpoint) > 0 {
		opts = append(opts, client.WithHost(endpoint))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker cannot be reached at %q: %w", endpoint, err)
	}

	return cli, nil
}

func (d *Driver) Class() runtime.Class {
	return d.class
}

func (d *Driver) Kind() driver.Kind {
	return driver.KindContainer
}

// Tasks are the class's containers. They can be dialled, through the ports
// docker publishes them on.
func (d *Driver) Tasks() task.Runtime {
	return d.tasks
}

func (d *Driver) Networks() network.Manager {
	return d.networks
}

func (d *Driver) Node() node.Manager {
	return d.node
}

// Offer is the class as the daemon runs it, from what the daemon last said
// about itself.
//
// A daemon that cannot be reached still offers the class, unhealthy and with
// what it could do when it last answered, so what it is running is taken for
// unknown rather than for lost. So does one that has no runtime of the name the
// class asks for, which it would refuse every container of the class.
func (d *Driver) Offer(ctx context.Context) runtime.Offer {
	info, err := d.about.get(ctx, d.client.Info)

	offer := runtime.Offer{
		Class:        d.class,
		Driver:       driver.KindContainer.String(),
		Version:      info.ServerVersion,
		Capabilities: capabilities(info),
		Capacity:     capacity(info),
	}

	switch {
	case err != nil:
		offer.Reason = fmt.Sprintf("docker cannot be reached: %v", err)
	case len(d.ociRuntime) > 0 && !hasRuntime(info, d.ociRuntime):
		offer.Reason = fmt.Sprintf("docker has no %q runtime", d.ociRuntime)
	default:
		offer.Healthy = true
	}

	return offer
}

// Close lets go of the daemon. The containers carry on: they are the
// daemon's, not the orchestrator's.
func (d *Driver) Close() error {
	return d.client.Close()
}

// capabilities is what a container class can do on a daemon.
func capabilities(info system.Info) runtime.Capabilities {
	return runtime.Capabilities{
		Isolation: runtime.IsolationContainer,
		NetworkPolicies: []network.Policy{
			network.PolicyNone,
			network.PolicyIsolated,
			network.PolicyPublic,
		},
		StackNetworks: true,
		ReadOnlyRoot:  true,

		// docker can only hold a container to a disk size with storage-opt
		// size=, which needs overlay2 on XFS with project quotas, and the dind
		// daemon the workload runs on has neither. A task's disk limit is
		// accepted and not enforced, which is what this says.
		DiskLimit: false,

		TTY:             true,
		RestartPolicies: slices.Clone(restartPolicies),

		// docker refuses to create a container limited to less than this.
		// There is no upper bound to declare: docker takes any limit, and one
		// asking for more CPUs than the daemon has is refused when it is
		// created, which is where the workload has always heard about it.
		MinMemory: task.MinMemory,

		Architectures: architectures(info.Architecture),
	}
}

// capacity is what a daemon has. Nothing on it is reserved: a container's
// limits are a ceiling it may not pass rather than something held for it, so
// its node shares what it has between every task, and none of it is allocated.
func capacity(info system.Info) runtime.Capacity {
	return runtime.Capacity{
		CPU:      float64(info.NCPU),
		Memory:   uint64(max(info.MemTotal, 0)),
		Reserved: false,
	}
}

// architectures is the architecture a daemon runs images for, as Go names it.
// Docker says what the kernel says, uname's machine name.
func architectures(machine string) []string {
	if len(machine) == 0 {
		return nil
	}

	switch machine {
	case "x86_64", "amd64":
		return []string{"amd64"}
	case "aarch64", "arm64":
		return []string{"arm64"}
	case "armv7l", "armv7", "armhf", "arm":
		return []string{"arm"}
	case "i386", "i686", "386":
		return []string{"386"}
	default:
		return []string{machine}
	}
}

// hasRuntime reports whether a daemon has an OCI runtime of that name.
func hasRuntime(info system.Info, name string) bool {
	_, ok := info.Runtimes[name]

	return ok
}

// about is what a daemon last said about itself.
//
// It is asked for every second and changes about never, so it is kept for a
// while rather than asked of the daemon each time. A daemon that did not
// answer is asked again once that while is up, so a class comes back as soon
// as its daemon does.
type about struct {
	mu sync.Mutex

	// asked is when the daemon was last asked, info what it said the last
	// time it answered, and err what went wrong the last time it was asked.
	asked time.Time
	info  system.Info
	err   error

	now func() time.Time
}

func (a *about) get(ctx context.Context, ask func(context.Context) (system.Info, error)) (system.Info, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.asked.IsZero() && a.now().Sub(a.asked) < aboutTTL {
		return a.info, a.err
	}

	askCtx, cancel := context.WithTimeout(ctx, aboutTimeout)
	defer cancel()

	info, err := ask(askCtx)

	a.asked = a.now()
	a.err = err

	if err == nil {
		a.info = info
	}

	return a.info, a.err
}
