package workload

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"
)

// vmhostEngine is the engine this orchestrator's VMs run on: its vmhost, the
// service in the microsandbox container beside it, reached over the unix
// socket the two share.
//
// Nothing is dialled until the engine is first asked something, so an
// orchestrator starts whether its vmhost is up or not, and one whose vmhost
// is not answering says so on every VM and in its heartbeat rather than
// failing to start: on a machine with no KVM, VMs fail and say why.
func vmhostEngine(socket string) (vm.Engine, error) {
	if len(socket) == 0 {
		return nil, errors.New("WORKLOAD_VMHOST_SOCKET is where this orchestrator's vmhost serves its engine, and has to be said")
	}

	return vmhost.NewClient(socket), nil
}
