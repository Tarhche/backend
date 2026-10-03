//go:build conformance

package container_test

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/container"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/runtime/conformance"
)

// TestConformance runs the conformance suite against a real docker daemon,
// as the sysbox class runs on one:
//
//	WORKLOAD_CONFORMANCE_DOCKER_HOST=tcp://127.0.0.1:2375 \
//	WORKLOAD_CONFORMANCE_ADVERTISE_HOST=127.0.0.1 \
//	go test -tags conformance -run TestConformance ./infrastructure/workload/container/
//
// WORKLOAD_CONFORMANCE_ADVERTISE_HOST is where the daemon's published ports
// are reached from here. WORKLOAD_CONFORMANCE_OCI_RUNTIME asks the daemon for
// a runtime of that name (sysbox-runc, runsc). WORKLOAD_CONFORMANCE_CLASS and
// WORKLOAD_CONFORMANCE_NETWORK_PREFIX run it as another class than sysbox,
// with networks of its own, as a second class on the same daemon is.
// WORKLOAD_CONFORMANCE_OFFLINE says the daemon has no way out to the internet,
// and WORKLOAD_CONFORMANCE_FORBIDDEN is what a public task must not reach, as
// host:port separated by commas, on top of the metadata address.
func TestConformance(t *testing.T) {
	endpoint := os.Getenv("WORKLOAD_CONFORMANCE_DOCKER_HOST")
	if len(endpoint) == 0 {
		t.Skip("WORKLOAD_CONFORMANCE_DOCKER_HOST names no docker daemon to run against")
	}

	options := url.Values{}

	if host := os.Getenv("WORKLOAD_CONFORMANCE_ADVERTISE_HOST"); len(host) > 0 {
		options.Set(driver.OptionAdvertiseHost, host)
	}

	if name := os.Getenv("WORKLOAD_CONFORMANCE_OCI_RUNTIME"); len(name) > 0 {
		options.Set(driver.OptionOCIRuntime, name)
	}

	if prefix := os.Getenv("WORKLOAD_CONFORMANCE_NETWORK_PREFIX"); len(prefix) > 0 {
		options.Set(driver.OptionNetworkPrefix, prefix)
	}

	class := runtime.Sysbox
	if named := runtime.Class(os.Getenv("WORKLOAD_CONFORMANCE_CLASS")); len(named) > 0 {
		class = named
	}

	spec := driver.Spec{
		Class:    class,
		Kind:     driver.KindContainer,
		Endpoint: endpoint,
		Options:  options,
	}

	var suiteOptions []conformance.Option

	if len(os.Getenv("WORKLOAD_CONFORMANCE_OFFLINE")) > 0 {
		suiteOptions = append(suiteOptions, conformance.Offline())
	}

	if forbidden := os.Getenv("WORKLOAD_CONFORMANCE_FORBIDDEN"); len(forbidden) > 0 {
		suiteOptions = append(suiteOptions, conformance.WithForbidden(strings.Split(forbidden, ",")...))
	}

	factory := container.NewFactory(slog.New(slog.DiscardHandler))

	conformance.Run(t, func(ctx context.Context, node string) (driver.Driver, error) {
		return factory(ctx, node, spec)
	}, runtime.Capabilities{
		Isolation:       runtime.IsolationContainer,
		NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
		StackNetworks:   true,
		ReadOnlyRoot:    true,
		DiskLimit:       false,
		TTY:             true,
		RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
		MinMemory:       task.MinMemory,
	}, suiteOptions...)
}
