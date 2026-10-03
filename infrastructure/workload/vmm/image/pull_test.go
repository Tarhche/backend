//go:build microvm

package image

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPull pulls real images and makes them into disks with the real sqfstar.
// It reaches registries on the internet, so it runs only when asked for with
// -tags microvm; it needs squashfs-tools 4.6 or later and e2fsprogs, and no
// root.
func TestPull(t *testing.T) {
	unsquashfs, err := exec.LookPath("unsquashfs")
	if err != nil {
		t.Skip("unsquashfs is not here")
	}

	dir := filepath.Join(t.TempDir(), "images")

	store, err := NewStore(Config{Dir: dir, Registries: []string{"docker.io"}}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// listing is what unsquashfs says of a disk: each path, and its mode and
	// owner.
	listing := func(t *testing.T, root string) map[string]string {
		t.Helper()

		output, err := exec.Command(unsquashfs, "-lln", root).CombinedOutput()
		require.NoError(t, err, string(output))

		entries := map[string]string{}
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 6 || !strings.HasPrefix(fields[5], "squashfs-root") {
				continue
			}

			entries[strings.TrimPrefix(fields[5], "squashfs-root")] = fields[0] + " " + fields[1]
		}

		for path := range entries {
			assert.NotContains(t, path, ".wh.", "no whiteout is carried into the disk")
		}

		return entries
	}

	t.Run("alpine", func(t *testing.T) {
		built, err := store.Ensure(ctx, "alpine:3.20")
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(built.Digest, "sha256:"))
		assert.Equal(t, []string{"/bin/sh"}, built.Config.Cmd)

		entries := listing(t, built.Root)
		assert.Equal(t, "drwxr-xr-x 0/0", entries[""], "the root is root's")
		assert.Equal(t, "-rw-r--r-- 0/0", entries["/etc/passwd"], "the image's own files are whose they were")
		assert.Equal(t, "drwxrwxrwt 0/0", entries["/tmp"], "and have the modes they had")

		again, err := store.Ensure(ctx, "docker.io/library/alpine:3.20")
		require.NoError(t, err)
		assert.Equal(t, built.Digest, again.Digest, "an image already here is not built again")
		assert.Equal(t, built.Root, again.Root)
	})

	t.Run("busybox, whose layers name their entries ./bin/… and have one for ./ itself", func(t *testing.T) {
		built, err := store.Ensure(ctx, "busybox:1.36")
		require.NoError(t, err)

		entries := listing(t, built.Root)
		assert.Contains(t, entries, "/bin/sh")

		for path, attributes := range entries {
			assert.NotContains(t, attributes, " 1000/1000", "nothing in an image is whoever made the disk's: %s", path)
		}
	})

	t.Run("nginx, whose files are not all root's", func(t *testing.T) {
		built, err := store.Ensure(ctx, "nginx:alpine")
		require.NoError(t, err)

		assert.Equal(t, []uint16{80}, built.Config.ExposedPorts)
		assert.Equal(t, []string{"/docker-entrypoint.sh"}, built.Config.Entrypoint)

		entries := listing(t, built.Root)

		var owned []string
		for path, attributes := range entries {
			if !strings.HasSuffix(attributes, " 0/0") {
				owned = append(owned, path+" "+attributes)
			}
		}

		assert.NotEmpty(t, owned, "files of users other than root keep their owners")
		t.Logf("owned by somebody other than root: %v", owned)
	})

	t.Run("an image from a registry images may not come from is refused", func(t *testing.T) {
		_, err := store.Ensure(ctx, "quay.io/prometheus/busybox:latest")

		assert.ErrorContains(t, err, "images may only come from index.docker.io")
	})

	t.Run("the images are listed, the most recently used first", func(t *testing.T) {
		images, err := store.List(ctx)
		require.NoError(t, err)

		require.Len(t, images, 3)
		assert.Equal(t, "nginx:alpine", images[0].Reference)

		for _, image := range images {
			assert.Positive(t, image.Size)
		}
	})
}
