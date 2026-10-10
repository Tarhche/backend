package kind

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBeat(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	beat, beating := BeatOf(WithBeat(context.Background(), at))
	assert.True(t, beating)
	assert.Equal(t, at, beat)

	_, beating = BeatOf(context.Background())
	assert.False(t, beating, "a query is no heartbeat")
}
