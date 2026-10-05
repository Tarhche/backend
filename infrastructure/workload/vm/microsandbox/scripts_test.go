package microsandbox

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
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
