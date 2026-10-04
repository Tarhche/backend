package vmhost

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServe reads the environment, which a parallel test may not.
func TestServe(t *testing.T) {
	t.Run("name, description and usage", func(t *testing.T) {
		command := NewServeCommand()

		assert.Equal(t, "serve-workload-vmhost", command.Name())
		assert.NotEmpty(t, command.Description())
		assert.Equal(t, "serve-workload-vmhost [arguments]", command.Usage())
	})

	t.Run("its configuration is read from flags, and falls back to the environment", func(t *testing.T) {
		t.Setenv("WORKLOAD_VMHOST_MAX_MEMORY", "8589934592")
		t.Setenv("WORKLOAD_VMHOST_PROCESS_MODE", "child")

		command := NewServeCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)
		command.Configure(flagSet)

		require.NoError(t, flagSet.Parse([]string{"--listen", "unix:///tmp/vmhost.sock", "--socket-gid", "1234"}))

		assert.Equal(t, uint64(8<<30), command.configs.MaxMemory)
		assert.Equal(t, "child", command.configs.ProcessMode)
		assert.Equal(t, 1234, command.configs.SocketGroup)

		socket, err := command.configs.SocketPath()
		require.NoError(t, err)
		assert.Equal(t, "/tmp/vmhost.sock", socket)
	})

	t.Run("there is no default memory budget", func(t *testing.T) {
		assert.Zero(t, NewServeCommand().configs.MaxMemory)
	})
}

func TestListen(t *testing.T) {
	t.Parallel()

	shortDir := func(t *testing.T) string {
		t.Helper()

		// a socket's path is at most 104 bytes on some systems.
		dir, err := os.MkdirTemp("", "vmh")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dir) })

		return dir
	}

	t.Run("the socket is for its owner and its group alone", func(t *testing.T) {
		t.Parallel()

		socket := filepath.Join(shortDir(t), "run", "vmhost.sock")

		listener, err := listen(socket, os.Getgid())
		require.NoError(t, err)
		defer listener.Close()

		info, err := os.Stat(socket)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o660), info.Mode().Perm())
		assert.NotZero(t, info.Mode()&os.ModeSocket)
	})

	t.Run("a socket a vmhost before this one left is replaced", func(t *testing.T) {
		t.Parallel()

		socket := filepath.Join(shortDir(t), "vmhost.sock")

		left, err := net.Listen("unix", socket)
		require.NoError(t, err)

		// a vmhost that went away without letting go of its socket.
		left.(*net.UnixListener).SetUnlinkOnClose(false)
		require.NoError(t, left.Close())

		listener, err := listen(socket, os.Getgid())
		require.NoError(t, err)
		require.NoError(t, listener.Close())
	})

	t.Run("a socket another vmhost answers on is not taken from it", func(t *testing.T) {
		t.Parallel()

		socket := filepath.Join(shortDir(t), "vmhost.sock")

		other, err := net.Listen("unix", socket)
		require.NoError(t, err)
		defer other.Close()

		go func() {
			for {
				conn, err := other.Accept()
				if err != nil {
					return
				}

				conn.Close()
			}
		}()

		_, err = listen(socket, os.Getgid())
		assert.ErrorContains(t, err, "another vmhost")
	})

	t.Run("something there that is not a socket is left alone", func(t *testing.T) {
		t.Parallel()

		socket := filepath.Join(shortDir(t), "vmhost.sock")
		require.NoError(t, os.WriteFile(socket, []byte("precious"), 0o600))

		_, err := listen(socket, os.Getgid())
		assert.ErrorContains(t, err, "not a socket")

		kept, err := os.ReadFile(socket)
		require.NoError(t, err)
		assert.Equal(t, "precious", string(kept))
	})
}
