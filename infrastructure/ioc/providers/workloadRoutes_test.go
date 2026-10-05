package providers

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workloadRoute is one of the dashboard's workload routes as the composition
// root registers it: its pattern, and the code that builds what serves it.
type workloadRoute struct {
	pattern string
	handler string
}

// workloadRoutes reads the workload's dashboard routes out of blog.go.
func workloadRoutes(t *testing.T) []workloadRoute {
	t.Helper()

	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, "blog.go", nil, 0)
	require.NoError(t, err)

	var routes []workloadRoute

	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}

		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Handle" {
			return true
		}

		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}

		pattern, err := strconv.Unquote(literal.Value)
		require.NoError(t, err)

		if !strings.Contains(pattern, " /api/dashboard/workload/") && !strings.Contains(pattern, " /api/dashboard/my/workload/") {
			return true
		}

		var handler bytes.Buffer
		require.NoError(t, printer.Fprint(&handler, files, call.Args[1]))

		routes = append(routes, workloadRoute{pattern: pattern, handler: handler.String()})

		return true
	})

	return routes
}

// TestTheWorkloadsRoutesActOnWhoseThingsTheySay holds the composition root to
// what the two sets of workload routes promise, because one route of the my
// set wired as one of the workload's would hand anybody's VMs to whoever asks
// for their own: a route of the my set acts on the caller's own, under a self
// permission; one of the workload's set acts on anybody's, under the
// workload's permission, and a create there acts for the caller.
func TestTheWorkloadsRoutesActOnWhoseThingsTheySay(t *testing.T) {
	t.Parallel()

	creates := map[string]struct{}{
		"POST /api/dashboard/workload/vms":                  {},
		"POST /api/dashboard/workload/vms/{uuid}/snapshots": {},
		"POST /api/dashboard/workload/containers":           {},
		"POST /api/dashboard/workload/stacks":               {},
	}

	routes := workloadRoutes(t)

	var mine, anybodys int

	for _, route := range routes {
		t.Run(route.pattern, func(t *testing.T) {
			if strings.Contains(route.pattern, " /api/dashboard/my/workload/") {
				mine++

				assert.Contains(t, route.handler, "dashboardWorkloadAPI.Caller")
				assert.NotContains(t, route.handler, "dashboardWorkloadAPI.Anybody")
				assert.Contains(t, route.handler, "permission.SelfWorkload")

				return
			}

			anybodys++

			assert.NotContains(t, route.handler, "dashboardWorkloadAPI.Caller")
			assert.NotContains(t, route.handler, "permission.SelfWorkload")
			assert.Contains(t, route.handler, "permission.Workload")

			if _, create := creates[route.pattern]; create {
				// a create takes the caller itself, whichever set it is in.
				assert.NotContains(t, route.handler, "dashboardWorkloadAPI.Anybody")

				return
			}

			assert.Contains(t, route.handler, "dashboardWorkloadAPI.Anybody")
		})
	}

	assert.Equal(t, 43, anybodys, "every route of the workload's set, creates included")
	assert.Equal(t, 39, mine, "every route of the my set, which has no creates")
}

// TestAWorkloadRouteIsServedUnderItsOwnKindOfPermission holds each route to
// the permission of the thing it acts on: a VM's routes to the VM ones, the
// images, networks and volumes inside a Docker VM to the containers' ones.
func TestAWorkloadRouteIsServedUnderItsOwnKindOfPermission(t *testing.T) {
	t.Parallel()

	for _, route := range workloadRoutes(t) {
		_, path, _ := strings.Cut(route.pattern, " ")
		path = strings.TrimPrefix(strings.TrimPrefix(path, "/api/dashboard/my/workload"), "/api/dashboard/workload")

		var kind string
		switch {
		case strings.HasPrefix(path, "/snapshots"), strings.HasSuffix(path, "/snapshots"):
			kind = "Snapshots"
		case strings.HasPrefix(path, "/stacks"):
			kind = "Stacks"
		case strings.HasPrefix(path, "/containers"),
			strings.Contains(path, "/containers"),
			strings.Contains(path, "/images"),
			strings.Contains(path, "/networks"),
			strings.Contains(path, "/volumes"):
			kind = "Containers"
		default:
			kind = "VMs"
		}

		assert.Contains(t, route.handler, "Workload"+kind, route.pattern)
	}
}
