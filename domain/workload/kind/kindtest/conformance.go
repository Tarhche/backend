// Package kindtest holds the kinds the services register to the rules every
// kind keeps, in a test, the way the MCP coverage test holds the routes to
// their tools: a kind that leaves a hole fails CI rather than production.
package kindtest

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Conformance fails t for every rule a kind registered in services breaks:
// those of its own descriptor (kind.Check), those between the services
// (kind.Services.Check), and the permissions its actions are asked under,
// which have to be among those permissions lists, with a name each
// (kind.CheckPermissions).
//
// It is given every service's registry as the services build them, so that
// what it holds to the rules is what runs. Registries with no kind in them
// are a test that holds nothing to anything, and fail it.
func Conformance(t testing.TB, services kind.Services, permissions permission.Repository) {
	t.Helper()

	descriptors := services.Descriptors()
	if len(descriptors) == 0 {
		t.Error("no kind is registered in any of the services, so none is held to anything")
	}

	var listed []permission.Permission
	if permissions == nil {
		t.Error("no permissions were given to hold the kinds' actions to")
	} else {
		listed = permissions.GetAll(t.Context())
	}

	for _, d := range descriptors {
		for _, problem := range kind.Check(d) {
			t.Error(problem)
		}

		if permissions != nil {
			for _, problem := range kind.CheckPermissions(d, listed) {
				t.Error(problem)
			}
		}
	}

	for _, problem := range services.Check() {
		t.Error(problem)
	}
}
