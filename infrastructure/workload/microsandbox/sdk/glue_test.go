//go:build microsandbox

package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// These need the SDK to compile but no VM to run.

func TestItemOf(t *testing.T) {
	t.Parallel()

	errno := 13

	testcases := map[string]struct {
		event msb.ExecEvent
		want  item
	}{
		"started": {
			event: msb.ExecEvent{Kind: msb.ExecEventStarted, PID: 206},
			want:  item{event: runs.Event{Kind: runs.EventStarted}},
		},
		"stdout": {
			event: msb.ExecEvent{Kind: msb.ExecEventStdout, Data: []byte("out")},
			want:  item{event: runs.Event{Kind: runs.EventStdout, Data: []byte("out")}},
		},
		"stderr": {
			event: msb.ExecEvent{Kind: msb.ExecEventStderr, Data: []byte("err")},
			want:  item{event: runs.Event{Kind: runs.EventStderr, Data: []byte("err")}},
		},
		"an exit": {
			event: msb.ExecEvent{Kind: msb.ExecEventExited, ExitCode: 7},
			want:  item{event: runs.Event{Kind: runs.EventExited, ExitCode: 7}},
		},
		"a signal's -1": {
			event: msb.ExecEvent{Kind: msb.ExecEventExited, ExitCode: -1},
			want:  item{event: runs.Event{Kind: runs.EventExited, ExitCode: -1}},
		},
		"a failure, as the agent reports it": {
			event: msb.ExecEvent{Kind: msb.ExecEventFailed, Failure: &msb.ExecFailure{
				Kind: "permission_denied", Errno: &errno, ErrnoName: "EACCES",
				Message: `spawn "/tmp": Permission denied (os error 13)`,
			}},
			want: item{event: runs.Event{Kind: runs.EventFailed, Errno: "EACCES", Message: `spawn "/tmp": Permission denied (os error 13)`}},
		},
		"a failure with only a kind": {
			event: msb.ExecEvent{Kind: msb.ExecEventFailed, Failure: &msb.ExecFailure{Kind: "not_found", Message: "spawn"}},
			want:  item{event: runs.Event{Kind: runs.EventFailed, Errno: "ENOENT", Message: "spawn"}},
		},
		"a failure with no detail": {
			event: msb.ExecEvent{Kind: msb.ExecEventFailed},
			want:  item{event: runs.Event{Kind: runs.EventFailed}},
		},
		"a failed write to stdin": {
			event: msb.ExecEvent{Kind: msb.ExecEventStdinError, Failure: &msb.ExecFailure{Message: "broken pipe"}},
			want:  item{skip: true},
		},
		"the end": {
			event: msb.ExecEvent{Kind: msb.ExecEventDone},
			want:  item{done: true},
		},
		"a kind of a later version": {
			event: msb.ExecEvent{Kind: msb.ExecEventKind(99)},
			want:  item{skip: true},
		},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			event := tt.event
			assert.Equal(t, tt.want, itemOf(&event))
		})
	}
}

// applied is what options make of a sandbox's configuration.
func applied(options []msb.SandboxOption) msb.SandboxConfig {
	var config msb.SandboxConfig
	for _, option := range options {
		option(&config)
	}

	return config
}

func TestCreateOptions(t *testing.T) {
	t.Parallel()

	t.Run("a public sandbox", func(t *testing.T) {
		t.Parallel()

		spec := runs.SandboxSpec{
			Name:              "wk-1",
			Image:             "nginx:alpine",
			CPUs:              1.5,
			Memory:            64<<20 + 1,
			Disk:              64 << 20,
			Env:               map[string]string{"A": "1"},
			Labels:            map[string]string{"workload.run": "1"},
			Workdir:           "/srv",
			Network:           "public",
			Ports:             []runs.PortBinding{{Bind: "10.89.0.10", HostPort: 20000, GuestPort: 80}},
			Nameservers:       []string{"1.1.1.1", "9.9.9.9"},
			MaxTCPConnections: 256,
		}

		options, err := createOptions(spec)
		require.NoError(t, err)
		config := applied(options)

		assert.Equal(t, "nginx:alpine", config.Image)
		assert.True(t, config.Detached, "detached, so the VM outlives this process")
		assert.Equal(t, msb.PullPolicyIfMissing, config.PullPolicy)
		assert.Equal(t, uint8(2), config.CPUs)
		assert.Equal(t, uint32(65), config.MemoryMiB)
		require.NotNil(t, config.RootDisk)
		assert.Equal(t, msb.RootDiskKindManaged, config.RootDisk.Kind())
		assert.Equal(t, uint32(142), config.RootDisk.SizeMiB)
		assert.Equal(t, spec.Env, config.Env)
		assert.Equal(t, spec.Labels, config.Labels)
		assert.Equal(t, "/srv", config.Workdir)
		assert.Equal(t, msb.SecurityProfileDefault, config.SecurityProfile)
		assert.Equal(t, msb.DeploymentProfileSingleTenant, config.DeploymentProfile, "multi-tenant drops published ports")
		assert.Equal(t, []msb.PortBinding{{Bind: "10.89.0.10", HostPort: 20000, GuestPort: 80, Protocol: msb.PortProtocolTCP}}, config.PortBindings)
		assert.Empty(t, config.Ports, "nothing is published on 127.0.0.1")

		require.NotNil(t, config.Network)
		assert.Equal(t, msb.NetworkPolicy.FromProfiles(msb.NetworkProfilePublic).Rules, config.Network.Rules)
		assert.Equal(t, msb.PolicyActionDeny, config.Network.DefaultEgress)
		require.NotNil(t, config.Network.DNS)
		assert.Equal(t, []string{"1.1.1.1", "9.9.9.9"}, config.Network.DNS.Nameservers)
		require.NotNil(t, config.Network.MaxTCPConnections)
		assert.Equal(t, uint(256), *config.Network.MaxTCPConnections)

		spec.Env["A"] = "changed"
		assert.Equal(t, "1", config.Env["A"], "the spec's maps are copied")
	})

	t.Run("an isolated sandbox has no egress at all and no DNS", func(t *testing.T) {
		t.Parallel()

		options, err := createOptions(runs.SandboxSpec{Name: "wk-2", Image: "busybox", Network: "isolated", Nameservers: []string{"1.1.1.1"}})
		require.NoError(t, err)
		network := applied(options).Network

		require.NotNil(t, network)
		assert.Empty(t, network.Rules)
		assert.Equal(t, msb.PolicyActionDeny, network.DefaultEgress)
		assert.Equal(t, msb.PolicyActionAllow, network.DefaultIngress, "published ports still let others in")
		assert.Nil(t, network.DNS)
		assert.Nil(t, network.MaxTCPConnections)
	})

	t.Run("what is not given is microsandbox's own", func(t *testing.T) {
		t.Parallel()

		options, err := createOptions(runs.SandboxSpec{Name: "wk-3", Image: "busybox", Network: "public"})
		require.NoError(t, err)
		config := applied(options)

		assert.Equal(t, uint8(1), config.CPUs)
		assert.Zero(t, config.MemoryMiB)
		assert.Nil(t, config.RootDisk)
		assert.Nil(t, config.Env)
		assert.Nil(t, config.Labels)
		assert.Empty(t, config.Workdir)
		assert.Empty(t, config.PortBindings)
		assert.Nil(t, config.Network.DNS)
	})

	for name, spec := range map[string]runs.SandboxSpec{
		"no name":           {Image: "busybox", Network: "public"},
		"no image":          {Name: "wk", Network: "public"},
		"no network":        {Name: "wk", Image: "busybox"},
		"a network of none": {Name: "wk", Image: "busybox", Network: "none"},
		"too many CPUs":     {Name: "wk", Image: "busybox", Network: "public", CPUs: 1000},
	} {
		t.Run(name+" is refused", func(t *testing.T) {
			t.Parallel()

			_, err := createOptions(spec)
			assert.Error(t, err)
		})
	}
}

func TestExecOptions(t *testing.T) {
	t.Parallel()

	t.Run("a terminal with stdin, an environment and a directory", func(t *testing.T) {
		t.Parallel()

		options, err := execOptions(runs.Command{
			Argv: []string{"sh"}, Env: []string{"WORKLOAD_TERMINAL_SESSION=1"}, Workdir: "/root",
			TTY: true, Stdin: true, Rows: 40, Cols: 100,
		})
		require.NoError(t, err)

		var config msb.ExecConfig
		for _, option := range options {
			option(&config)
		}

		assert.Equal(t, msb.ExecConfig{
			Cwd: "/root", StdinPipe: true, TTY: true,
			Env: map[string]string{"WORKLOAD_TERMINAL_SESSION": "1"},
		}, config)
	})

	t.Run("a plain command gets a stdin to be closed, so that it reads an empty one", func(t *testing.T) {
		t.Parallel()

		options, err := execOptions(runs.Command{Argv: []string{"cat"}})
		require.NoError(t, err)

		var config msb.ExecConfig
		for _, option := range options {
			option(&config)
		}

		assert.Equal(t, msb.ExecConfig{StdinPipe: true}, config)
		assert.True(t, stdinPipe(runs.Command{}))
	})

	t.Run("a terminal without stdin stays silent, as docker's", func(t *testing.T) {
		t.Parallel()

		options, err := execOptions(runs.Command{Argv: []string{"top"}, TTY: true})
		require.NoError(t, err)

		var config msb.ExecConfig
		for _, option := range options {
			option(&config)
		}

		assert.Equal(t, msb.ExecConfig{TTY: true}, config)
	})

	t.Run("an environment entry that is not KEY=VALUE is refused", func(t *testing.T) {
		t.Parallel()

		_, err := execOptions(runs.Command{Argv: []string{"true"}, Env: []string{"KEY"}})
		assert.Error(t, err)
	})
}

func TestMetricsOf(t *testing.T) {
	t.Parallel()

	resident := uint64(147 << 20)
	assert.Equal(t, runs.Metrics{
		CPUPercent:  2.5,
		MemoryUsage: 150 << 20,
		MemoryLimit: 256 << 20,
		NetRx:       1, NetTx: 2, DiskRead: 3, DiskWrite: 4,
	}, metricsOf(&msb.Metrics{
		CPUPercent:              2.5,
		MemoryBytes:             150 << 20,
		MemoryHostResidentBytes: &resident,
		MemoryLimitBytes:        256 << 20,
		NetRxBytes:              1, NetTxBytes: 2, DiskReadBytes: 3, DiskWriteBytes: 4,
	}), "usage is what the guest has in use, not what the host handed it")
}

func TestImageConfigOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, runs.ImageConfig{
		Entrypoint: []string{"/docker-entrypoint.sh"},
		Cmd:        []string{"nginx", "-g", "daemon off;"},
		Env:        []string{"PATH=/usr/bin"},
		WorkingDir: "/srv",
		User:       "101",
		StopSignal: "SIGQUIT",
	}, imageConfigOf(&msb.ImageDetail{Config: &msb.ImageConfig{
		Entrypoint: []string{"/docker-entrypoint.sh"},
		Cmd:        []string{"nginx", "-g", "daemon off;"},
		Env:        []string{"PATH=/usr/bin"},
		WorkingDir: "/srv",
		User:       "101",
		StopSignal: "SIGQUIT",
	}}))

	assert.Equal(t, runs.ImageConfig{}, imageConfigOf(&msb.ImageDetail{}))
}

// running reads statuses as strings, so that it needs no SDK; these are the
// SDK's.
func TestRunningKnowsTheSDKsStatuses(t *testing.T) {
	t.Parallel()

	for _, status := range []msb.SandboxStatus{msb.SandboxStatusStarting, msb.SandboxStatusRunning, msb.SandboxStatusDraining, msb.SandboxStatusPaused} {
		assert.True(t, running(string(status)), status)
	}

	for _, status := range []msb.SandboxStatus{msb.SandboxStatusCreated, msb.SandboxStatusStopped, msb.SandboxStatusCrashed} {
		assert.False(t, running(string(status)), status)
	}
}
