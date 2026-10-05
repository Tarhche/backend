package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/container/createContainer"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// TestTimeoutsOutlastTheControlPlane holds the blog to waiting for the
// control plane longer than the control plane waits for its nodes, as it is
// configured by default. A blog that gave up first would answer timeout for a
// container the control plane was still creating, and nobody would hear what
// came of it.
func TestTimeoutsOutlastTheControlPlane(t *testing.T) {
	t.Parallel()

	controlPlane := configs.NewWorkloadControlPlane()

	assert.Greater(t, nodeRequestTimeout, controlPlane.NodeRequestTimeout, "a request a node answers")

	// a container may wait for a Docker VM made for it to come up, then for
	// its dockerd, then for its image.
	assert.Greater(t, pullRequestTimeout, createContainer.BootTimeout+controlPlane.PullRequestTimeout(), "a container created, or an image pulled")
}
