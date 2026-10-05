package workload

import (
	"context"
	"errors"
	"io"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// vmhostEngine is the engine this orchestrator's VMs run on: its vmhost, the
// service in the microsandbox container beside it, reached over the unix
// socket the two share.
//
// The vmhost's client is not part of this tree yet. Until it is, this is an
// engine that refuses everything it is asked, so an orchestrator still starts
// and serves what does not need VMs, and says plainly why every VM fails. The
// integration replaces the body with the client:
//
//	return vmhost.NewClient(socket)
func vmhostEngine(socket string) (vm.Engine, error) {
	return unintegratedEngine{}, nil
}

// errNotIntegrated is what an engine that is not there answers.
var errNotIntegrated = errors.New("vmhost client not integrated yet")

// unintegratedEngine refuses every call with errNotIntegrated.
type unintegratedEngine struct{}

var _ vm.Engine = unintegratedEngine{}

func (unintegratedEngine) Info(context.Context) (vm.Info, error) {
	return vm.Info{}, errNotIntegrated
}

func (unintegratedEngine) List(context.Context) ([]vm.Instance, error) {
	return nil, errNotIntegrated
}

func (unintegratedEngine) Inspect(context.Context, string) (vm.Instance, error) {
	return vm.Instance{}, errNotIntegrated
}

func (unintegratedEngine) Create(context.Context, vm.Spec) (vm.Instance, error) {
	return vm.Instance{}, errNotIntegrated
}

func (unintegratedEngine) Start(context.Context, string) error {
	return errNotIntegrated
}

func (unintegratedEngine) Stop(context.Context, string) error {
	return errNotIntegrated
}

func (unintegratedEngine) Restart(context.Context, string) error {
	return errNotIntegrated
}

func (unintegratedEngine) Delete(context.Context, string) error {
	return errNotIntegrated
}

func (unintegratedEngine) Reconfigure(context.Context, vm.Spec) (vm.Instance, error) {
	return vm.Instance{}, errNotIntegrated
}

func (unintegratedEngine) Stats(context.Context, string) (vm.Stats, error) {
	return vm.Stats{}, errNotIntegrated
}

func (unintegratedEngine) Logs(context.Context, string, vm.LogOptions) ([]vm.LogLine, error) {
	return nil, errNotIntegrated
}

func (unintegratedEngine) Exec(context.Context, string, vm.ExecOptions) (vm.ExecSession, error) {
	return nil, errNotIntegrated
}

func (unintegratedEngine) Snapshot(context.Context, string, io.Writer) (vm.Archive, error) {
	return vm.Archive{}, errNotIntegrated
}

func (unintegratedEngine) Restore(context.Context, vm.Spec, io.Reader) (vm.Instance, error) {
	return vm.Instance{}, errNotIntegrated
}
