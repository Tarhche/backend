package stacks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

func TestStackBson_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a stack's class is stored, and read back", func(t *testing.T) {
		t.Parallel()

		encoded, err := bson.Marshal(toBson(&stack.Stack{UUID: "stack-uuid", Runtime: runtime.Firecracker}))
		require.NoError(t, err)

		var raw bson.M
		require.NoError(t, bson.Unmarshal(encoded, &raw))
		assert.Equal(t, "firecracker", raw["runtime"])

		var back StackBson
		require.NoError(t, bson.Unmarshal(encoded, &back))

		assert.Equal(t, runtime.Firecracker, toStack(&back).Runtime)
	})

	t.Run("a stack stored before there were classes ran under sysbox", func(t *testing.T) {
		t.Parallel()

		encoded, err := bson.Marshal(bson.M{"_id": "stack-uuid", "name": "myapp", "slug": "myapp-abcde"})
		require.NoError(t, err)

		var stored StackBson
		require.NoError(t, bson.Unmarshal(encoded, &stored))

		assert.Equal(t, runtime.Sysbox, toStack(&stored).Runtime)
	})
}
