package executions

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/executionstest"
)

func TestRepository(t *testing.T) {
	t.Parallel()

	executionstest.Repository(t, func(*testing.T) graph.Executions {
		return NewRepository()
	})
}
