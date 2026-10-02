package driver

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

func TestParseSpecs(t *testing.T) {
	t.Parallel()

	t.Run("both classes, with their options", func(t *testing.T) {
		t.Parallel()

		specs, err := ParseSpecs("sysbox=container@tcp://docker:2375?oci-runtime=sysbox-runc&advertise-host=docker," +
			"firecracker=microvm@unix:///run/workload-vmhost/vmhost.sock")
		require.NoError(t, err)

		assert.Equal(t, []Spec{
			{
				Class:    runtime.Sysbox,
				Kind:     KindContainer,
				Endpoint: "tcp://docker:2375",
				Options:  url.Values{OptionOCIRuntime: {"sysbox-runc"}, OptionAdvertiseHost: {"docker"}},
			},
			{
				Class:    runtime.Firecracker,
				Kind:     KindMicroVM,
				Endpoint: "unix:///run/workload-vmhost/vmhost.sock",
				Options:  url.Values{},
			},
		}, specs)

		assert.Equal(t, "sysbox-runc", specs[0].Option(OptionOCIRuntime))
		assert.Empty(t, specs[1].Option(OptionNodeMemory))
	})

	t.Run("nothing configured is no spec at all", func(t *testing.T) {
		t.Parallel()

		specs, err := ParseSpecs(" , ")
		require.NoError(t, err)
		assert.Empty(t, specs)
	})

	t.Run("a container driver may take the docker client's own default", func(t *testing.T) {
		t.Parallel()

		specs, err := ParseSpecs("sysbox=container@")
		require.NoError(t, err)
		require.Len(t, specs, 1)
		assert.Empty(t, specs[0].Endpoint)
	})

	t.Run("a spec reads back as it was written", func(t *testing.T) {
		t.Parallel()

		written := "gvisor=container@tcp://docker:2375?oci-runtime=runsc"

		specs, err := ParseSpecs(written)
		require.NoError(t, err)
		assert.Equal(t, written, specs[0].String())
	})

	t.Run("what is not a spec is refused", func(t *testing.T) {
		t.Parallel()

		for _, given := range []string{
			"sysbox",
			"sysbox=container",
			"Sysbox=container@tcp://docker:2375",
			"sysbox=Container@tcp://docker:2375",
			"sysbox=container@/var/run/docker.sock",
			"sysbox=container@tcp://a,sysbox=container@tcp://b",
			"sysbox=container@tcp://docker:2375?oci-runtime=%zz",
		} {
			_, err := ParseSpecs(given)
			assert.Error(t, err, given)
		}
	})
}
