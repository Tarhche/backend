package reply

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestFit(t *testing.T) {
	t.Parallel()

	// lines of 100 KiB, ten of which are more than a reply carries.
	line := strings.Repeat("x", 100<<10)
	lines := make([]string, 12)
	for n := range lines {
		lines[n] = string(rune('a'+n)) + line
	}

	t.Run("what fits is kept whole", func(t *testing.T) {
		t.Parallel()

		kept, truncated := First(lines[:3])
		assert.Equal(t, lines[:3], kept)
		assert.False(t, truncated)
	})

	t.Run("a listing keeps its first items", func(t *testing.T) {
		t.Parallel()

		kept, truncated := First(lines)
		assert.True(t, truncated)
		require.NotEmpty(t, kept)
		assert.Equal(t, lines[0], kept[0])
		assert.LessOrEqual(t, size(kept), noderequest.MaxReplyBytes)
		assert.Greater(t, size(lines[:len(kept)+1]), budget, "as many as fit, and no fewer")
	})

	t.Run("a log keeps its last lines", func(t *testing.T) {
		t.Parallel()

		kept, truncated := Last(lines)
		assert.True(t, truncated)
		require.NotEmpty(t, kept)
		assert.Equal(t, lines[len(lines)-1], kept[len(kept)-1])
		assert.LessOrEqual(t, size(kept), noderequest.MaxReplyBytes)
	})
}

func TestDecode(t *testing.T) {
	t.Parallel()

	var into struct {
		ID string `json:"id"`
	}

	require.NoError(t, Decode(nil, &into), "no payload is the zero one")
	require.NoError(t, Decode([]byte(`{"id":"c1"}`), &into))
	assert.Equal(t, "c1", into.ID)

	err := Decode([]byte(`{`), &into)

	var refused *noderequest.Error
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, noderequest.CodeInvalid, refused.Code)
}
