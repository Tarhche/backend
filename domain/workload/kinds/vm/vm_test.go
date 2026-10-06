package vm_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := vmKind.Descriptor()

	t.Run("it keeps every rule a kind keeps", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, kind.Check(d))
	})

	t.Run("it lives in nothing, is held on a node, and has ports the ingress serves", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "vm", d.Name)
		assert.Equal(t, "vms", d.Plural)
		assert.Equal(t, kind.OnNode, d.StateBy)
		assert.Empty(t, d.Parent)
		assert.Equal(t, kind.ParentRules{}, d.OnParent)
		assert.True(t, d.Endpoints)
	})

	t.Run("its actions are asked under the permissions VMs always had", func(t *testing.T) {
		t.Parallel()

		permissions := map[string]string{}
		for _, a := range d.Actions {
			if !a.Internal {
				admin, _ := d.Permissions(a.Permission)
				permissions[a.Name] = admin
			}
		}

		assert.Equal(t, map[string]string{
			"start":   "workload.vms.manage",
			"stop":    "workload.vms.manage",
			"restart": "workload.vms.manage",
			"restore": "workload.vms.manage",
			"update":  "workload.vms.update",
			"delete":  "workload.vms.delete",
			"state":   "workload.vms.show",
			"stats":   "workload.vms.show",
			"logs":    "workload.vms.logs",
			"attach":  "workload.vms.attach",
		}, permissions)

		for _, name := range []string{vmKind.ActionCreate, vmKind.ActionReconfigure} {
			a, _ := d.Action(name)
			assert.True(t, a.Internal, "%s is the workload's own to ask", name)
		}
	})

	t.Run("where each action runs and how it is asked", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			runs kind.Executor
			mode kind.Mode
		}{
			vmKind.ActionCreate:      {runs: kind.OnNode, mode: kind.ModeCommand},
			vmKind.ActionStart:       {runs: kind.OnNode, mode: kind.ModeCommand},
			vmKind.ActionStop:        {runs: kind.OnNode, mode: kind.ModeCommand},
			vmKind.ActionRestart:     {runs: kind.OnNode, mode: kind.ModeCommand},
			vmKind.ActionUpdate:      {runs: kind.OnControlPlane, mode: kind.ModeCommand},
			vmKind.ActionReconfigure: {runs: kind.OnNode, mode: kind.ModeCommand},
			vmKind.ActionRestore:     {runs: kind.OnNode, mode: kind.ModeCommand},
			vmKind.ActionDelete:      {runs: kind.OnNode, mode: kind.ModeCommand},
			vmKind.ActionState:       {runs: kind.OnNode, mode: kind.ModeQuery},
			vmKind.ActionLogs:        {runs: kind.OnNode, mode: kind.ModeQuery},
			vmKind.ActionStats:       {runs: kind.OnNode, mode: kind.ModeQuery},
			vmKind.ActionAttach:      {runs: kind.OnNode, mode: kind.ModeStream},
		} {
			a, found := d.Action(name)
			require.True(t, found, name)
			assert.Equal(t, tt.runs, a.Runs, name)
			assert.Equal(t, tt.mode, a.Mode, name)
		}

		assert.Len(t, d.Actions, 12)
	})

	t.Run("a start, a stop and a restart asked of one on its way somewhere wait for it to get there", func(t *testing.T) {
		t.Parallel()

		for _, a := range d.Actions {
			switch a.Name {
			case vmKind.ActionStart, vmKind.ActionStop, vmKind.ActionRestart:
				assert.True(t, a.Waits, a.Name)
			default:
				assert.False(t, a.Waits, a.Name)
			}
		}
	})

	t.Run("a restore gives it a snapshot's disk, and what lives in it is what that disk holds", func(t *testing.T) {
		t.Parallel()

		restore, _ := d.Action(vmKind.ActionRestore)
		assert.True(t, restore.Restores)
		assert.Empty(t, restore.Desires, "it is to be as it was asked to be before: running, or stopped")

		value, invalid, err := restore.Payload.Decode([]byte(`{"snapshot_uuid":"snapshot-uuid"}`))
		require.NoError(t, err)
		assert.Empty(t, invalid)
		assert.Equal(t, vmKind.RestorePayload{SnapshotUUID: "snapshot-uuid"}, value)

		_, invalid, err = restore.Payload.Decode([]byte(`{"snapshot_uuid":" "}`))
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"snapshot_uuid": "required_field"}, invalid)
	})

	t.Run("what each action may be asked in", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			action string
			state  kind.State
			allows bool
		}{
			"made once admitted":                          {action: vmKind.ActionCreate, state: vmKind.Created, allows: true},
			"and not over one that is made":               {action: vmKind.ActionCreate, state: vmKind.Stopped},
			"started when stopped":                        {action: vmKind.ActionStart, state: vmKind.Stopped, allows: true},
			"or failed":                                   {action: vmKind.ActionStart, state: vmKind.Failed, allows: true},
			"or never made":                               {action: vmKind.ActionStart, state: vmKind.Created, allows: true},
			"not when it runs":                            {action: vmKind.ActionStart, state: vmKind.Running},
			"nor while it starts":                         {action: vmKind.ActionStart, state: vmKind.Starting},
			"stopped when it runs":                        {action: vmKind.ActionStop, state: vmKind.Running, allows: true},
			"or failed, to be stopped":                    {action: vmKind.ActionStop, state: vmKind.Failed, allows: true},
			"not when stopped":                            {action: vmKind.ActionStop, state: vmKind.Stopped},
			"restarted when it runs":                      {action: vmKind.ActionRestart, state: vmKind.Running, allows: true},
			"and when it does not":                        {action: vmKind.ActionRestart, state: vmKind.Stopped, allows: true},
			"not while it restarts":                       {action: vmKind.ActionRestart, state: vmKind.Restarting},
			"updated whatever it is doing":                {action: vmKind.ActionUpdate, state: vmKind.Starting, allows: true},
			"but on its way out":                          {action: vmKind.ActionUpdate, state: vmKind.Deleting},
			"reconfigured at rest":                        {action: vmKind.ActionReconfigure, state: vmKind.Stopped, allows: true},
			"not on its way somewhere":                    {action: vmKind.ActionReconfigure, state: vmKind.Scheduled},
			"nor before it is made":                       {action: vmKind.ActionReconfigure, state: vmKind.Created},
			"restored at rest":                            {action: vmKind.ActionRestore, state: vmKind.Running, allows: true},
			"not on its way somewhere either":             {action: vmKind.ActionRestore, state: vmKind.Stopping},
			"deleted whatever it is doing":                {action: vmKind.ActionDelete, state: vmKind.Restoring, allows: true},
			"even while it is being deleted":              {action: vmKind.ActionDelete, state: vmKind.Deleting, allows: true},
			"asked what it is doing whatever it is":       {action: vmKind.ActionState, state: vmKind.Scheduled, allows: true},
			"its log read once its node holds it":         {action: vmKind.ActionLogs, state: vmKind.Stopped, allows: true},
			"and not before":                              {action: vmKind.ActionLogs, state: vmKind.Scheduled},
			"sampled while it runs":                       {action: vmKind.ActionStats, state: vmKind.Running, allows: true},
			"and opened in while it runs":                 {action: vmKind.ActionAttach, state: vmKind.Running, allows: true},
			"and only then":                               {action: vmKind.ActionAttach, state: vmKind.Stopped},
			"and nothing at all of one that is gone":      {action: vmKind.ActionDelete, state: vmKind.Deleted},
			"nor what a vm has not":                       {action: "explode", state: vmKind.Running},
			"nor in a state a vm cannot be in":            {action: vmKind.ActionStop, state: "degraded"},
			"a failed one is asked what it is doing too":  {action: vmKind.ActionState, state: vmKind.Failed, allows: true},
			"and so is one admitted and asked of nobody":  {action: vmKind.ActionState, state: vmKind.Created, allows: true},
			"one being restored is read as it is":         {action: vmKind.ActionLogs, state: vmKind.Restoring, allows: true},
			"one never made is not restored":              {action: vmKind.ActionRestore, state: vmKind.Created},
			"one that is gone is no longer read, either":  {action: vmKind.ActionLogs, state: vmKind.Deleted},
			"nor stopped while it is being deleted":       {action: vmKind.ActionStop, state: vmKind.Deleting},
			"nor started, which would take it back":       {action: vmKind.ActionStart, state: vmKind.Deleting},
			"nor reconfigured, which would take it back":  {action: vmKind.ActionReconfigure, state: vmKind.Deleting},
			"nor given a snapshot's disk on its way out":  {action: vmKind.ActionRestore, state: vmKind.Deleting},
			"nor opened in on its way out":                {action: vmKind.ActionAttach, state: vmKind.Deleting},
			"nor sampled on its way out":                  {action: vmKind.ActionStats, state: vmKind.Deleting},
			"nor restarted on its way out":                {action: vmKind.ActionRestart, state: vmKind.Deleting},
			"nor made on its way out":                     {action: vmKind.ActionCreate, state: vmKind.Deleting},
			"nor updated once gone":                       {action: vmKind.ActionUpdate, state: vmKind.Deleted},
			"but read on its way out, while there is one": {action: vmKind.ActionState, state: vmKind.Deleting, allows: true},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, tt.allows, d.Allows(tt.action, tt.state))
			})
		}
	})
}

func TestMachine(t *testing.T) {
	t.Parallel()

	m := vmKind.Machine()

	assert.Empty(t, m.Validate())
	assert.Equal(t, vmKind.Created, m.Initial, "a vm is admitted asked of nobody yet")
	assert.ElementsMatch(t, []kind.State{vmKind.Restarting, vmKind.Restoring}, m.Answered, "a vm runs before a restart or a restore as it does after it, so only their answers say they were carried out")

	for name, tt := range map[string]struct {
		from  kind.State
		on    kind.Trigger
		to    kind.State
		taken bool
	}{
		"made, it is scheduled":                         {from: vmKind.Created, on: kind.OnAction(vmKind.ActionCreate), to: vmKind.Scheduled, taken: true},
		"and runs once its node says so":                {from: vmKind.Scheduled, on: kind.OnObserved(vmKind.Running), to: vmKind.Running, taken: true},
		"and nothing its node holds of it ends it":      {from: vmKind.Scheduled, on: kind.OnObserved(kind.Missing), to: vmKind.Scheduled},
		"started, it starts":                            {from: vmKind.Stopped, on: kind.OnAction(vmKind.ActionStart), to: vmKind.Starting, taken: true},
		"and so does a failed one":                      {from: vmKind.Failed, on: kind.OnAction(vmKind.ActionStart), to: vmKind.Starting, taken: true},
		"a start ends running":                          {from: vmKind.Starting, on: kind.OnObserved(vmKind.Running), to: vmKind.Running, taken: true},
		"and is not undone by a report sent before":     {from: vmKind.Starting, on: kind.OnObserved(vmKind.Stopped), to: vmKind.Starting},
		"stopped, it stops":                             {from: vmKind.Running, on: kind.OnAction(vmKind.ActionStop), to: vmKind.Stopping, taken: true},
		"a stop ends stopped":                           {from: vmKind.Stopping, on: kind.OnObserved(vmKind.Stopped), to: vmKind.Stopped, taken: true},
		"and is not undone by a report sent before it":  {from: vmKind.Stopping, on: kind.OnObserved(vmKind.Running), to: vmKind.Stopping},
		"a failed one asked to stop stays failed":       {from: vmKind.Failed, on: kind.OnAction(vmKind.ActionStop), to: vmKind.Failed},
		"restarted, it restarts":                        {from: vmKind.Running, on: kind.OnAction(vmKind.ActionRestart), to: vmKind.Restarting, taken: true},
		"and one that does not run starts":              {from: vmKind.Stopped, on: kind.OnAction(vmKind.ActionRestart), to: vmKind.Starting, taken: true},
		"a restart ends running":                        {from: vmKind.Restarting, on: kind.OnObserved(vmKind.Running), to: vmKind.Running, taken: true},
		"reconfigured while it runs, it restarts":       {from: vmKind.Running, on: kind.OnAction(vmKind.ActionReconfigure), to: vmKind.Restarting, taken: true},
		"and stays where it is when it does not":        {from: vmKind.Stopped, on: kind.OnAction(vmKind.ActionReconfigure), to: vmKind.Stopped},
		"restored, it is restoring":                     {from: vmKind.Stopped, on: kind.OnAction(vmKind.ActionRestore), to: vmKind.Restoring, taken: true},
		"until it runs again":                           {from: vmKind.Restoring, on: kind.OnObserved(vmKind.Running), to: vmKind.Running, taken: true},
		"or is stopped again":                           {from: vmKind.Restoring, on: kind.OnObserved(vmKind.Stopped), to: vmKind.Stopped, taken: true},
		"at rest, it is what its node says":             {from: vmKind.Running, on: kind.OnObserved(vmKind.Stopped), to: vmKind.Stopped, taken: true},
		"a failed one comes back when its node says so": {from: vmKind.Failed, on: kind.OnObserved(vmKind.Running), to: vmKind.Running, taken: true},
		"one its node no longer holds is not running":   {from: vmKind.Running, on: kind.OnObserved(kind.Missing), to: vmKind.Stopped, taken: true},
		"and a stopped one stays stopped":               {from: vmKind.Stopped, on: kind.OnObserved(kind.Missing), to: vmKind.Stopped},
		"and a failed one stays failed, saying why":     {from: vmKind.Failed, on: kind.OnObserved(kind.Missing), to: vmKind.Failed},
		"a failure is believed from anywhere":           {from: vmKind.Restarting, on: kind.OnObserved(vmKind.Failed), to: vmKind.Failed, taken: true},
		"deleted from anywhere":                         {from: vmKind.Starting, on: kind.OnAction(vmKind.ActionDelete), to: vmKind.Deleting, taken: true},
		"and gone once its node holds none of it":       {from: vmKind.Deleting, on: kind.OnObserved(kind.Missing), to: vmKind.Deleted, taken: true},
		"but not while its node still holds it":         {from: vmKind.Deleting, on: kind.OnObserved(vmKind.Running), to: vmKind.Deleting},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, taken := m.Next(tt.from, tt.on)

			assert.Equal(t, tt.to, to)
			assert.Equal(t, tt.taken, taken)
		})
	}
}

func TestUpdatePayload_Validate(t *testing.T) {
	t.Parallel()

	name := func(s string) *string { return &s }
	lifetime := func(d time.Duration) *time.Duration { return &d }
	ports := func(p ...port.Port) *[]port.Port { return &p }

	for title, tt := range map[string]struct {
		payload vmKind.UpdatePayload
		want    domain.ValidationErrors
	}{
		"nothing changes nothing, and is valid": {},
		"a name, a lifetime, ports, a network and resources": {
			payload: vmKind.UpdatePayload{
				Name:      name("box"),
				Lifetime:  lifetime(time.Hour),
				Ports:     ports(80, 443),
				Network:   &vmKind.Network{Ingress: vm.AccessDeny},
				Resources: &vmKind.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30},
			},
		},
		"an empty name": {
			payload: vmKind.UpdatePayload{Name: name("  ")},
			want:    domain.ValidationErrors{"name": "required_field"},
		},
		"one too long to show": {
			payload: vmKind.UpdatePayload{Name: name(strings.Repeat("a", vmKind.MaxNameLength+1))},
			want:    domain.ValidationErrors{"name": "invalid_name"},
		},
		"a lifetime that ended before it began": {
			payload: vmKind.UpdatePayload{Lifetime: lifetime(-time.Second)},
			want:    domain.ValidationErrors{"lifetime_seconds": "invalid_lifetime"},
		},
		"a port there is not": {
			payload: vmKind.UpdatePayload{Ports: ports(0)},
			want:    domain.ValidationErrors{"ports": "invalid_port"},
		},
		"more ports than a vm may have": {
			payload: vmKind.UpdatePayload{Ports: ports(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17)},
			want:    domain.ValidationErrors{"ports": "too_many_ports"},
		},
		"a way of a network that is neither allowed nor denied": {
			payload: vmKind.UpdatePayload{Network: &vmKind.Network{Ingress: "open", Egress: "shut"}},
			want:    domain.ValidationErrors{"network.ingress": "invalid_access", "network.egress": "invalid_access"},
		},
	} {
		t.Run(title, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, nilIfEmpty(tt.payload.Validate()))
		})
	}

	assert.False(t, vmKind.UpdatePayload{Name: name("box"), Lifetime: lifetime(time.Hour)}.Changes(), "a name and a lifetime are the record's alone")
	assert.True(t, vmKind.UpdatePayload{Ports: ports()}.Changes(), "ports, even none, are its node's to apply")
	assert.True(t, vmKind.UpdatePayload{Network: &vmKind.Network{}}.Changes())
	assert.True(t, vmKind.UpdatePayload{Resources: &vmKind.Resources{}}.Changes())
}

func nilIfEmpty(invalid domain.ValidationErrors) domain.ValidationErrors {
	if len(invalid) == 0 {
		return nil
	}

	return invalid
}

func TestNormalized(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []port.Port{22, 80, 443}, vmKind.Normalized([]port.Port{443, 80, 22, 80}))
	assert.Equal(t, []port.Port{}, vmKind.Normalized(nil), "none, rather than nothing")
}

func TestConfig(t *testing.T) {
	t.Parallel()

	spec := vmKind.Spec{
		Flavor:    vmKind.FlavorMachine,
		Image:     "ubuntu:24.04",
		Resources: vmKind.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30},
		Ports:     []port.Port{80},
		Network:   vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
	}

	config := spec.Config()
	assert.True(t, config.Equal(spec.Config()))

	spec.Ports[0] = 8080
	assert.Equal(t, []port.Port{80}, config.Ports, "a config shares nothing with its spec")
	assert.False(t, config.Equal(spec.Config()))

	other := config
	other.Network.Egress = vm.AccessAllow
	assert.False(t, config.Equal(other))

	other = config
	other.Resources.Disk++
	assert.False(t, config.Equal(other))
}

func TestStatus(t *testing.T) {
	t.Parallel()

	t.Run("a vm sampled is shown with its sample, its cpu held to 0 to 100", func(t *testing.T) {
		t.Parallel()

		sampled := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
		stats := vmKind.StatsOf(vm.Stats{CPUPercent: 130, MemoryUsed: 1, MemoryLimit: 2, DiskUsed: 3, DiskTotal: 4, NetworkRx: 5, NetworkTx: 6, SampledAt: sampled})

		assert.Equal(t, 100.0, stats.CPUPercent)
		assert.Equal(t, vm.Stats{CPUPercent: 100, MemoryUsed: 1, MemoryLimit: 2, DiskUsed: 3, DiskTotal: 4, NetworkRx: 5, NetworkTx: 6, SampledAt: sampled}, stats.VM())

		assert.Equal(t, 0.0, vmKind.StatsOf(vm.Stats{CPUPercent: -1}).CPUPercent)
		assert.Equal(t, vm.Stats{}, (*vmKind.Stats)(nil).VM(), "no sample is nothing")
	})

	t.Run("what a node says of it beside its state travels whole, and nothing of it is left out", func(t *testing.T) {
		t.Parallel()

		written, err := json.Marshal(vmKind.Status{Status: kind.Status{State: vmKind.Stopped}})
		require.NoError(t, err)

		// a stopped vm's node says it uses nothing and publishes nothing, which
		// is written down over what it said while it ran.
		assert.JSONEq(t, `{"state":"stopped","stats":null,"endpoints":null}`, string(written))
	})
}

func TestUp(t *testing.T) {
	t.Parallel()

	docker := func(state kind.State, node string) vmKind.VM {
		return vmKind.VM{Metadata: kind.Metadata{Node: node}, Spec: vmKind.Spec{Flavor: vmKind.FlavorDocker}, Status: vmKind.Status{Status: kind.Status{State: state}}}
	}

	assert.True(t, vmKind.Up(docker(vmKind.Running, "node-1")))
	assert.True(t, vmKind.Up(docker(vmKind.Starting, "node-1")), "on its way up")
	assert.False(t, vmKind.Up(docker(vmKind.Stopped, "node-1")))
	assert.False(t, vmKind.Up(docker(vmKind.Running, "")), "on no node")

	machine := docker(vmKind.Running, "node-1")
	machine.Spec.Flavor = vmKind.FlavorMachine
	assert.False(t, vmKind.Up(machine), "a machine has no dockerd")
	assert.False(t, vmKind.DockerVM(machine))
}

func TestStateOf(t *testing.T) {
	t.Parallel()

	for state, want := range map[kind.State]vm.State{
		vmKind.Created:    vm.Created,
		vmKind.Scheduled:  vm.Scheduled,
		vmKind.Starting:   vm.Starting,
		vmKind.Running:    vm.Running,
		vmKind.Stopping:   vm.Stopping,
		vmKind.Stopped:    vm.Stopped,
		vmKind.Restarting: vm.Restarting,
		vmKind.Restoring:  vm.Restoring,
		vmKind.Failed:     vm.Failed,
		vmKind.Deleting:   vm.Deleting,
		vmKind.Deleted:    vm.Deleting,
		"degraded":        0,
		"":                0,
	} {
		assert.Equal(t, want, vmKind.StateOf(state), "%q", state)
	}
}

func TestEntity(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	v := vmKind.VM{
		Kind: vmKind.Name,
		Metadata: kind.Metadata{
			UUID:      "vm-uuid",
			Name:      "box",
			Slug:      "box-abcde",
			OwnerUUID: "owner-uuid",
			Labels:    map[string]string{vmKind.LabelFlavor: "machine"},
			Node:      "workload-orchestrator-01",
			Lifetime:  time.Hour,
			ExpiresAt: created.Add(time.Hour),
			CreatedAt: created,
			UpdatedAt: created.Add(time.Minute),
		},
		Spec: vmKind.Spec{
			Flavor:         vmKind.FlavorMachine,
			Image:          "ubuntu:24.04",
			Resources:      vmKind.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30},
			Ports:          []port.Port{80, 8080},
			Network:        vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
			PersistentDisk: true,
		},
		Status: vmKind.Status{
			Status:    kind.Status{State: vmKind.Running, Expected: vmKind.Running, Reason: "", ObservedAt: created.Add(2 * time.Minute)},
			Stats:     &vmKind.Stats{CPUPercent: 75, MemoryUsed: 256 << 20, SampledAt: created.Add(2 * time.Minute)},
			StartedAt: created.Add(time.Minute),
		},
	}

	assert.Equal(t, vm.VM{
		UUID:            "vm-uuid",
		Name:            "box",
		Slug:            "box-abcde",
		OwnerUUID:       "owner-uuid",
		Kind:            vm.KindMachine,
		Image:           "ubuntu:24.04",
		Resources:       vm.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30},
		Ports:           []port.Port{80, 8080},
		Network:         vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		PersistentDisk:  true,
		Lifetime:        time.Hour,
		ExpiresAt:       created.Add(time.Hour),
		CurrentState:    vm.Running,
		ExpectedState:   vm.Running,
		NodeName:        "workload-orchestrator-01",
		Stats:           vm.Stats{CPUPercent: 75, MemoryUsed: 256 << 20, SampledAt: created.Add(2 * time.Minute)},
		LastHeartbeatAt: created.Add(2 * time.Minute),
		CreatedAt:       created,
		StartedAt:       created.Add(time.Minute),
		UpdatedAt:       created.Add(time.Minute),
	}, vmKind.Entity(v))

	v.Spec.Ports = nil
	v.Status.Expected = vmKind.Deleted
	v.Metadata.Labels = map[string]string{vmKind.LabelManagedBy: vmKind.ManagedByCodeRunner}

	entity := vmKind.Entity(v)
	assert.Equal(t, []port.Port{}, entity.Ports, "none, rather than nothing")
	assert.Equal(t, vm.Deleting, entity.ExpectedState, "one expected deleted is on its way out")
	assert.Equal(t, vm.ManagedByCodeRunner, entity.ManagedBy)
}
