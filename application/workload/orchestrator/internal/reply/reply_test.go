package reply

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestLast(t *testing.T) {
	t.Parallel()

	// lines of 100 KiB, ten of which are more than a reply carries.
	line := strings.Repeat("x", 100<<10)
	lines := make([]string, 12)
	for n := range lines {
		lines[n] = string(rune('a'+n)) + line
	}

	t.Run("what fits is kept whole", func(t *testing.T) {
		t.Parallel()

		kept, truncated := Last(lines[:3])
		assert.Equal(t, lines[:3], kept)
		assert.False(t, truncated)
	})

	t.Run("a log keeps its last lines", func(t *testing.T) {
		t.Parallel()

		kept, truncated := Last(lines)
		assert.True(t, truncated)
		require.NotEmpty(t, kept)
		assert.Equal(t, lines[len(lines)-1], kept[len(kept)-1])
		assert.LessOrEqual(t, size(kept), noderequest.MaxReplyBytes)
		assert.Greater(t, size(lines[len(lines)-len(kept)-1:]), budget, "as many as fit, and no fewer")
	})
}

func TestRequired(t *testing.T) {
	t.Parallel()

	require.NoError(t, Required("vm_uuid", "01"))

	var refused *noderequest.Error
	require.ErrorAs(t, Required("vm_uuid", ""), &refused)
	assert.Equal(t, noderequest.CodeInvalid, refused.Code)
	assert.Equal(t, "vm_uuid is required", refused.Message)
}
