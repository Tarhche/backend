package microsandbox

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScripts checks that what runs in a guest is shell its sh can parse. The
// guests' shells are busybox ash and dash, so nothing beyond POSIX is used;
// this only catches what any sh refuses.
func TestScripts(t *testing.T) {
	t.Parallel()

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to parse the scripts with")
	}

	for name, script := range map[string]string{
		"vminit":            vminitScript,
		"ensure":            ensureScript,
		"preStop":           preStopScript,
		"dockerReady":       dockerReadyScript,
		"syncForSnapshot":   syncForSnapshotScript,
		"diskUsage":         diskUsageScript,
		"ensure exit codes": "exit 0",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			out, err := exec.Command(sh, "-n", "-c", script).CombinedOutput()
			assert.NoError(t, err, "%s", out)
		})
	}
}

// TestDockerReadyScript holds a Docker VM's readiness probe to asking dockerd
// and nothing more, since it is polled while dockerd starts: without dockerd's
// socket it runs nothing at all, and with it, it asks for the server's version
// rather than docker info, which runs every CLI plugin's metadata command
// each time.
func TestDockerReadyScript(t *testing.T) {
	t.Parallel()

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to run the script with")
	}

	// a unix socket's path is short everywhere: the test's own directory can
	// be too long for one on a Mac.
	dir, err := os.MkdirTemp("", "dockerready")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// the docker the probe finds says what it was asked, and that dockerd
	// answered.
	calls := filepath.Join(dir, "calls")
	docker := "#!/bin/sh\necho \"$*\" >> " + calls + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(docker), 0o755))

	socket := filepath.Join(dir, "docker.sock")
	script := strings.ReplaceAll(dockerReadyScript, "/var/run/docker.sock", socket)
	require.NotEqual(t, dockerReadyScript, script, "the probe looks for dockerd's socket")

	probe := func() error {
		cmd := exec.Command(sh, "-c", script)
		cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))

		return cmd.Run()
	}

	asked := func() string {
		b, err := os.ReadFile(calls)
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}

		require.NoError(t, err)

		return string(b)
	}

	assert.Error(t, probe(), "dockerd is not up while its socket is not there")
	assert.Empty(t, asked(), "nothing is run before dockerd's socket is there")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	assert.NoError(t, probe())
	assert.Equal(t, "version --format {{.Server.Version}}\n", asked())
}
