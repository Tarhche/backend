package container_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	d := container.Descriptor()

	t.Run("it keeps every rule a kind keeps", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, kind.Check(d))
	})

	t.Run("it lives in a vm, goes with it, and is what a restored one holds", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "container", d.Name)
		assert.Equal(t, "containers", d.Plural)
		assert.Equal(t, kind.OnNode, d.StateBy)
		assert.Equal(t, "vm", d.Parent)
		assert.Equal(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}, d.OnParent)
	})

	t.Run("its actions are asked under the permissions containers always had", func(t *testing.T) {
		t.Parallel()

		permissions := map[string]string{}
		for _, a := range d.Actions {
			if !a.Internal {
				admin, _ := d.Permissions(a.Permission)
				permissions[a.Name] = admin
			}
		}

		assert.Equal(t, map[string]string{
			"start":      "workload.containers.manage",
			"stop":       "workload.containers.manage",
			"restart":    "workload.containers.manage",
			"connect":    "workload.containers.manage",
			"disconnect": "workload.containers.manage",
			"delete":     "workload.containers.delete",
			"state":      "workload.containers.show",
			"logs":       "workload.containers.logs",
			"stats":      "workload.containers.show",
		}, permissions)

		create, _ := d.Action(container.ActionCreate)
		assert.True(t, create.Internal, "making one is the workload's own to ask")
	})

	t.Run("what each action may be asked in", func(t *testing.T) {
		t.Parallel()

		for name, tt := range map[string]struct {
			action string
			state  kind.State
			allows bool
		}{
			"made where it is not yet":               {action: container.ActionCreate, state: container.Pending, allows: true},
			"or not any more":                        {action: container.ActionCreate, state: container.Missing, allows: true},
			"not over one that is":                   {action: container.ActionCreate, state: container.Running},
			"started when stopped":                   {action: container.ActionStart, state: container.Stopped, allows: true},
			"or completed":                           {action: container.ActionStart, state: container.Completed, allows: true},
			"not when it runs":                       {action: container.ActionStart, state: container.Running},
			"stopped when it runs":                   {action: container.ActionStop, state: container.Running, allows: true},
			"not when it is stopped":                 {action: container.ActionStop, state: container.Stopped},
			"restarted when it runs":                 {action: container.ActionRestart, state: container.Running, allows: true},
			"put on a network when it is there":      {action: container.ActionConnect, state: container.Stopped, allows: true},
			"not when its vm is not running":         {action: container.ActionConnect, state: container.Waiting},
			"taken off one when it is there":         {action: container.ActionDisconnect, state: container.Running, allows: true},
			"deleted in any state":                   {action: container.ActionDelete, state: container.Creating, allows: true},
			"its log read when it ran":               {action: container.ActionLogs, state: container.Completed, allows: true},
			"not before it was made":                 {action: container.ActionLogs, state: container.Pending},
			"what it uses sampled only when it runs": {action: container.ActionStats, state: container.Stopped},
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

	m := container.Machine()

	assert.ElementsMatch(t, []kind.State{container.Starting, container.Restarting}, m.Answered, "a look taken before a start or a restart reached its node says nothing of what it came to, so only their answers say")

	for name, tt := range map[string]struct {
		from kind.State
		on   kind.Trigger
		to   kind.State
	}{
		"made, it runs":                                     {from: container.Creating, on: kind.OnObserved(container.Running), to: container.Running},
		"or completes, a one-off":                           {from: container.Creating, on: kind.OnObserved(container.Completed), to: container.Completed},
		"not missing while it is made":                      {from: container.Creating, on: kind.OnObserved(container.Missing), to: container.Creating},
		"stopped, it is stopped":                            {from: container.Stopping, on: kind.OnObserved(container.Stopped), to: container.Stopped},
		"and a heartbeat sent before the stop says nothing": {from: container.Stopping, on: kind.OnObserved(container.Running), to: container.Stopping},
		"one its vm has none of is missing":                 {from: container.Running, on: kind.OnObserved(container.Missing), to: container.Missing},
		"and so is a completed one":                         {from: container.Completed, on: kind.OnObserved(container.Missing), to: container.Missing},
		"one in a vm that is not running waits":             {from: container.Running, on: kind.OnObserved(container.Waiting), to: container.Waiting},
		"and so does one not made yet":                      {from: container.Pending, on: kind.OnObserved(container.Waiting), to: container.Waiting},
		"once its vm runs, one it has none of is missing":   {from: container.Waiting, on: kind.OnObserved(container.Missing), to: container.Missing},
		"and one it has is what it is":                      {from: container.Waiting, on: kind.OnObserved(container.Running), to: container.Running},
		"at rest it is what its node says":                  {from: container.Running, on: kind.OnObserved(container.Completed), to: container.Completed},
		"removed, it is gone once its vm has none of it":    {from: container.Removing, on: kind.OnObserved(container.Missing), to: container.Deleted},
		"a missing one is made again":                       {from: container.Missing, on: kind.OnAction(container.ActionCreate), to: container.Creating},
		"or started, which makes it":                        {from: container.Missing, on: kind.OnAction(container.ActionStart), to: container.Starting},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, _ := m.Next(tt.from, tt.on)
			assert.Equal(t, tt.to, to)
		})
	}
}

func TestStateOf(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		docker string
		policy string
		want   kind.State
	}{
		"up is running":                                          {docker: "running", want: container.Running},
		"and so is one docker is restarting":                     {docker: "restarting", policy: container.RestartAlways, want: container.Running},
		"and one paused":                                         {docker: "paused", want: container.Running},
		"one that exited and is not started again completed":     {docker: "exited", policy: container.RestartNo, want: container.Completed},
		"and so did one with no policy at all":                   {docker: "exited", want: container.Completed},
		"or on-failure, which docker gave up on or never had to": {docker: "exited", policy: container.RestartOnFailure, want: container.Completed},
		"one docker starts again by itself is stopped":           {docker: "exited", policy: container.RestartAlways, want: container.Stopped},
		"and so is one unless-stopped":                           {docker: "exited", policy: container.RestartUnlessStopped, want: container.Stopped},
		"one never started is stopped, not completed":            {docker: "created", policy: container.RestartNo, want: container.Stopped},
		"and so is a dead one":                                   {docker: "dead", want: container.Stopped},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, container.StateOf(tt.docker, tt.policy))
		})
	}
}

func TestSpec(t *testing.T) {
	t.Parallel()

	asked := docker.ContainerSpec{
		Name:          "web",
		Image:         "nginx:1.27",
		Command:       []string{"nginx", "-g", "daemon off;"},
		Env:           []string{"A=1"},
		Ports:         []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
		Mounts:        []docker.Mount{{Type: "volume", Source: "data", Target: "/data"}},
		Networks:      []string{"backend"},
		RestartPolicy: container.RestartAlways,
		CPUs:          0.5,
		Memory:        64 << 20,
	}

	spec := container.SpecOf(asked)

	made := spec.Docker("c-uuid", map[string]string{container.LabelSpec: "{}"})

	assert.Equal(t, map[string]string{"workload.managed": "true", "workload.container": "c-uuid", container.LabelSpec: "{}"}, made.Labels, "labelled as the platform's, as the resource it is")

	made.Labels = nil
	assert.Equal(t, asked, made, "what docker is asked for is what was asked")

	encoded, err := json.Marshal(spec)
	require.NoError(t, err)

	var read container.Spec
	require.NoError(t, json.Unmarshal(encoded, &read))
	assert.Equal(t, spec, read)
}

func TestDocker(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	seen := docker.Container{
		ID:        "c0ffee",
		Name:      "web",
		Image:     "nginx:1.27",
		State:     "running",
		Status:    "Up 3 minutes",
		Command:   "nginx",
		Ports:     []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
		Networks:  []string{"bridge"},
		Labels:    map[string]string{docker.LabelComposeProject: "shop-abcde", docker.LabelComposeService: "web"},
		Stack:     "shop-abcde",
		Service:   "web",
		CreatedAt: at,
	}

	assert.Equal(t, seen, container.DockerOf(seen).Container(), "what docker said is read back as it said it")

	var none *container.Docker
	assert.Equal(t, docker.Container{}, none.Container(), "and nothing is nothing")

	empty := container.DockerOf(docker.Container{ID: "c0ffee"})
	assert.Equal(t, []container.PortBinding{}, empty.Ports, "a list is a list even when it is empty")
	assert.Equal(t, []string{}, empty.Networks)
	assert.Equal(t, []container.Mount{}, empty.Mounts)
}

func TestStatus(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	status := container.Status{
		Status: kind.Status{State: container.Stopped, Expected: container.Running, Reason: "it stopped", Since: at, ObservedAt: at},
		Docker: &container.Docker{
			ID:            "c0ffee",
			Name:          "web",
			Image:         "nginx:1.27",
			State:         "exited",
			Status:        "Exited (1) 5 minutes ago",
			Command:       "nginx",
			Ports:         []container.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
			Networks:      []string{"bridge"},
			Mounts:        []container.Mount{{Type: "volume", Source: "data", Target: "/data"}},
			Labels:        map[string]string{"workload.managed": "true"},
			RestartPolicy: container.RestartAlways,
			CreatedAt:     at,
		},
		Failure: &noderequest.Error{Code: noderequest.CodeInvalid, Message: "it would not start"},
	}

	written, err := json.Marshal(status)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"state": "stopped",
		"expected": "running",
		"reason": "it stopped",
		"since": "2026-10-06T12:00:00Z",
		"observed_at": "2026-10-06T12:00:00Z",
		"id": "c0ffee",
		"name": "web",
		"image": "nginx:1.27",
		"docker_state": "exited",
		"docker_status": "Exited (1) 5 minutes ago",
		"command": "nginx",
		"ports": [{"container_port": 80, "host_port": 8080, "protocol": "tcp"}],
		"networks": ["bridge"],
		"mounts": [{"type": "volume", "source": "data", "target": "/data"}],
		"labels": {"workload.managed": "true"},
		"restart_policy": "always",
		"created_at": "2026-10-06T12:00:00Z",
		"failure": {"code": "invalid", "message": "it would not start"}
	}`, string(written), "what docker said of it is beside its state, docker's own state under a name of its own")

	var read container.Status
	require.NoError(t, json.Unmarshal(written, &read))
	assert.Equal(t, status, read)
	assert.Equal(t, container.Stopped, read.Status.State, "its state, in the kind's words, survives")
	assert.Equal(t, "exited", read.Docker.State, "and docker's beside it")

	raw, err := kind.Encode(container.Container{Kind: container.Name, Status: status})
	require.NoError(t, err)

	decoded, err := kind.Decode[container.Spec, container.Status](raw)
	require.NoError(t, err)
	assert.Equal(t, status, decoded.Status, "as the framework writes and reads it")

	pending, err := json.Marshal(container.Status{Status: kind.Status{State: container.Pending, Expected: container.Running}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"state": "pending", "expected": "running"}`, string(pending), "one docker has not made yet has nothing of docker's")

	var unseen container.Status
	require.NoError(t, json.Unmarshal(pending, &unseen))
	assert.Nil(t, unseen.Docker)
	assert.Equal(t, container.Pending, unseen.Status.State)

	t.Run("taken onto its record, what docker says takes the place of all it said before", func(t *testing.T) {
		t.Parallel()

		again, err := json.Marshal(container.Status{Docker: &container.Docker{ID: "c0ffee", Name: "web", Image: "nginx:1.27", State: "running"}})
		require.NoError(t, err)

		merged, err := resource.Merge(written, again)
		require.NoError(t, err)

		var taken container.Status
		require.NoError(t, json.Unmarshal(merged, &taken))
		assert.Equal(t, &container.Docker{ID: "c0ffee", Name: "web", Image: "nginx:1.27", State: "running"}, taken.Docker, "what it left empty among it")

		missing, err := resource.Merge(written, json.RawMessage(`{"state": "missing"}`))
		require.NoError(t, err)

		var kept container.Status
		require.NoError(t, json.Unmarshal(missing, &kept))
		assert.Equal(t, status.Docker, kept.Docker, "and a report that says nothing of docker leaves what it said as it was")
	})
}

func TestPayloads(t *testing.T) {
	t.Parallel()

	assert.Equal(t, domain.ValidationErrors{"network": "required_field"}, (&container.ConnectPayload{}).Validate())
	assert.Empty(t, (&container.ConnectPayload{Network: "backend"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"network": "required_field"}, (&container.DisconnectPayload{Network: " "}).Validate())
	assert.Empty(t, (&container.DeletePayload{}).Validate())
	assert.Empty(t, (&container.LogsPayload{}).Validate())
}

func TestVMOf(t *testing.T) {
	t.Parallel()

	in := container.Container{Metadata: kind.Metadata{Owners: []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}}}
	assert.Equal(t, "vm-uuid", container.VMOf(in))

	asked := container.Container{Spec: container.Spec{}}
	asked.Spec.VM.UUID = "named"
	assert.Equal(t, "named", container.VMOf(asked), "one not kept yet goes where it asks")
}

func TestSpecLabel(t *testing.T) {
	t.Parallel()

	spec := container.Spec{Name: "web", Image: "nginx:1.27", Env: []string{"A=1"}, RestartPolicy: container.RestartOnFailure}
	spec.VM.UUID = "vm-uuid"

	written, err := container.SpecLabel(spec)
	require.NoError(t, err)
	assert.NotContains(t, written, "vm-uuid", "the vm it is in is not written beside it")

	read, labelled := container.SpecFromLabels(map[string]string{container.LabelSpec: written})
	require.True(t, labelled)

	spec.VM.UUID = ""
	assert.Equal(t, spec, read)

	policy, known := container.PolicyOf(map[string]string{container.LabelSpec: written})
	assert.True(t, known)
	assert.Equal(t, container.RestartOnFailure, policy)

	none, err := container.SpecLabel(container.Spec{Image: "nginx"})
	require.NoError(t, err)

	policy, known = container.PolicyOf(map[string]string{container.LabelSpec: none})
	assert.True(t, known)
	assert.Equal(t, container.RestartNo, policy, "none is docker's no")

	_, known = container.PolicyOf(nil)
	assert.False(t, known, "and one the platform did not make is not known")

	_, labelled = container.SpecFromLabels(map[string]string{container.LabelSpec: "{"})
	assert.False(t, labelled)
}
