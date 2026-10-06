package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	kindsAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/kinds"
)

// TestTimeoutsOutlastTheControlPlane holds the blog to waiting for the
// control plane longer than the control plane waits for its nodes, as it is
// configured by default. A blog that gave up first would answer timeout for a
// container the control plane was still creating, and nobody would hear what
// came of it.
func TestTimeoutsOutlastTheControlPlane(t *testing.T) {
	t.Parallel()

	controlPlane := configs.NewWorkloadControlPlane()
	orchestrator := configs.NewWorkloadOrchestrator()

	assert.Greater(t, nodeRequestTimeout, controlPlane.NodeRequestTimeout, "a request a node answers")
	assert.GreaterOrEqual(t, commandWait, controlPlane.NodeRequestTimeout, "a command is waited for as long as a node is given to answer")

	// a container may wait for its dockerd, then for its image: the control
	// plane waits for that, as long as it may be asked to, and the blog a
	// little longer.
	assert.Greater(t, pullWait, orchestrator.PullRequestTimeout(), "what a node may take for a container, or an image")
	assert.LessOrEqual(t, pullWait, kindsAPI.MaxWait, "and no longer than the control plane waits for anything")
	assert.Greater(t, pullRequestTimeout, pullWait+requestTimeout, "a container created, or an image pulled")
}
