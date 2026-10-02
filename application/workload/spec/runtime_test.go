package spec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

func TestServiceRuntime(t *testing.T) {
	t.Parallel()

	t.Run("a service names its class under compose's own key", func(t *testing.T) {
		t.Parallel()

		var service Service
		require.NoError(t, json.Unmarshal([]byte(`{"image": "nginx:alpine", "runtime": "firecracker"}`), &service))

		assert.Equal(t, runtime.Firecracker, service.Runtime)
		assert.Empty(t, service.Validate(""))
	})

	t.Run("naming none leaves it to the workload", func(t *testing.T) {
		t.Parallel()

		var service Service
		require.NoError(t, json.Unmarshal([]byte(`{"image": "nginx:alpine"}`), &service))

		assert.Empty(t, service.Runtime)
		assert.Empty(t, service.Validate(""))
	})

	t.Run("what cannot be a class is refused where it was written", func(t *testing.T) {
		t.Parallel()

		service := Service{Image: "nginx:alpine", Runtime: "Fire Cracker"}

		assert.Equal(t, domain.ValidationErrors{"runtime": "invalid_value"}, service.Validate(""))
		assert.Equal(t, domain.ValidationErrors{"services.web.runtime": "invalid_value"}, service.Validate("services.web"))
	})

	t.Run("a class nobody offers is still a well formed name", func(t *testing.T) {
		t.Parallel()

		// which classes may be asked for is the workload's configuration,
		// which the specification knows nothing about.
		service := Service{Image: "nginx:alpine", Runtime: "gvisor"}

		assert.Empty(t, service.Validate(""))
	})

	t.Run("the class survives being passed on", func(t *testing.T) {
		t.Parallel()

		written, err := json.Marshal(Service{Image: "nginx:alpine", Runtime: runtime.Firecracker})
		require.NoError(t, err)

		var back Service
		require.NoError(t, json.Unmarshal(written, &back))
		assert.Equal(t, runtime.Firecracker, back.Runtime)

		// and one that names none says nothing about it, so the workload's
		// default is what it gets on the other side too.
		written, err = json.Marshal(Service{Image: "nginx:alpine"})
		require.NoError(t, err)
		assert.NotContains(t, string(written), "runtime")
	})
}

func TestStackRuntime(t *testing.T) {
	t.Parallel()

	stackOf := func(t *testing.T, body string) Stack {
		t.Helper()

		var s Stack
		require.NoError(t, json.Unmarshal([]byte(body), &s))

		return s
	}

	t.Run("the stack's class is every service's that names none", func(t *testing.T) {
		t.Parallel()

		s := stackOf(t, `{
			"name": "myapp",
			"runtime": "firecracker",
			"services": {"web": {"image": "nginx:alpine"}, "db": {"image": "postgres:17", "runtime": "firecracker"}}
		}`)

		assert.Empty(t, s.Validate())
		assert.Equal(t, runtime.Firecracker, s.ClassOf(s.Services["web"]))
		assert.Equal(t, runtime.Firecracker, s.ClassOf(s.Services["db"]))

		class, agree := s.Class(runtime.Sysbox)
		assert.True(t, agree)
		assert.Equal(t, runtime.Firecracker, class)
	})

	t.Run("services naming two classes cannot be one stack", func(t *testing.T) {
		t.Parallel()

		s := stackOf(t, `{
			"name": "myapp",
			"services": {"web": {"image": "nginx:alpine", "runtime": "sysbox"}, "db": {"image": "postgres:17", "runtime": "firecracker"}}
		}`)

		assert.Equal(t, domain.ValidationErrors{"runtime": "mixed_runtimes_in_stack"}, s.Validate())

		_, agree := s.Class(runtime.Sysbox)
		assert.False(t, agree)
	})

	t.Run("nor can a service naming another class than its stack", func(t *testing.T) {
		t.Parallel()

		s := stackOf(t, `{
			"name": "myapp",
			"runtime": "firecracker",
			"services": {"web": {"image": "nginx:alpine"}, "db": {"image": "postgres:17", "runtime": "sysbox"}}
		}`)

		assert.Equal(t, domain.ValidationErrors{"runtime": "mixed_runtimes_in_stack"}, s.Validate())
	})

	t.Run("one class named and none named agree only if the default is that class", func(t *testing.T) {
		t.Parallel()

		s := stackOf(t, `{
			"name": "myapp",
			"services": {"web": {"image": "nginx:alpine", "runtime": "firecracker"}, "db": {"image": "postgres:17"}}
		}`)

		// the default is not known here, so this is not refused here.
		assert.Empty(t, s.Validate())

		class, agree := s.Class(runtime.Firecracker)
		assert.True(t, agree)
		assert.Equal(t, runtime.Firecracker, class)

		_, agree = s.Class(runtime.Sysbox)
		assert.False(t, agree, "the service naming none would be sysbox, the other firecracker")
	})

	t.Run("a stack naming nothing is the default", func(t *testing.T) {
		t.Parallel()

		s := stackOf(t, `{"name": "myapp", "services": {"web": {"image": "nginx:alpine"}, "db": {"image": "postgres:17"}}}`)

		class, agree := s.Class(runtime.Sysbox)
		assert.True(t, agree)
		assert.Equal(t, runtime.Sysbox, class)
	})

	t.Run("what cannot be a class is refused where it was written, and not counted as one", func(t *testing.T) {
		t.Parallel()

		s := stackOf(t, `{
			"name": "myapp",
			"runtime": "Fire!",
			"services": {"web": {"image": "nginx:alpine", "runtime": "sysbox"}, "db": {"image": "postgres:17", "runtime": "Not One"}}
		}`)

		assert.Equal(t, domain.ValidationErrors{
			"runtime":             "invalid_value",
			"services.db.runtime": "invalid_value",
		}, s.Validate())
	})

	t.Run("the stack's class survives being passed on", func(t *testing.T) {
		t.Parallel()

		written, err := json.Marshal(Stack{Name: "myapp", Runtime: runtime.Firecracker, Services: map[string]Service{"web": {Image: "nginx:alpine"}}})
		require.NoError(t, err)

		var back Stack
		require.NoError(t, json.Unmarshal(written, &back))

		assert.Equal(t, runtime.Firecracker, back.Runtime)
	})
}
