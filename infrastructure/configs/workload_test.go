package configs

import (
	"io"
	"testing"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkloadOrchestrator_IngressAddresses(t *testing.T) {
	tests := []struct {
		name  string
		given string
		want  []string
	}{
		{name: "one ingress", given: "workload-ingress:81", want: []string{"workload-ingress:81"}},
		{name: "several", given: "a:81,b:81,c:81", want: []string{"a:81", "b:81", "c:81"}},
		{name: "spaces around them are not part of them", given: " a:81 , b:81 ", want: []string{"a:81", "b:81"}},
		{name: "an empty one is not an ingress", given: "a:81,,b:81", want: []string{"a:81", "b:81"}},
		{name: "none at all", given: "", want: []string{}},
		{name: "nothing but separators", given: " , ", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := WorkloadOrchestrator{TunnelAddresses: tt.given}

			assert.Equal(t, tt.want, c.IngressAddresses())
		})
	}
}

func TestWorkloadOrchestrator_Runtime(t *testing.T) {
	// parsed is an orchestrator's configuration as the console fills it from
	// these arguments and whatever the environment holds.
	parsed := func(t *testing.T, arguments ...string) *WorkloadOrchestrator {
		t.Helper()

		c := NewWorkloadOrchestrator()

		flagSet := console.NewFlagSet("serve-workload-orchestrator", io.Discard)
		require.NoError(t, flagSet.Struct(c))
		require.NoError(t, flagSet.Parse(arguments))

		return c
	}

	t.Run("an orchestrator runs on sysbox unless it is told otherwise", func(t *testing.T) {
		c := parsed(t)

		assert.Equal(t, RuntimeSysbox, c.Runtime)
		assert.Equal(t, "https://workload-microsandbox:8443", c.MicrosandboxURL)
	})

	t.Run("an empty environment keeps the default", func(t *testing.T) {
		t.Setenv("WORKLOAD_ORCHESTRATOR_RUNTIME", "")

		assert.Equal(t, RuntimeSysbox, parsed(t).Runtime)
	})

	t.Run("the runtime and where microsandbox is are read from the environment", func(t *testing.T) {
		t.Setenv("WORKLOAD_ORCHESTRATOR_RUNTIME", "microsandbox")
		t.Setenv("WORKLOAD_MICROSANDBOX_URL", "https://10.89.0.10:8443")

		c := parsed(t)

		assert.Equal(t, RuntimeMicrosandbox, c.Runtime)
		assert.Equal(t, "https://10.89.0.10:8443", c.MicrosandboxURL)
	})

	t.Run("and from flags, which win over the environment", func(t *testing.T) {
		t.Setenv("WORKLOAD_ORCHESTRATOR_RUNTIME", "sysbox")

		c := parsed(t, "--runtime", "microsandbox", "--microsandbox-url", "https://localhost:8443")

		assert.Equal(t, RuntimeMicrosandbox, c.Runtime)
		assert.Equal(t, "https://localhost:8443", c.MicrosandboxURL)
	})
}

func TestWorkloadOrchestrator_PortsHost(t *testing.T) {
	tests := []struct {
		name          string
		runtime       string
		advertiseHost string
		url           string
		want          string
	}{
		{
			name:          "a container's ports are reached on the docker daemon's host",
			runtime:       RuntimeSysbox,
			advertiseHost: "docker",
			url:           "https://workload-microsandbox:8443",
			want:          "docker",
		},
		{
			name:          "so are they when no runtime is named",
			advertiseHost: "docker",
			url:           "https://workload-microsandbox:8443",
			want:          "docker",
		},
		{
			name:          "a microVM's are reached on the service's host",
			runtime:       RuntimeMicrosandbox,
			advertiseHost: "docker",
			url:           "https://workload-microsandbox:8443",
			want:          "workload-microsandbox",
		},
		{
			name:    "an address is a host",
			runtime: RuntimeMicrosandbox,
			url:     "https://10.89.0.10:8443",
			want:    "10.89.0.10",
		},
		{
			name:    "an IPv6 address comes without its brackets",
			runtime: RuntimeMicrosandbox,
			url:     "https://[fd00::10]:8443",
			want:    "fd00::10",
		},
		{
			name:    "a port is not part of the host, even where it is the default",
			runtime: RuntimeMicrosandbox,
			url:     "https://workload-microsandbox",
			want:    "workload-microsandbox",
		},
		{
			name:          "a URL that cannot be read has no host, whatever docker's is",
			runtime:       RuntimeMicrosandbox,
			advertiseHost: "docker",
			url:           "https://workload microsandbox:8443",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := WorkloadOrchestrator{Runtime: tt.runtime, AdvertiseHost: tt.advertiseHost, MicrosandboxURL: tt.url}

			assert.Equal(t, tt.want, c.PortsHost())
		})
	}
}
