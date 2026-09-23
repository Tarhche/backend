package image

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
)

func TestConfigOf(t *testing.T) {
	t.Run("an image's own say in how it runs is kept, and its TCP ports in order", func(t *testing.T) {
		config := configOf(v1.Config{
			Entrypoint:   []string{"/docker-entrypoint.sh"},
			Cmd:          []string{"nginx", "-g", "daemon off;"},
			Env:          []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			WorkingDir:   "/",
			User:         "101:101",
			ExposedPorts: map[string]struct{}{"80/tcp": {}, "443/tcp": {}, "53/udp": {}, "8080": {}},
		})

		assert.Equal(t, Config{
			Entrypoint:   []string{"/docker-entrypoint.sh"},
			Cmd:          []string{"nginx", "-g", "daemon off;"},
			Env:          []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			WorkingDir:   "/",
			User:         "101:101",
			ExposedPorts: []uint16{80, 443, 8080},
		}, config)
	})
}

func TestFilesystemSize(t *testing.T) {
	t.Run("a filesystem has room for what it holds, and for its own tables", func(t *testing.T) {
		size := filesystemSize(100 << 20)

		assert.GreaterOrEqual(t, size, int64(125<<20+32<<20))
		assert.Zero(t, size%sizeAlign)
	})

	t.Run("a tiny image still gets a filesystem ext4 can be made in", func(t *testing.T) {
		assert.Equal(t, int64(minimumSize), filesystemSize(1<<10))
	})
}

func TestDirName(t *testing.T) {
	assert.Equal(t, "sha256-abcdef", dirName("sha256:abcdef"))
}
