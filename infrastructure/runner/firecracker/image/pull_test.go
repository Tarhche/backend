//go:build firecracker

package image

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPull pulls a real image and makes it into a root. It reaches a registry,
// so it runs only when asked for with -tags firecracker.
func TestPull(t *testing.T) {
	store, err := NewStore(t.TempDir(), os.Getuid(), os.Getgid(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	built, err := store.Ensure(ctx, "alpine:3.20")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(built.Digest, "sha256:"))
	assert.Equal(t, []string{"/bin/sh"}, built.Config.Cmd)

	// the image's own files are in its root, whose they were.
	listing, err := exec.Command("debugfs", "-R", "stat /etc/passwd", built.Root).CombinedOutput()
	require.NoError(t, err, string(listing))
	assert.Contains(t, string(listing), "User:     0")

	again, err := store.Ensure(ctx, "alpine:3.20")
	require.NoError(t, err)
	assert.Equal(t, built, again, "an image already here is not built again")
}
