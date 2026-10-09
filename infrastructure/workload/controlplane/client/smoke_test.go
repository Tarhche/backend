//go:build smoke

package client_test

// The smoke test plays a node against a control plane that is running for
// real, on MongoDB and NATS, and drives it through the blog's client: a VM is
// asked for, its node hears its create and answers it, reports it running,
// answers a question about it, stops it and removes it, and the control
// plane's record follows.
//
//	app serve-workload-controlplane --port 18020
//	SMOKE_CONTROLPLANE_URL=http://127.0.0.1:18020 SMOKE_NATS_URL=nats://127.0.0.1:14222 \
//		go test -tags smoke -run TestSmoke ./infrastructure/workload/controlplane/client/

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	nodeContract "github.com/khanzadimahdi/testproject/domain/workload/node"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

// commands is what the control plane asked of the node.
type commands struct {
	lock  sync.Mutex
	asked []kind.ActOnResource
}

func (c *commands) keep(_ context.Context, payload []byte) error {
	var command kind.ActOnResource
	if err := json.Unmarshal(payload, &command); err != nil {
		return nil
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.asked = append(c.asked, command)

	return nil
}

// addressedTo is the first command for action of the VM vmUUID addressed to
// nodeName, once it has arrived.
func (c *commands) addressedTo(t *testing.T, action string, vmUUID string, nodeName string) kind.ActOnResource {
	t.Helper()

	var found kind.ActOnResource

	require.Eventually(t, func() bool {
		c.lock.Lock()
		defer c.lock.Unlock()

		for _, command := range c.asked {
			if command.Kind == vmKind.Name && command.Action == action && command.UUID == vmUUID && command.Node == nodeName {
				found = command

				return true
			}
		}

		return false
	}, 15*time.Second, 100*time.Millisecond, "the node never heard %s for the vm", action)

	return found
}

func TestSmoke(t *testing.T) {
	controlPlaneURL, natsURL := os.Getenv("SMOKE_CONTROLPLANE_URL"), os.Getenv("SMOKE_NATS_URL")
	if len(controlPlaneURL) == 0 || len(natsURL) == 0 {
		t.Skip("SMOKE_CONTROLPLANE_URL and SMOKE_NATS_URL name no control plane to smoke")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeName := "smoke-node-" + time.Now().Format("150405")
	ownerUUID := "smoke-owner-" + time.Now().Format("150405")

	connection, err := nats.Connect(natsURL)
	require.NoError(t, err)
	defer connection.Close()

	jetstream, err := produceConsumer.NewProduceConsumer(connection, "workload-orchestrator-"+nodeName, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	// the node listens for what it may be asked before anything is asked, as
	// a node's consumers do: every node hears every kind's commands.
	asked := &commands{}
	require.NoError(t, jetstream.Consume(ctx, kind.ActOnResourceName, domain.MessageHandlerFunc(asked.keep)))

	// and answers the control plane's questions on its own subject: a VM's
	// log is the vm kind's logs query.
	subscription, err := connection.Subscribe(noderequest.Subject(nodeName), func(msg *nats.Msg) {
		lines, _ := json.Marshal(vmKind.Logs{Lines: []vmKind.LogLine{{At: time.Now(), Source: vm.LogSourceKernel, Line: "booted"}}})
		reply, _ := json.Marshal(noderequest.Reply{OK: true, Result: lines})
		_ = msg.Respond(reply)
	})
	require.NoError(t, err)
	defer subscription.Unsubscribe()

	capacity := vm.Info{Engine: "microsandbox", Version: "0.7.6", CPUs: 8, Memory: 16 << 30, Disk: 200 << 30}

	// heartbeat says what the node offers, and every VM it holds, each in a
	// heartbeat of its own on the vm kind's subject.
	heartbeat := func(instances ...kind.Observed[vmKind.Status]) {
		t.Helper()

		at := time.Now()

		beat, err := json.Marshal(nodeEvents.Heartbeat{
			Name:     nodeName,
			Role:     nodeContract.OrchestratorRole,
			Capacity: capacity,
			At:       at,
		})
		require.NoError(t, err)
		require.NoError(t, jetstream.Produce(ctx, nodeEvents.HeartbeatName, beat))

		for _, instance := range instances {
			status, err := json.Marshal(instance.Status)
			require.NoError(t, err)

			held, err := json.Marshal(kind.Heartbeat{Node: nodeName, At: at, Observed: kind.Observation{Kind: vmKind.Name, UUID: instance.UUID, Status: status}})
			require.NoError(t, err)
			require.NoError(t, jetstream.Produce(ctx, kind.HeartbeatName(vmKind.Name), held))
		}
	}

	// answer is what the node says a command came to.
	answer := func(command kind.ActOnResource, state kind.State) {
		t.Helper()

		status, err := json.Marshal(vmKind.Status{Status: kind.Status{State: state}})
		require.NoError(t, err)

		result, err := json.Marshal(kind.ResourceActedOn{
			ID:      command.ID,
			Kind:    command.Kind,
			UUID:    command.UUID,
			Action:  command.Action,
			Node:    command.Node,
			Attempt: command.Attempt,
			OK:      true,
			Status:  status,
			At:      time.Now(),
		})
		require.NoError(t, err)
		require.NoError(t, jetstream.Produce(ctx, kind.ResourceActedOnName, result))
	}

	running := func(uuid string, started time.Time, cpu float64) kind.Observed[vmKind.Status] {
		return kind.Observed[vmKind.Status]{UUID: uuid, Status: vmKind.Status{
			Status:    kind.Status{State: vmKind.Running},
			Stats:     &vmKind.Stats{CPUPercent: cpu, MemoryUsed: 256 << 20},
			StartedAt: started,
		}}
	}

	// the node says what it offers, and nothing else is offering more.
	heartbeat()
	time.Sleep(time.Second)

	workload, err := client.New(controlPlaneURL)
	require.NoError(t, err)

	created, err := workload.CreateVM(ctx, ownerUUID, workloadControlPlane.VMRequest{
		Name:      "smoke",
		Kind:      vm.KindMachine,
		Resources: vm.Resources{CPUs: 1, Memory: 1 << 30, Disk: 5 << 30},
		Ports:     []port.Port{22},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
	})
	require.NoError(t, err)

	t.Logf("asked for vm %s (%s)", created.UUID, created.Slug)
	assert.Equal(t, vm.Scheduled, created.CurrentState)
	require.Equal(t, nodeName, created.NodeName, "placed on the only node with room")

	create := asked.addressedTo(t, vmKind.ActionCreate, created.UUID, nodeName)

	made, err := kind.Decode[vmKind.Spec, vmKind.Status](create.Resource)
	require.NoError(t, err)
	assert.Equal(t, ownerUUID, made.Metadata.OwnerUUID, "the command carries the vm as it is recorded")
	assert.Equal(t, created.Slug, made.Metadata.Slug)

	// the node made it, says so, and says it is running from then on.
	started := time.Now()
	answer(create, vmKind.Running)
	heartbeat(running(created.UUID, started, 3.5))

	require.Eventually(t, func() bool {
		v, err := workload.VM(ctx, ownerUUID, created.UUID)

		return err == nil && v.CurrentState == vm.Running && v.Stats.CPUPercent == 3.5
	}, 15*time.Second, 200*time.Millisecond, "the control plane never heard the vm is running")

	lines, err := workload.VMLogs(ctx, ownerUUID, created.UUID, vm.LogOptions{Tail: 10})
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "booted", lines[0].Line)

	require.NoError(t, workload.StopVM(ctx, ownerUUID, created.UUID))
	stop := asked.addressedTo(t, vmKind.ActionStop, created.UUID, nodeName)

	// a heartbeat saying it still runs, before the stop is done, does not
	// undo the stop.
	heartbeat(running(created.UUID, started, 1))
	time.Sleep(500 * time.Millisecond)

	stopping, err := workload.VM(ctx, ownerUUID, created.UUID)
	require.NoError(t, err)
	assert.Equal(t, vm.Stopping, stopping.CurrentState)

	answer(stop, vmKind.Stopped)
	heartbeat(kind.Observed[vmKind.Status]{UUID: created.UUID, Status: vmKind.Status{Status: kind.Status{State: vmKind.Stopped}}})

	require.Eventually(t, func() bool {
		v, err := workload.VM(ctx, ownerUUID, created.UUID)

		return err == nil && v.CurrentState == vm.Stopped
	}, 15*time.Second, 200*time.Millisecond, "the control plane never heard the vm stopped")

	require.NoError(t, workload.DeleteVM(ctx, ownerUUID, created.UUID))
	answer(asked.addressedTo(t, vmKind.ActionDelete, created.UUID, nodeName), vmKind.Deleted)

	require.Eventually(t, func() bool {
		_, err := workload.VM(ctx, ownerUUID, created.UUID)

		return err == domain.ErrNotExists
	}, 15*time.Second, 200*time.Millisecond, "the vm's record never went")
}
