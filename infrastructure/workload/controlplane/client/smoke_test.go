//go:build smoke

package client_test

// The smoke test plays a node against a control plane that is running for
// real, on MongoDB and NATS, and drives it through the blog's client: a VM is
// asked for, its node hears of it, reports it running, answers a question
// about it, stops it and removes it, and the control plane's record follows.
//
//	app migrate
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
	nodeContract "github.com/khanzadimahdi/testproject/domain/workload/node"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

// commands is what the control plane asked of the node, by subject.
type commands struct {
	lock  sync.Mutex
	asked map[string][]json.RawMessage
}

func (c *commands) keep(subject string) domain.MessageHandlerFunc {
	return func(_ context.Context, payload []byte) error {
		c.lock.Lock()
		defer c.lock.Unlock()

		c.asked[subject] = append(c.asked[subject], append(json.RawMessage(nil), payload...))

		return nil
	}
}

// addressedTo is the first command on subject for vmUUID, once it has arrived.
func (c *commands) addressedTo(t *testing.T, subject string, vmUUID string, nodeName string) json.RawMessage {
	t.Helper()

	var found json.RawMessage

	require.Eventually(t, func() bool {
		c.lock.Lock()
		defer c.lock.Unlock()

		for _, payload := range c.asked[subject] {
			var command struct {
				VMUUID   string `json:"vm_uuid"`
				NodeName string `json:"node_name"`
			}

			if json.Unmarshal(payload, &command) == nil && command.VMUUID == vmUUID && command.NodeName == nodeName {
				found = payload

				return true
			}
		}

		return false
	}, 15*time.Second, 100*time.Millisecond, "the node never heard %s for the vm", subject)

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
	// a node's consumers do: every node hears every command.
	asked := &commands{asked: make(map[string][]json.RawMessage)}
	for _, subject := range []string{events.VMScheduledName, events.VMStopRequestedName, events.VMDeleteRequestedName} {
		require.NoError(t, jetstream.Consume(ctx, subject, asked.keep(subject)))
	}

	// and answers the control plane's questions on its own subject.
	subscription, err := connection.Subscribe(noderequest.Subject(nodeName), func(msg *nats.Msg) {
		lines, _ := json.Marshal([]noderequest.VMLogLine{{At: time.Now(), Source: vm.LogSourceKernel, Line: "booted"}})
		reply, _ := json.Marshal(noderequest.Reply{OK: true, Result: lines})
		_ = msg.Respond(reply)
	})
	require.NoError(t, err)
	defer subscription.Unsubscribe()

	capacity := events.Info{Engine: "microsandbox", Version: "0.7.6", CPUs: 8, Memory: 16 << 30, Disk: 200 << 30}

	heartbeat := func(beats ...events.VMBeat) {
		t.Helper()

		nodeBeat, err := json.Marshal(nodeEvents.Heartbeat{Name: nodeName, Role: nodeContract.OrchestratorRole, At: time.Now()})
		require.NoError(t, err)
		require.NoError(t, jetstream.Produce(ctx, nodeEvents.HeartbeatName, nodeBeat))

		vmBeat, err := json.Marshal(events.VMHeartbeat{NodeName: nodeName, Capacity: capacity, VMs: beats, At: time.Now()})
		require.NoError(t, err)
		require.NoError(t, jetstream.Produce(ctx, events.VMHeartbeatName, vmBeat))
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

	var scheduled events.VMScheduled
	require.NoError(t, json.Unmarshal(asked.addressedTo(t, events.VMScheduledName, created.UUID, nodeName), &scheduled))
	assert.Equal(t, created.UUID, scheduled.Spec.ID)
	assert.Equal(t, vm.PurposeVM, scheduled.Spec.Labels[vm.LabelPurpose])
	assert.Equal(t, ownerUUID, scheduled.Spec.Labels[vm.LabelOwner])

	// the node made it and says it is running.
	started := time.Now()
	heartbeat(events.VMBeat{UUID: created.UUID, State: vm.InstanceRunning, StartedAt: started, Stats: events.Stats{CPUPercent: 3.5, MemoryUsed: 256 << 20}})

	require.Eventually(t, func() bool {
		v, err := workload.VM(ctx, ownerUUID, created.UUID)

		return err == nil && v.CurrentState == vm.Running && v.Stats.CPUPercent == 3.5
	}, 15*time.Second, 200*time.Millisecond, "the control plane never heard the vm is running")

	lines, err := workload.VMLogs(ctx, ownerUUID, created.UUID, vm.LogOptions{Tail: 10})
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "booted", lines[0].Line)

	require.NoError(t, workload.StopVM(ctx, ownerUUID, created.UUID))
	asked.addressedTo(t, events.VMStopRequestedName, created.UUID, nodeName)

	// a heartbeat from before the stop reached the node does not undo it.
	heartbeat(events.VMBeat{UUID: created.UUID, State: vm.InstanceRunning, StartedAt: started})
	time.Sleep(500 * time.Millisecond)

	stopping, err := workload.VM(ctx, ownerUUID, created.UUID)
	require.NoError(t, err)
	assert.Equal(t, vm.Stopping, stopping.CurrentState)

	heartbeat(events.VMBeat{UUID: created.UUID, State: vm.InstanceStopped})

	require.Eventually(t, func() bool {
		v, err := workload.VM(ctx, ownerUUID, created.UUID)

		return err == nil && v.CurrentState == vm.Stopped
	}, 15*time.Second, 200*time.Millisecond, "the control plane never heard the vm stopped")

	require.NoError(t, workload.DeleteVM(ctx, ownerUUID, created.UUID))
	asked.addressedTo(t, events.VMDeleteRequestedName, created.UUID, nodeName)

	deleted, err := json.Marshal(events.VMDeleted{VMUUID: created.UUID, NodeName: nodeName, At: time.Now()})
	require.NoError(t, err)
	require.NoError(t, jetstream.Produce(ctx, events.VMDeletedName, deleted))

	require.Eventually(t, func() bool {
		_, err := workload.VM(ctx, ownerUUID, created.UUID)

		return err == domain.ErrNotExists
	}, 15*time.Second, 200*time.Millisecond, "the vm's record never went")
}
