//go:build conformance

package microvm_test

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microvm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/runtime/conformance"
)

// TestConformance runs the conformance suite against a real vmhost, as the
// firecracker class runs on one:
//
//	WORKLOAD_CONFORMANCE_VMHOST=unix:///run/workload-vmhost/vmhost.sock \
//	go test -exec 'sudo -E' -tags conformance -run TestConformance ./infrastructure/workload/microvm/
//
// It is run as root because vmhost's socket is its group's, and make
// test-conformance-microvm runs it against a vmhost deployed as production
// deploys it. vmhost is expected to run with its own defaults for what one VM
// may be given (the least and most memory, the most CPUs), which is what the
// class declares. WORKLOAD_CONFORMANCE_OFFLINE and WORKLOAD_CONFORMANCE_FORBIDDEN
// are the container driver's: a backend with no way out to the internet, and
// what a public task must not reach on top of the metadata address.
func TestConformance(t *testing.T) {
	endpoint := os.Getenv("WORKLOAD_CONFORMANCE_VMHOST")
	if len(endpoint) == 0 {
		t.Skip("WORKLOAD_CONFORMANCE_VMHOST names no vmhost to run against")
	}

	spec := driver.Spec{
		Class:    runtime.Firecracker,
		Kind:     driver.KindMicroVM,
		Endpoint: endpoint,
	}

	// a microVM's guest keeps /tmp and /run writable on a read-only root
	suiteOptions := []conformance.Option{conformance.ScratchOnReadOnlyRoot()}

	if len(os.Getenv("WORKLOAD_CONFORMANCE_OFFLINE")) > 0 {
		suiteOptions = append(suiteOptions, conformance.Offline())
	}

	if forbidden := os.Getenv("WORKLOAD_CONFORMANCE_FORBIDDEN"); len(forbidden) > 0 {
		suiteOptions = append(suiteOptions, conformance.WithForbidden(strings.Split(forbidden, ",")...))
	}

	// what one VM may be given is vmhost's configuration, as deployed
	defaults := configs.NewWorkloadVMHost()

	factory := microvm.NewFactory(slog.New(slog.DiscardHandler))

	conformance.Run(t, func(ctx context.Context, node string) (driver.Driver, error) {
		return factory(ctx, node, spec)
	}, runtime.Capabilities{
		Isolation:       runtime.IsolationMicroVM,
		NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
		StackNetworks:   true,
		ReadOnlyRoot:    true,
		DiskLimit:       true,
		TTY:             true,
		RestartPolicies: slices.Clone(vm.RestartPolicies),
		MinMemory:       defaults.MinMemory,
		MaxMemory:       defaults.MaxVMMemory,
		MaxCPU:          defaults.MaxVMCPU,
	}, suiteOptions...)
}
