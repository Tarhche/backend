package container

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemoryLimit(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		bytes uint64
		want  int64
	}{
		// what compose's "256M" is read as is what docker has to be given.
		"256 MiB is handed over in bytes": {bytes: 256 << 20, want: 268435456},
		"docker's own floor stays itself": {bytes: 6 << 20, want: 6291456},
		"nothing is docker's no limit":    {bytes: 0, want: 0},
		"too large for docker is capped":  {bytes: math.MaxUint64, want: math.MaxInt64},
	}

	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, memoryLimit(tt.bytes))
		})
	}
}
