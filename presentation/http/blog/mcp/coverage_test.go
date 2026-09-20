package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compositionRoot is where the blog's routes are registered.
const compositionRoot = "../../../../infrastructure/ioc/providers/blog.go"

// TestEveryRouteTheBlogServesIsReachable is the promise this server makes,
// held to when the tests run rather than when somebody starts the service.
//
// The server checks the same thing as it is built, against the router it is
// given, and refuses to be built if it does not hold. This reads the routes
// out of the composition root instead, so a route added without a tool is
// caught here rather than by the first person to boot the blog.
func TestEveryRouteTheBlogServesIsReachable(t *testing.T) {
	t.Parallel()

	patterns := registeredPatterns(t)
	require.NotEmpty(t, patterns)

	reached := make(map[string]string, len(patterns))
	for _, tool := range tools() {
		reached[tool.route] = tool.name
	}

	for _, pattern := range patterns {
		if _, ok := reached[pattern]; ok {
			continue
		}

		if reason, ok := unreachable[pattern]; ok {
			assert.NotEmpty(t, reason, "%q is left out without saying why", pattern)

			continue
		}

		t.Errorf("no tool reaches %q: give it one, or say in unreachable why it has none", pattern)
	}

	for route, name := range reached {
		assert.Contains(t, patterns, route, "the tool %q names a route the blog does not register", name)
	}
}

// TestNothingIsLeftOutWithoutAReason holds the other half of that promise: a
// route may be left out, but not silently.
func TestNothingIsLeftOutWithoutAReason(t *testing.T) {
	t.Parallel()

	patterns := registeredPatterns(t)

	reached := make(map[string]struct{}, len(patterns))
	for _, tool := range tools() {
		reached[tool.route] = struct{}{}
	}

	for pattern, reason := range unreachable {
		assert.NotEmpty(t, reason, pattern)

		if _, ok := reached[pattern]; ok {
			t.Errorf("%q has a tool as well as an excuse; one of them is wrong", pattern)
		}

		assert.Contains(t, patterns, pattern, "%q is excused but is not a route the blog registers", pattern)
	}
}

// registeredPatterns reads the "METHOD /path" patterns the composition root
// hands to the router.
func registeredPatterns(t *testing.T) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), compositionRoot, nil, 0)
	require.NoError(t, err)

	var patterns []string

	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Handle" {
			return true
		}

		if receiver, ok := selector.X.(*ast.Ident); !ok || receiver.Name != "mux" {
			return true
		}

		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			// a pattern named rather than written out, such as the MCP
			// endpoint's own, which is a constant of this package
			if pattern, ok := namedPattern(call.Args[0]); ok {
				patterns = append(patterns, pattern)
			}

			return true
		}

		pattern, err := strconv.Unquote(literal.Value)
		require.NoError(t, err)
		patterns = append(patterns, pattern)

		return true
	})

	return patterns
}

// namedPattern reads a pattern this package itself names.
func namedPattern(argument ast.Expr) (string, bool) {
	selector, ok := argument.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}

	receiver, ok := selector.X.(*ast.Ident)
	if !ok || receiver.Name != "mcpAPI" || selector.Sel.Name != "Path" {
		return "", false
	}

	return Path, true
}
