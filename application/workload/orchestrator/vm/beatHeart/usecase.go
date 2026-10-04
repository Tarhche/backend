// Package beatHeart says, every beat, what this node offers to VMs and what
// becomes of the VMs it holds.
package beatHeart

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// Gauges is where what this node holds is published for the dashboards.
type Gauges interface {
	// Node records what the node offers, what it has given, and how many VMs
	// are in each state.
	Node(ctx context.Context, node string, info vm.Info, counts map[vm.State]int)

	// VMHost records whether this node's last call to its engine succeeded.
	VMHost(ctx context.Context, node string, up bool)
}

// UseCase reports this node's VMs.
//
// The engine is the only thing that knows what a VM is doing, and it knows
// instances rather than records, so what goes back is the engine's word for
// each: what it makes of the VM is the control plane's to decide, since it
// alone knows what was asked of it. A VM the beat leaves out is one this node
// does not have. Only VMs are reported here: the instances code-runner tasks
// run in have heartbeats of their own.
type UseCase struct {
	engine   vm.Engine
	producer domain.Producer
	gauges   Gauges
	nodeName string
	logger   *slog.Logger
}

func NewUseCase(engine vm.Engine, producer domain.Producer, gauges Gauges, nodeName string, logger *slog.Logger) *UseCase {
	return &UseCase{engine: engine, producer: producer, gauges: gauges, nodeName: nodeName, logger: logger}
}

func (uc *UseCase) Execute(ctx context.Context) error {
	info, err := uc.engine.Info(ctx)
	if err != nil {
		uc.gauges.VMHost(ctx, uc.nodeName, false)

		return err
	}

	instances, err := uc.engine.List(ctx)
	if err != nil {
		uc.gauges.VMHost(ctx, uc.nodeName, false)

		return err
	}

	uc.gauges.VMHost(ctx, uc.nodeName, true)

	beats := make([]events.VMBeat, 0, len(instances))
	counts := make(map[vm.State]int)

	for _, instance := range instances {
		if instance.Labels[vm.LabelPurpose] != vm.PurposeVM {
			continue
		}

		beat := events.VMBeat{
			UUID:      vmUUID(instance),
			State:     instance.State,
			Reason:    instance.Reason,
			StartedAt: instance.StartedAt,
		}

		// only a running VM uses anything worth sampling.
		if instance.State == vm.InstanceRunning {
			stats, err := uc.engine.Stats(ctx, instance.ID)
			if err != nil {
				// sampled again on the next beat; a VM is no less there for
				// one sample missing.
				uc.logger.WarnContext(ctx, "a vm could not be sampled", "error", err, "vm", instance.ID)
			} else {
				beat.Stats = events.NewStats(stats)
			}
		}

		beats = append(beats, beat)
		counts[stateOf(instance.State)]++
	}

	uc.gauges.Node(ctx, uc.nodeName, info, counts)

	payload, err := json.Marshal(events.VMHeartbeat{
		NodeName: uc.nodeName,
		Capacity: events.NewInfo(info),
		VMs:      beats,
		At:       time.Now(),
	})
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.VMHeartbeatName, payload)
}

// vmUUID is which VM an instance is: its label says, and a VM is created under
// its own uuid when a label has gone missing.
func vmUUID(instance vm.Instance) string {
	if uuid := instance.Labels[vm.LabelVM]; len(uuid) > 0 {
		return uuid
	}

	return instance.ID
}

// stateOf is what an instance's state counts as among a node's VMs. An
// instance that exists and has not booted yet is on its way up.
func stateOf(state vm.InstanceState) vm.State {
	switch state {
	case vm.InstanceRunning:
		return vm.Running
	case vm.InstanceStopped, vm.InstanceExited:
		return vm.Stopped
	case vm.InstanceCreated:
		return vm.Starting
	default:
		return vm.Failed
	}
}
