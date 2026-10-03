package allowed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

func TestClasses(t *testing.T) {
	t.Parallel()

	t.Run("a task naming no class gets the default, and one naming one keeps it", func(t *testing.T) {
		t.Parallel()

		classes, err := New([]runtime.Class{runtime.Sysbox, runtime.Firecracker}, runtime.Firecracker)
		require.NoError(t, err)

		assert.Equal(t, runtime.Firecracker, classes.Resolve(""))
		assert.Equal(t, runtime.Sysbox, classes.Resolve(runtime.Sysbox))
		assert.Equal(t, runtime.Firecracker, classes.Default())
	})

	t.Run("only what the platform names may be asked for, in the order it names them", func(t *testing.T) {
		t.Parallel()

		classes, err := New([]runtime.Class{runtime.Firecracker, runtime.Sysbox}, runtime.Sysbox)
		require.NoError(t, err)

		assert.True(t, classes.Allows(runtime.Sysbox))
		assert.True(t, classes.Allows(runtime.Firecracker))
		assert.False(t, classes.Allows("gvisor"))
		assert.False(t, classes.Allows(""))

		assert.Equal(t, []runtime.Class{runtime.Firecracker, runtime.Sysbox}, classes.All())
	})

	t.Run("a default nobody may ask for is refused", func(t *testing.T) {
		t.Parallel()

		_, err := New([]runtime.Class{runtime.Sysbox}, runtime.Firecracker)
		assert.Error(t, err)

		_, err = New([]runtime.Class{runtime.Sysbox}, "")
		assert.Error(t, err)
	})

	t.Run("so is allowing nothing at all, or what cannot be a class", func(t *testing.T) {
		t.Parallel()

		_, err := New(nil, runtime.Sysbox)
		assert.Error(t, err)

		_, err = New([]runtime.Class{"Not A Class"}, "Not A Class")
		assert.Error(t, err)
	})

	t.Run("nothing configured is the platform as it always was", func(t *testing.T) {
		t.Parallel()

		var classes Classes

		assert.Equal(t, []runtime.Class{runtime.Sysbox}, classes.All())
		assert.Equal(t, runtime.Sysbox, classes.Default())
		assert.Equal(t, runtime.Sysbox, classes.Resolve(""))
		assert.True(t, classes.Allows(runtime.Sysbox))
		assert.False(t, classes.Allows(runtime.Firecracker))
	})

	t.Run("what it hands out cannot change it", func(t *testing.T) {
		t.Parallel()

		given := []runtime.Class{runtime.Sysbox, runtime.Firecracker}
		classes, err := New(given, runtime.Sysbox)
		require.NoError(t, err)

		given[1] = "gvisor"
		all := classes.All()
		all[0] = "kata"

		assert.Equal(t, []runtime.Class{runtime.Sysbox, runtime.Firecracker}, classes.All())
	})
}
