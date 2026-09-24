package image

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestDirName(t *testing.T) {
	assert.Equal(t, "sha256-abcdef", dirName("sha256:abcdef"))
}

func TestNormalize(t *testing.T) {
	t.Run("entries are named relative to the root, and the root itself is left to sqfstar", func(t *testing.T) {
		var layers bytes.Buffer
		writer := tar.NewWriter(&layers)

		entries := []tar.Header{
			{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "./bin/", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "./bin/busybox", Typeflag: tar.TypeReg, Mode: 0o755, Size: 4, Uid: 0},
			{Name: "./bin/sh", Typeflag: tar.TypeLink, Linkname: "./bin/busybox"},
			{Name: "bin/ls", Typeflag: tar.TypeSymlink, Linkname: "./busybox"},
			{Name: "/etc/passwd", Typeflag: tar.TypeReg, Mode: 0o644, Size: 0, Uid: 0},
			{Name: "../../escape", Typeflag: tar.TypeReg, Mode: 0o644, Size: 0},
		}

		for _, entry := range entries {
			require.NoError(t, writer.WriteHeader(&entry))

			if entry.Size > 0 {
				_, err := writer.Write([]byte("elf!"))
				require.NoError(t, err)
			}
		}
		require.NoError(t, writer.Close())

		var normalized bytes.Buffer
		require.NoError(t, normalize(&layers, &normalized))

		reader := tar.NewReader(&normalized)

		var names []string
		links := map[string]string{}
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)

			names = append(names, header.Name)
			if header.Linkname != "" {
				links[header.Name] = header.Linkname
			}

			if header.Name == "bin/busybox" {
				content, err := io.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, "elf!", string(content), "what an entry holds travels with it")
			}
		}

		assert.Equal(t, []string{"bin", "bin/busybox", "bin/sh", "bin/ls", "etc/passwd", "escape"}, names)
		assert.Equal(t, "bin/busybox", links["bin/sh"], "a hard link names its entry the same way")
		assert.Equal(t, "./busybox", links["bin/ls"], "a symlink says what it says")
	})
}
