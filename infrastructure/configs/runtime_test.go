package configs

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

func TestWorkloadControlPlane_AllowedRuntimes(t *testing.T) {
	t.Parallel()

	t.Run("a platform configured as it always was allows sysbox alone", func(t *testing.T) {
		t.Parallel()

		c := NewWorkloadControlPlane()

		allowed, err := c.AllowedRuntimes()
		require.NoError(t, err)

		assert.Equal(t, []runtime.Class{runtime.Sysbox}, allowed)
		assert.Equal(t, string(runtime.Sysbox), c.DefaultRuntime)
	})

	t.Run("both classes", func(t *testing.T) {
		t.Parallel()

		c := WorkloadControlPlane{Runtimes: "sysbox, firecracker"}

		allowed, err := c.AllowedRuntimes()
		require.NoError(t, err)

		assert.Equal(t, []runtime.Class{runtime.Sysbox, runtime.Firecracker}, allowed)
	})
}

func TestWorkloadOrchestrator_RuntimeSpecs(t *testing.T) {
	t.Parallel()

	t.Run("an orchestrator configured as it always was offers sysbox on its docker daemon", func(t *testing.T) {
		t.Parallel()

		c := WorkloadOrchestrator{DockerHost: "tcp://docker:2375", AdvertiseHost: "docker"}

		specs, err := c.RuntimeSpecs()
		require.NoError(t, err)

		assert.Equal(t, []driver.Spec{{
			Class:    runtime.Sysbox,
			Kind:     driver.KindContainer,
			Endpoint: "tcp://docker:2375",
			Options:  url.Values{driver.OptionAdvertiseHost: {"docker"}},
		}}, specs)
	})

	t.Run("a container driver that says where its ports are says so itself", func(t *testing.T) {
		t.Parallel()

		c := WorkloadOrchestrator{
			AdvertiseHost: "docker",
			Runtimes:      "sysbox=container@tcp://docker:2375?advertise-host=dind,firecracker=microvm@unix:///run/workload-vmhost/vmhost.sock",
		}

		specs, err := c.RuntimeSpecs()
		require.NoError(t, err)
		require.Len(t, specs, 2)

		assert.Equal(t, "dind", specs[0].Option(driver.OptionAdvertiseHost))
		assert.Empty(t, specs[1].Option(driver.OptionAdvertiseHost))
	})

	t.Run("one that does not is told where they used to be said to be", func(t *testing.T) {
		t.Parallel()

		c := WorkloadOrchestrator{AdvertiseHost: "docker", Runtimes: "sysbox=container@tcp://docker:2375"}

		specs, err := c.RuntimeSpecs()
		require.NoError(t, err)

		assert.Equal(t, "docker", specs[0].Option(driver.OptionAdvertiseHost))
	})

	t.Run("what cannot be read is refused", func(t *testing.T) {
		t.Parallel()

		_, err := (&WorkloadOrchestrator{Runtimes: "sysbox"}).RuntimeSpecs()
		assert.Error(t, err)
	})
}

func TestWorkloadVMHost(t *testing.T) {
	t.Parallel()

	t.Run("the defaults are usable as they are", func(t *testing.T) {
		t.Parallel()

		c := NewWorkloadVMHost()

		socket, err := c.SocketPath()
		require.NoError(t, err)
		assert.Equal(t, "/run/workload-vmhost/vmhost.sock", socket)

		pool, err := c.Pool()
		require.NoError(t, err)
		assert.Equal(t, "10.250.0.0/16", pool.String())

		ports, err := c.BlockedPorts()
		require.NoError(t, err)
		assert.Contains(t, ports, uint16(3333))

		assert.Equal(t, []string{"1.1.1.1", "9.9.9.9"}, c.Nameservers())
		assert.Empty(t, c.Registries())
		assert.Equal(t, VMHostProcessModeSystemd, c.ProcessMode)
	})

	t.Run("vmhost listens on a unix socket and nothing else", func(t *testing.T) {
		t.Parallel()

		for _, listen := range []string{"tcp://0.0.0.0:80", "/run/vmhost.sock", "unix://"} {
			_, err := (&WorkloadVMHost{Listen: listen}).SocketPath()
			assert.Error(t, err, listen)
		}
	})

	t.Run("what is not a port is refused", func(t *testing.T) {
		t.Parallel()

		_, err := (&WorkloadVMHost{BlockedEgressPorts: "3333,stratum"}).BlockedPorts()
		assert.Error(t, err)
	})
}
