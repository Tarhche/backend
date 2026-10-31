package permissions

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/permission"
)

// definitions is where the permissions are declared.
const definitions = "../../../../domain/permission/permission.go"

// repository is built once, as the blog builds it: building one sorts the
// listing in place, which two tests building theirs at once would race over.
var repository = NewRepository()

// declared reads every permission the domain declares, by its value.
func declared(t *testing.T) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), definitions, nil, 0)
	require.NoError(t, err)

	var values []string

	for _, declaration := range parsed.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}

		for _, spec := range group.Specs {
			for _, value := range spec.(*ast.ValueSpec).Values {
				literal, ok := value.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}

				unquoted, err := strconv.Unquote(literal.Value)
				require.NoError(t, err)

				values = append(values, unquoted)
			}
		}
	}

	return values
}

// TestEveryPermissionIsListed holds the listing the roles page is built from
// to what the routes are served under: a permission that is not listed is one
// no role can be given, so its routes are nobody's.
func TestEveryPermissionIsListed(t *testing.T) {
	t.Parallel()

	values := declared(t)
	require.NotEmpty(t, values)

	listed := repository.GetAll(context.Background())

	seen := make(map[string]int, len(listed))
	for _, p := range listed {
		seen[p.Value]++

		assert.NotEmpty(t, p.Name, "%q is listed without saying what it is", p.Value)
	}

	for _, value := range values {
		assert.Equal(t, 1, seen[value], "%q should be listed once", value)
	}

	assert.Len(t, listed, len(values), "everything listed is a permission the domain declares")
}

func TestTheWorkloadsPermissionsAreListed(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		permission.WorkloadVMsIndex, permission.WorkloadVMsCreate, permission.WorkloadVMsShow,
		permission.WorkloadVMsUpdate, permission.WorkloadVMsDelete, permission.WorkloadVMsManage,
		permission.WorkloadVMsLogs, permission.WorkloadVMsAttach,
		permission.WorkloadSnapshotsIndex, permission.WorkloadSnapshotsCreate, permission.WorkloadSnapshotsShow,
		permission.WorkloadSnapshotsUpdate, permission.WorkloadSnapshotsDelete,
		permission.WorkloadContainersIndex, permission.WorkloadContainersCreate, permission.WorkloadContainersShow,
		permission.WorkloadContainersDelete, permission.WorkloadContainersManage, permission.WorkloadContainersLogs,
		permission.WorkloadStacksIndex, permission.WorkloadStacksCreate, permission.WorkloadStacksShow,
		permission.WorkloadStacksDelete, permission.WorkloadStacksManage,
		permission.SelfWorkloadVMsIndex, permission.SelfWorkloadVMsShow, permission.SelfWorkloadVMsUpdate,
		permission.SelfWorkloadVMsDelete, permission.SelfWorkloadVMsManage, permission.SelfWorkloadVMsLogs,
		permission.SelfWorkloadVMsAttach,
		permission.SelfWorkloadSnapshotsIndex, permission.SelfWorkloadSnapshotsShow,
		permission.SelfWorkloadSnapshotsUpdate, permission.SelfWorkloadSnapshotsDelete,
		permission.SelfWorkloadContainersIndex, permission.SelfWorkloadContainersShow,
		permission.SelfWorkloadContainersDelete, permission.SelfWorkloadContainersManage,
		permission.SelfWorkloadContainersLogs,
		permission.SelfWorkloadStacksIndex, permission.SelfWorkloadStacksShow,
		permission.SelfWorkloadStacksManage, permission.SelfWorkloadStacksDelete,
	} {
		found, err := repository.GetOne(value)
		require.NoError(t, err, value)
		assert.Equal(t, value, found.Value)
	}
}
