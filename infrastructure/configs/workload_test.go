package configs

import (
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
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

func TestWorkloadControlPlane_DockerDefaultPorts(t *testing.T) {
	tests := []struct {
		name    string
		given   string
		want    []port.Port
		wantErr bool
	}{
		{name: "the default", given: defaultWorkloadVMDockerDefaultPorts, want: []port.Port{80, 443, 8080}},
		{name: "spaces around them are not part of them", given: " 80 , 443 ", want: []port.Port{80, 443}},
		{name: "none at all", given: "", want: []port.Port{}},
		{name: "a port that is not a number", given: "80,http", wantErr: true},
		{name: "a port no machine has", given: "65536", wantErr: true},
		{name: "port zero", given: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := WorkloadControlPlane{VMDockerDefaultPorts: tt.given}

			got, err := c.DockerDefaultPorts()
			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestWorkloadConfigsBind holds the workload's settings to the flags and
// environment variables they are documented under, with the defaults the plan
// gives them.
func TestWorkloadConfigsBind(t *testing.T) {
	defaults := func(t *testing.T, target any, want map[string]string) {
		t.Helper()

		flagSet := console.NewFlagSet("workload", io.Discard)
		require.NoError(t, flagSet.Struct(target))

		for env, value := range want {
			var found *console.Flag
			for _, name := range flagNames(target) {
				if flag := flagSet.Lookup(name); flag != nil && flag.Env() == env {
					found = flag
				}
			}

			require.NotNil(t, found, "nothing is read from %s", env)
			assert.Equal(t, value, found.DefValue(), env)
			assert.NotEmpty(t, found.Usage(), env)
		}
	}

	t.Run("the vmhost", func(t *testing.T) {
		defaults(t, NewWorkloadVMHost(), map[string]string{
			"WORKLOAD_VMHOST_SOCKET":         "/run/vmhost/vmhost.sock",
			"WORKLOAD_VMHOST_PORT_RANGE":     "20000-29999",
			"WORKLOAD_VMHOST_ADVERTISE_HOST": "",
			"WORKLOAD_VMHOST_DOCKER_IMAGE":   "docker:29-dind",
			"WORKLOAD_VMHOST_CPUS":           "0",
			"WORKLOAD_VMHOST_MEMORY":         "0",
			"WORKLOAD_VMHOST_DISK":           "214748364800",
		})
	})

	t.Run("the control plane", func(t *testing.T) {
		defaults(t, NewWorkloadControlPlane(), map[string]string{
			"WORKLOAD_VM_DEFAULT_IMAGE":                  "ubuntu:24.04",
			"WORKLOAD_VM_DOCKER_IMAGE":                   "docker:29-dind",
			"WORKLOAD_VM_MIN_MEMORY":                     "134217728",
			"WORKLOAD_VM_MIN_DISK":                       "1073741824",
			"WORKLOAD_VM_DOCKER_MIN_MEMORY":              "536870912",
			"WORKLOAD_VM_DOCKER_MIN_DISK":                "4294967296",
			"WORKLOAD_VM_MAX_CPUS":                       "4",
			"WORKLOAD_VM_MAX_MEMORY":                     "8589934592",
			"WORKLOAD_VM_MAX_DISK":                       "53687091200",
			"WORKLOAD_VM_USER_MAX_VMS":                   "5",
			"WORKLOAD_VM_USER_CPUS":                      "8",
			"WORKLOAD_VM_USER_MEMORY":                    "17179869184",
			"WORKLOAD_VM_USER_DISK":                      "214748364800",
			"WORKLOAD_VM_MAX_LIFETIME":                   "720h0m0s",
			"WORKLOAD_VM_CPU_OVERCOMMIT":                 "4",
			"WORKLOAD_VM_DOCKER_DEFAULT_CPUS":            "2",
			"WORKLOAD_VM_DOCKER_DEFAULT_MEMORY":          "2147483648",
			"WORKLOAD_VM_DOCKER_DEFAULT_DISK":            "21474836480",
			"WORKLOAD_VM_DOCKER_DEFAULT_PORTS":           "80,443,8080",
			"WORKLOAD_VM_DOCKER_DEFAULT_INGRESS":         "allow",
			"WORKLOAD_VM_DOCKER_DEFAULT_EGRESS":          "allow",
			"WORKLOAD_VM_DOCKER_DEFAULT_PERSISTENT_DISK": "true",
			"WORKLOAD_VM_DOCKER_DEFAULT_LIFETIME":        "0s",
			"WORKLOAD_SNAPSHOT_USER_MAX":                 "10",
			"WORKLOAD_SNAPSHOT_S3_ENDPOINT":              "",
			"WORKLOAD_SNAPSHOT_S3_BUCKET":                "workload-snapshots",
			"WORKLOAD_SNAPSHOT_S3_USE_SSL":               "false",
			"WORKLOAD_NODE_REQUEST_TIMEOUT":              "30s",
		})
	})

	t.Run("an orchestrator", func(t *testing.T) {
		defaults(t, NewWorkloadOrchestrator(), map[string]string{
			"WORKLOAD_VMHOST_SOCKET":            "/run/vmhost/vmhost.sock",
			"WORKLOAD_DOCKER_READY_TIMEOUT":     "3m0s",
			"WORKLOAD_DOCKER_PULL_TIMEOUT":      "10m0s",
			"WORKLOAD_SNAPSHOT_S3_BUCKET":       "workload-snapshots",
			"WORKLOAD_SNAPSHOT_S3_ACCESS_KEY":   "",
			"WORKLOAD_SNAPSHOT_S3_SECRET_KEY":   "",
			"WORKLOAD_NODE_REQUEST_CONCURRENCY": "16",
		})
	})
}

// TestWorkloadPullRequestTimeout holds a node to giving a command that may
// pull an image the wait for the VM's dockerd and then the pull.
func TestWorkloadPullRequestTimeout(t *testing.T) {
	orchestrator := NewWorkloadOrchestrator()

	assert.Equal(t, 13*time.Minute, orchestrator.PullRequestTimeout(), "three minutes for dockerd and ten for the pull")

	orchestrator.DockerReadyTimeout, orchestrator.DockerPullTimeout = time.Minute, 30*time.Minute

	assert.Equal(t, 31*time.Minute, orchestrator.PullRequestTimeout())
}

// flagNames is the long name of every field of a configuration struct,
// nested structs included.
func flagNames(target any) []string {
	var names []string

	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for field := range t.Fields() {
			if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeFor[time.Duration]() {
				walk(field.Type)

				continue
			}

			if long := field.Tag.Get("long"); len(long) > 0 {
				names = append(names, long)
			}
		}
	}

	walk(reflect.TypeOf(target).Elem())

	return names
}
