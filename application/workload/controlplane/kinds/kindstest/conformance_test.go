package kindstest_test

import (
	"testing"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kind/kindtest"
)

// TestConformance holds the fan the control plane's tests run on to the rules
// every kind keeps, so that what they show of the generic code is what a real
// kind would get.
func TestConformance(t *testing.T) {
	t.Parallel()

	kindtest.Conformance(t, kind.Services{
		ControlPlane: kindstest.Registry(&kindstest.Fans{}),
		Node:         kindstest.NodeRegistry(&kindstest.Node{}),
	}, kindstest.FanPermissions())
}
