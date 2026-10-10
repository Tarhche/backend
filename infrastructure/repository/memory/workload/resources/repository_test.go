package resources

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/resource/resourcetest"
)

func TestRepository(t *testing.T) {
	t.Parallel()

	resourcetest.Repository(t, func(*testing.T, ...string) resource.Repository {
		return NewRepository()
	})
}
