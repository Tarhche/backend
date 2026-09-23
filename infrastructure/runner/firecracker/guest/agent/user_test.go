//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// image builds a root holding the given files.
func image(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()

	for path, content := range files {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o755))
	}

	return root
}

func TestResolveUser(t *testing.T) {
	root := image(t, map[string]string{
		"etc/passwd": "root:x:0:0:root:/root:/bin/sh\n" +
			"nginx:x:101:101:nginx:/var/cache/nginx:/sbin/nologin\n" +
			"# a comment\n",
		"etc/group": "root:x:0:root\n" +
			"nginx:x:101:\n" +
			"www-data:x:82:nginx,other\n",
	})

	t.Run("nobody named is root", func(t *testing.T) {
		who, err := resolveUser(root, "")

		require.NoError(t, err)
		assert.Equal(t, user{uid: 0, gid: 0, home: "/root"}, who)
	})

	t.Run("a name is read out of the image, with the groups it is a member of", func(t *testing.T) {
		who, err := resolveUser(root, "nginx")

		require.NoError(t, err)
		assert.Equal(t, user{uid: 101, gid: 101, groups: []uint32{82}, home: "/var/cache/nginx"}, who)
	})

	t.Run("a group named after a colon is the one it runs with", func(t *testing.T) {
		who, err := resolveUser(root, "nginx:www-data")

		require.NoError(t, err)
		assert.Equal(t, uint32(82), who.gid)

		who, err = resolveUser(root, "101:5")

		require.NoError(t, err)
		assert.Equal(t, uint32(101), who.uid)
		assert.Equal(t, uint32(5), who.gid)
	})

	t.Run("a uid need not be in the image, but a name has to be", func(t *testing.T) {
		who, err := resolveUser(root, "1000")

		require.NoError(t, err)
		assert.Equal(t, user{uid: 1000, gid: 0, home: "/"}, who)

		_, err = resolveUser(root, "somebody")
		assert.ErrorContains(t, err, "no user")

		_, err = resolveUser(root, "nginx:nobody")
		assert.ErrorContains(t, err, "no group")
	})

	t.Run("an image without a passwd still runs as root", func(t *testing.T) {
		who, err := resolveUser(t.TempDir(), "")

		require.NoError(t, err)
		assert.Equal(t, uint32(0), who.uid)
	})
}

func TestLookPath(t *testing.T) {
	root := image(t, map[string]string{
		"bin/busybox":           "",
		"usr/local/bin/app":     "",
		"usr/bin/directory/.ok": "",
	})
	require.NoError(t, os.Symlink("/bin/busybox", filepath.Join(root, "bin/sh")))

	t.Run("a command is looked for along PATH inside the root", func(t *testing.T) {
		found, err := lookPath(root, "app", "")

		require.NoError(t, err)
		assert.Equal(t, "/usr/local/bin/app", found)
	})

	t.Run("a symlink is taken as it is, since it points inside the image", func(t *testing.T) {
		found, err := lookPath(root, "sh", "/bin")

		require.NoError(t, err)
		assert.Equal(t, "/bin/sh", found)
	})

	t.Run("a path is taken as it is", func(t *testing.T) {
		found, err := lookPath(root, "./run.sh", "")

		require.NoError(t, err)
		assert.Equal(t, "./run.sh", found)
	})

	t.Run("a directory is not a command, and neither is nothing", func(t *testing.T) {
		_, err := lookPath(root, "directory", "/usr/bin")
		assert.Error(t, err)

		_, err = lookPath(root, "missing", "")
		assert.Error(t, err)

		_, err = lookPath(root, "", "")
		assert.Error(t, err)
	})
}

func TestWithDefaults(t *testing.T) {
	t.Run("a process is given a PATH and a HOME unless it has its own", func(t *testing.T) {
		assert.Equal(t, []string{"A=1", "PATH=" + defaultPath, "HOME=/root"}, withDefaults([]string{"A=1"}, "/root"))
		assert.Equal(t, []string{"PATH=/bin", "HOME=/home"}, withDefaults([]string{"PATH=/bin", "HOME=/home"}, "/root"))
	})

	t.Run("the last of a variable given twice is the one that counts", func(t *testing.T) {
		value, found := envValue([]string{"A=1", "A=2"}, "A")

		assert.True(t, found)
		assert.Equal(t, "2", value)
	})
}
