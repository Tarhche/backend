package spec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

func TestServiceUnmarshalling(t *testing.T) {
	t.Parallel()

	t.Run("takes a compose service as it stands", func(t *testing.T) {
		t.Parallel()

		var service Service
		require.NoError(t, json.Unmarshal([]byte(`{
			"image": "nginx:1.27-alpine",
			"command": ["nginx", "-g", "daemon off;"],
			"entrypoint": "/docker-entrypoint.sh",
			"working_dir": "/app",
			"environment": {"TZ": "UTC", "APP_ENV": "production"},
			"ports": ["8080:80", 443, "127.0.0.1:9090:9000/tcp"],
			"restart": "unless-stopped",
			"network_mode": "public",
			"deploy": {"resources": {"limits": {"cpus": "0.5", "memory": "256M"}}}
		}`), &service))

		assert.Equal(t, "nginx:1.27-alpine", service.Image)
		assert.Equal(t, StringOrSlice{"nginx", "-g", "daemon off;"}, service.Command)
		assert.Equal(t, StringOrSlice{"/docker-entrypoint.sh"}, service.Entrypoint)
		assert.Equal(t, "/app", service.WorkingDir)

		// a map has no order, so the entries come out sorted.
		assert.Equal(t, Environment{"APP_ENV=production", "TZ=UTC"}, service.Environment)

		// only the container side of a port is honoured; the runner picks the
		// host side itself.
		assert.Equal(t, []port.Port{80, 443, 9000}, service.ExposedPorts())

		assert.Equal(t, network.PolicyPublic, service.NetworkPolicy())
		assert.Equal(t, Decimal(0.5), service.Deploy.Resources.Limits.CPUs)
		assert.Equal(t, ByteSize(256<<20), service.Deploy.Resources.Limits.Memory)
	})

	t.Run("takes the list shapes of the same fields", func(t *testing.T) {
		t.Parallel()

		var service Service
		require.NoError(t, json.Unmarshal([]byte(`{
			"image": "app:1",
			"command": "npm run start",
			"environment": ["TZ=UTC", "APP_ENV=production"],
			"deploy": {"resources": {"limits": {"cpus": 2, "memory": 1073741824}}}
		}`), &service))

		assert.Equal(t, StringOrSlice{"npm", "run", "start"}, service.Command)
		assert.Equal(t, Environment{"TZ=UTC", "APP_ENV=production"}, service.Environment)
		assert.Equal(t, Decimal(2), service.Deploy.Resources.Limits.CPUs)
		assert.Equal(t, ByteSize(1<<30), service.Deploy.Resources.Limits.Memory)
	})

	t.Run("a host-side range still names one container port", func(t *testing.T) {
		t.Parallel()

		// the runner picks the host port itself, so whatever a compose file
		// asked for on the host side is not something to fail over.
		var service Service
		require.NoError(t, json.Unmarshal([]byte(`{"image": "a:1", "ports": ["8000-8010:80"]}`), &service))

		assert.Equal(t, []port.Port{80}, service.ExposedPorts())
	})

	t.Run("a duplicated container port is exposed once", func(t *testing.T) {
		t.Parallel()

		var service Service
		require.NoError(t, json.Unmarshal([]byte(`{"image": "a:1", "ports": ["8080:80", "9090:80", 80]}`), &service))

		assert.Equal(t, []port.Port{80}, service.ExposedPorts())
	})

	t.Run("rejects what it cannot honour", func(t *testing.T) {
		t.Parallel()

		testcases := map[string]string{
			"a container port range":      `{"image": "a:1", "ports": ["8000-8010"]}`,
			"a mapped container range":    `{"image": "a:1", "ports": ["8000-8010:80-90"]}`,
			"a port that is text":         `{"image": "a:1", "ports": ["http"]}`,
			"a command as an object":      `{"image": "a:1", "command": {"run": true}}`,
			"an environment as a number":  `{"image": "a:1", "environment": 3}`,
			"cpus that are not a number":  `{"image": "a:1", "deploy": {"resources": {"limits": {"cpus": "half"}}}}`,
			"a memory that is not a size": `{"image": "a:1", "deploy": {"resources": {"limits": {"memory": "lots"}}}}`,
		}

		for name, payload := range testcases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				var service Service
				assert.Error(t, json.Unmarshal([]byte(payload), &service))
			})
		}
	})
}

func TestByteSize(t *testing.T) {
	t.Parallel()

	testcases := map[string]ByteSize{
		`"256M"`:  256 << 20,
		`"256m"`:  256 << 20,
		`"256MB"`: 256 << 20,
		`"2G"`:    2 << 30,
		`"512k"`:  512 << 10,
		`"1024b"`: 1024,
		`"1024"`:  1024,
		`1024`:    1024,
		`""`:      0,
	}

	for payload, want := range testcases {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()

			var size ByteSize
			require.NoError(t, json.Unmarshal([]byte(payload), &size))
			assert.Equal(t, want, size)
		})
	}
}

func TestServiceValidate(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		service Service
		want    domain.ValidationErrors
	}{
		{
			name:    "a service that only names an image is enough",
			service: Service{Image: "nginx:alpine"},
			want:    domain.ValidationErrors{},
		},
		{
			name:    "an image is required",
			service: Service{},
			want:    domain.ValidationErrors{"image": "required_field"},
		},
		{
			name:    "an unknown restart policy is rejected",
			service: Service{Image: "a:1", Restart: "sometimes"},
			want:    domain.ValidationErrors{"restart": "invalid_value"},
		},
		{
			name:    "an unknown network policy is rejected",
			service: Service{Image: "a:1", NetworkMode: "host"},
			want:    domain.ValidationErrors{"network_mode": "invalid_network_policy"},
		},
		{
			name:    "a port on a container with no network is a contradiction",
			service: Service{Image: "a:1", NetworkMode: "none", Ports: Ports{{Container: 80}}},
			want:    domain.ValidationErrors{"ports": "ports_require_network"},
		},
		{
			name:    "port zero is not a port",
			service: Service{Image: "a:1", Ports: Ports{{Container: 0}}},
			want:    domain.ValidationErrors{"ports": "invalid_value"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.service.Validate(""))
		})
	}

	t.Run("an error inside a stack points at the service it came from", func(t *testing.T) {
		t.Parallel()

		service := Service{}

		assert.Equal(t, domain.ValidationErrors{"services.api.image": "required_field"}, service.Validate("services.api"))
	})
}

func TestServiceResourceLimits(t *testing.T) {
	t.Parallel()

	defaults := task.ResourceLimits{Cpu: 0.5, Memory: 128 << 20, Disk: 200 << 20}

	t.Run("falls back to the defaults for what it does not name", func(t *testing.T) {
		t.Parallel()

		service := Service{Image: "a:1"}

		assert.Equal(t, defaults, service.ResourceLimits(defaults))
	})

	t.Run("keeps what it does name", func(t *testing.T) {
		t.Parallel()

		var service Service
		require.NoError(t, json.Unmarshal([]byte(`{"image":"a:1","deploy":{"resources":{"limits":{"cpus":"2","memory":"1G"}}}}`), &service))

		assert.Equal(t, task.ResourceLimits{Cpu: 2, Memory: 1 << 30, Disk: defaults.Disk}, service.ResourceLimits(defaults))
	})
}

func TestStackValidate(t *testing.T) {
	t.Parallel()

	t.Run("a compose file's services block is enough", func(t *testing.T) {
		t.Parallel()

		var stack Stack
		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "myapp",
			"services": {
				"web": {"image": "nginx:alpine", "ports": ["80"]},
				"api": {"image": "api:1", "environment": {"DATABASE_URL": "postgres://db:5432/app"}},
				"db":  {"image": "postgres:17", "environment": {"POSTGRES_PASSWORD": "secret"}}
			}
		}`), &stack))

		assert.Empty(t, stack.Validate())
		assert.Len(t, stack.Services, 3)
	})

	t.Run("a stack needs a name and at least one service", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, domain.ValidationErrors{
			"name":     "required_field",
			"services": "required_field",
		}, (&Stack{}).Validate())
	})

	t.Run("a service name has to be reachable as a hostname", func(t *testing.T) {
		t.Parallel()

		stack := Stack{Name: "myapp", Services: map[string]Service{"My Web": {Image: "a:1"}}}

		assert.Equal(t, domain.ValidationErrors{"services.My Web": "invalid_value"}, stack.Validate())
	})

	t.Run("a stack is capped", func(t *testing.T) {
		t.Parallel()

		services := make(map[string]Service, maxServices+1)
		for i := range maxServices + 1 {
			services[string(rune('a'+i%26))+string(rune('a'+i/26))] = Service{Image: "a:1"}
		}

		assert.Equal(t, domain.ValidationErrors{"services": "too_many_services"}, (&Stack{Name: "big", Services: services}).Validate())
	})
}

func TestServiceRoundTrip(t *testing.T) {
	t.Parallel()

	// a specification is read by the service the browser talks to and passed on
	// to the one that runs it, so it has to survive being written back out.
	var read Service
	require.NoError(t, json.Unmarshal([]byte(`{
		"image": "nginx:1.27-alpine",
		"command": "nginx -g 'daemon off;'",
		"environment": {"TZ": "UTC"},
		"ports": ["8080:80", 443],
		"network_mode": "public",
		"restart": "always",
		"deploy": {"resources": {"limits": {"cpus": "0.5", "memory": "256M"}}}
	}`), &read))

	written, err := json.Marshal(read)
	require.NoError(t, err)

	var back Service
	require.NoError(t, json.Unmarshal(written, &back))

	assert.Equal(t, read, back)
	assert.Equal(t, []port.Port{80, 443}, back.ExposedPorts())
	assert.Equal(t, network.PolicyPublic, back.NetworkPolicy())
	assert.Equal(t, task.ResourceLimits{Cpu: 0.5, Memory: 256 << 20, Disk: 7},
		back.ResourceLimits(task.ResourceLimits{Disk: 7}))
}

func TestStackRoundTrip(t *testing.T) {
	t.Parallel()

	var read Stack
	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "myapp",
		"services": {"web": {"image": "nginx:alpine", "ports": ["80"]}}
	}`), &read))

	written, err := json.Marshal(read)
	require.NoError(t, err)

	var back Stack
	require.NoError(t, json.Unmarshal(written, &back))

	assert.Equal(t, read, back)
	assert.Empty(t, back.Validate())
}
