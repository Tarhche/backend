package runs

import (
	"math"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

func validSpec() api.RunSpec {
	return api.RunSpec{
		Node:    "orchestrator-1",
		Name:    "task-1",
		Image:   "busybox",
		Memory:  128 << 20,
		CPU:     0.5,
		Network: api.NetworkIsolated,
	}
}

func TestValidateSpec(t *testing.T) {
	t.Parallel()

	t.Run("a spec that holds", func(t *testing.T) {
		t.Parallel()

		spec := validSpec()
		spec.Ports = []uint16{80, 443}
		spec.Environment = []string{"A=1", "B=", "C=x=y"}
		spec.WorkingDir = "/app"
		spec.RestartPolicy = "on-failure:3"

		assert.NoError(t, validateSpec(spec))
	})

	tests := []struct {
		name    string
		mutate  func(*api.RunSpec)
		code    string
		message string
	}{
		{"no node", func(s *api.RunSpec) { s.Node = "" }, api.CodeInvalid, "node is required"},
		{"no name", func(s *api.RunSpec) { s.Name = "" }, api.CodeInvalid, "name is required"},
		{"a name longer than 128 bytes", func(s *api.RunSpec) { s.Name = strings.Repeat("n", 129) }, api.CodeInvalid, "name is longer than 128 bytes"},
		{"no image", func(s *api.RunSpec) { s.Image = " " }, api.CodeInvalid, "image is required"},
		{"no memory", func(s *api.RunSpec) { s.Memory = 0 }, api.CodeInvalid, "memory has to be more than 0 bytes"},
		{"negative cpu", func(s *api.RunSpec) { s.CPU = -1 }, api.CodeInvalid, "cpu has to be a number of cores"},
		{"cpu that is no number", func(s *api.RunSpec) { s.CPU = math.NaN() }, api.CodeInvalid, "cpu has to be a number of cores"},
		{"more than 64 ports", func(s *api.RunSpec) {
			for port := range uint16(65) {
				s.Ports = append(s.Ports, port+1)
			}
		}, api.CodeInvalid, "at most 64 ports may be published"},
		{"port 0", func(s *api.RunSpec) { s.Ports = []uint16{0} }, api.CodeInvalid, "port 0 cannot be published"},
		{"a port twice", func(s *api.RunSpec) { s.Ports = []uint16{80, 80} }, api.CodeInvalid, "port 80 is published twice"},
		{"an entry that is not KEY=VALUE", func(s *api.RunSpec) { s.Environment = []string{"NOVALUE"} }, api.CodeInvalid, `environment entry "NOVALUE" is not KEY=VALUE`},
		{"an entry with no key", func(s *api.RunSpec) { s.Environment = []string{"=1"} }, api.CodeInvalid, "is not KEY=VALUE"},
		{"an entry with a tab", func(s *api.RunSpec) { s.Environment = []string{"A=1\t2"} }, api.CodeInvalid, "environment entry A holds a tab"},
		{"an entry with a NUL", func(s *api.RunSpec) { s.Environment = []string{"A=1\x002"} }, api.CodeInvalid, "environment entry A holds a NUL"},
		{"a relative working directory", func(s *api.RunSpec) { s.WorkingDir = "app" }, api.CodeInvalid, `working_dir "app" is not an absolute path`},
		{"an unknown restart policy", func(s *api.RunSpec) { s.RestartPolicy = "sometimes" }, api.CodeInvalid, `restart policy "sometimes"`},
		{"no network", func(s *api.RunSpec) { s.Network = "" }, api.CodeInvalid, "network is required"},
		{"an unknown network", func(s *api.RunSpec) { s.Network = "bridge" }, api.CodeInvalid, `network "bridge" is neither isolated nor public`},
		{"no network interface", func(s *api.RunSpec) { s.Network = "none" }, api.CodeNotSupported, "microsandbox cannot run a task with no network interface"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			spec := validSpec()
			test.mutate(&spec)

			err := validateSpec(spec)

			require.Error(t, err)
			assert.Equal(t, test.code, Code(err))
			assert.Contains(t, err.Error(), test.message)
		})
	}

	t.Run("every problem is reported at once", func(t *testing.T) {
		t.Parallel()

		err := validateSpec(api.RunSpec{Network: "none"})

		require.Error(t, err)
		assert.Equal(t, api.CodeInvalid, Code(err), "a spec that does not hold is invalid before it is unsupported")
		assert.Equal(t, "node is required; name is required; image is required; memory has to be more than 0 bytes", err.Error())
	})
}

func TestValidateExec(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateExec(api.ExecRequest{Command: []string{"sh"}, Env: []string{"A=1"}, WorkDir: "/"}))

	tests := []struct {
		name    string
		request api.ExecRequest
		message string
	}{
		{"no command", api.ExecRequest{}, "command is required"},
		{"an empty program", api.ExecRequest{Command: []string{""}}, "command is required"},
		{"an entry that is not KEY=VALUE", api.ExecRequest{Command: []string{"sh"}, Env: []string{"A"}}, "is not KEY=VALUE"},
		{"a relative working directory", api.ExecRequest{Command: []string{"sh"}, WorkDir: "tmp"}, `workdir "tmp" is not an absolute path`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := validateExec(test.request)

			require.Error(t, err)
			assert.Equal(t, api.CodeInvalid, Code(err))
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestEnvironmentMap(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		map[string]string{"A": "2", "B": "", "C": "x=y"},
		environmentMap([]string{"A=1", "B=", "C=x=y", "A=2"}),
		"the later of two entries for one key wins, as it does for docker",
	)
}

func TestResolveArgv(t *testing.T) {
	t.Parallel()

	image := ImageConfig{Entrypoint: []string{"/entrypoint.sh"}, Cmd: []string{"nginx", "-g", "daemon off;"}}

	tests := []struct {
		name       string
		entrypoint []string
		command    []string
		image      ImageConfig
		want       []string
	}{
		{"nothing asked for runs the image's entrypoint and command", nil, nil, image, []string{"/entrypoint.sh", "nginx", "-g", "daemon off;"}},
		{"a command alone runs under the image's entrypoint", nil, []string{"echo", "hi"}, image, []string{"/entrypoint.sh", "echo", "hi"}},
		{"an entrypoint alone drops the image's command", []string{"/bin/sh"}, nil, image, []string{"/bin/sh"}},
		{"both run together", []string{"/bin/sh", "-c"}, []string{"exit 3"}, image, []string{"/bin/sh", "-c", "exit 3"}},
		{"an image with a command and no entrypoint", nil, nil, ImageConfig{Cmd: []string{"sh"}}, []string{"sh"}},
		{"a command where the image has no entrypoint", nil, []string{"python", "main.py"}, ImageConfig{Cmd: []string{"python3"}}, []string{"python", "main.py"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			argv, err := resolveArgv(test.entrypoint, test.command, test.image)

			require.NoError(t, err)
			assert.Equal(t, test.want, argv)
		})
	}

	t.Run("nothing to run is invalid", func(t *testing.T) {
		t.Parallel()

		_, err := resolveArgv(nil, nil, ImageConfig{})

		require.Error(t, err)
		assert.Equal(t, api.CodeInvalid, Code(err))

		_, err = resolveArgv([]string{""}, nil, image)

		assert.Equal(t, api.CodeInvalid, Code(err))
	})

	t.Run("the image's configuration is not changed", func(t *testing.T) {
		t.Parallel()

		entrypoint := []string{"/entrypoint.sh"}

		argv, err := resolveArgv(nil, []string{"a"}, ImageConfig{Entrypoint: entrypoint})
		require.NoError(t, err)

		argv[0] = "changed"

		assert.Equal(t, "/entrypoint.sh", entrypoint[0])
	})
}

func TestParseSignal(t *testing.T) {
	t.Parallel()

	tests := map[string]syscall.Signal{
		"SIGTERM":    15,
		"TERM":       15,
		"sigint":     2,
		"15":         15,
		"9":          9,
		"SIGQUIT":    3,
		"SIGUSR1":    10,
		"USR2":       12,
		"SIGWINCH":   28,
		"SIGRTMIN":   34,
		"RTMIN+3":    37,
		"SIGRTMAX":   64,
		"SIGRTMAX-2": 62,
	}

	for value, want := range tests {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			signal, ok := parseSignal(value)

			require.True(t, ok)
			assert.Equal(t, want, signal)
		})
	}

	for _, value := range []string{"", "0", "65", "-1", "SIGNOPE", "RTMIN+40", "RTMAX-40", "RTMIN-1", "RTMIN+x"} {
		t.Run("not "+value, func(t *testing.T) {
			t.Parallel()

			_, ok := parseSignal(value)

			assert.False(t, ok)
		})
	}

	t.Run("a stop signal is the image's, or SIGTERM", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, syscall.Signal(15), stopSignalOf(ImageConfig{}))
		assert.Equal(t, syscall.Signal(3), stopSignalOf(ImageConfig{StopSignal: "SIGQUIT"}))
		assert.Equal(t, syscall.Signal(15), stopSignalOf(ImageConfig{StopSignal: "SIGNOPE"}))
	})
}

func TestEndingOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		last   Event
		ended  bool
		sent   syscall.Signal
		forced bool
		want   ending
	}{
		{"an exit with N is N", Event{Kind: EventExited, ExitCode: 3}, true, 0, false, ending{code: 3}},
		{"an exit with 0 after the stop signal is 0", Event{Kind: EventExited, ExitCode: 0}, true, 15, false, ending{code: 0}},
		{"the stop signal it was sent is 128 and the signal", Event{Kind: EventExited, ExitCode: -1}, true, 15, false, ending{code: 143}},
		{"another stop signal", Event{Kind: EventExited, ExitCode: -1}, true, 2, false, ending{code: 130}},
		{"SIGKILL is 137", Event{Kind: EventExited, ExitCode: -1}, true, 9, false, ending{code: 137}},
		{"a signal nobody sent is 137, killed", Event{Kind: EventExited, ExitCode: -1}, true, 0, false, ending{code: 137, reason: ReasonKilled}},
		{"a program not found is 127", Event{Kind: EventFailed, Errno: "ENOENT", Message: "no such file"}, true, 0, false, ending{code: 127, reason: "ENOENT: no such file"}},
		{"a program that may not be run is 126", Event{Kind: EventFailed, Errno: "EACCES"}, true, 0, false, ending{code: 126, reason: "EACCES"}},
		{"a program that is not one is 126", Event{Kind: EventFailed, Errno: "ENOEXEC"}, true, 0, false, ending{code: 126, reason: "ENOEXEC"}},
		{"anything else that keeps it from starting is 1", Event{Kind: EventFailed, Message: "out of pids"}, true, 0, false, ending{code: 1, reason: "out of pids"}},
		{"a lost stream is 137, its VM lost", Event{Kind: EventLost}, true, 0, false, ending{code: 137, reason: ReasonVMLost}},
		{"a stream that ended without its last event is lost too", Event{}, false, 0, false, ending{code: 137, reason: ReasonVMLost}},
		{"a VM the supervisor stopped under it is not lost", Event{Kind: EventLost}, true, 9, true, ending{code: 137}},
		{"nor is one lost after SIGKILL", Event{Kind: EventLost}, true, 9, false, ending{code: 137}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, endingOf(test.last, test.ended, test.sent, test.forced))
		})
	}

	t.Run("a failure with nothing to say says so", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "the main process could not be started", spawnFailureMessage(Event{Kind: EventFailed}))
	})
}

func TestPolicy(t *testing.T) {
	t.Parallel()

	t.Run("parsing", func(t *testing.T) {
		t.Parallel()

		tests := map[string]policy{
			"":               {name: policyNo},
			"no":             {name: policyNo},
			"always":         {name: policyAlways},
			"unless-stopped": {name: policyUnlessStopped},
			"on-failure":     {name: policyOnFailure},
			"on-failure:0":   {name: policyOnFailure},
			"on-failure:5":   {name: policyOnFailure, maxRetries: 5},
		}

		for value, want := range tests {
			got, err := parsePolicy(value)

			require.NoError(t, err, value)
			assert.Equal(t, want, got, value)
		}

		for _, value := range []string{"sometimes", "always:3", "no:1", "on-failure:", "on-failure:-1", "on-failure:x", "ON-FAILURE"} {
			_, err := parsePolicy(value)

			assert.Error(t, err, value)
		}
	})

	t.Run("restarting a process that ended by itself", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			policy   string
			code     int
			restarts uint
			want     bool
		}{
			{"no", 1, 0, false},
			{"always", 0, 0, true},
			{"always", 1, 100, true},
			{"unless-stopped", 0, 0, true},
			{"on-failure", 0, 0, false},
			{"on-failure", 1, 0, true},
			{"on-failure", 137, 1000, true},
			{"on-failure:2", 1, 1, true},
			{"on-failure:2", 1, 2, false},
		}

		for _, test := range tests {
			p, err := parsePolicy(test.policy)
			require.NoError(t, err)

			assert.Equal(t, test.want, p.restarts(test.code, test.restarts), "%s, exited with %d after %d restarts", test.policy, test.code, test.restarts)
		}
	})

	t.Run("reviving runs the service found running", func(t *testing.T) {
		t.Parallel()

		for value, want := range map[string]bool{"no": false, "on-failure": false, "on-failure:3": false, "always": true, "unless-stopped": true} {
			p, err := parsePolicy(value)
			require.NoError(t, err)

			assert.Equal(t, want, p.revives(), value)
		}
	})

	t.Run("backoff is docker's", func(t *testing.T) {
		t.Parallel()

		backoff := DefaultConfig().Backoff

		wait := time.Duration(0)
		var waits []time.Duration

		for range 12 {
			wait = backoff.next(wait, time.Second)
			waits = append(waits, wait)
		}

		assert.Equal(t, []time.Duration{
			100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond,
			1600 * time.Millisecond, 3200 * time.Millisecond, 6400 * time.Millisecond, 12800 * time.Millisecond,
			25600 * time.Millisecond, 51200 * time.Millisecond, time.Minute, time.Minute,
		}, waits)

		assert.Equal(t, 100*time.Millisecond, backoff.next(time.Minute, 10*time.Second), "a run that stayed up for ten seconds starts over")
	})
}

func TestLineSplitting(t *testing.T) {
	t.Parallel()

	t.Run("a line longer than the most a line holds is cut", func(t *testing.T) {
		t.Parallel()

		line := []byte(strings.Repeat("a", api.MaxLogContent*2+10))

		parts := splitLong(line)

		require.Len(t, parts, 3)
		assert.Len(t, parts[0], api.MaxLogContent)
		assert.Len(t, parts[1], api.MaxLogContent)
		assert.Len(t, parts[2], 10)
	})

	t.Run("a cut does not split a character", func(t *testing.T) {
		t.Parallel()

		// two bytes before the limit, then a character of three: cutting at
		// the limit would split it.
		data := []byte(strings.Repeat("a", api.MaxLogContent-2) + "€" + "b")

		cut := cutAt(data, api.MaxLogContent)

		assert.Equal(t, api.MaxLogContent-2, cut)
	})

	t.Run("what fits is not cut", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 3, cutAt([]byte("abc"), api.MaxLogContent))
		assert.Len(t, splitLong([]byte("abc")), 1)
	})
}

func TestBytesOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "512 bytes", bytesOf(512))
	assert.Equal(t, "96 MiB", bytesOf(96<<20))
	assert.Equal(t, "12.3 GiB", bytesOf(12<<30+307<<20))
}

func TestVersionOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0.7.6", versionOf("msb 0.7.6"))
	assert.Equal(t, "0.7.6", versionOf("v0.7.6"))
	assert.Equal(t, "0.7.6", versionOf(" 0.7.6\n"))
	assert.Equal(t, "", versionOf(""))
}
