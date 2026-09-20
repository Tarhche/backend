package mcp

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAToolReadsItsOwnRoute(t *testing.T) {
	t.Parallel()

	article := tool{
		name:  "dashboard_article_show",
		route: "GET /api/dashboard/articles/{correlationUUID}/{language_code}",
	}

	assert.Equal(t, http.MethodGet, article.method())
	assert.Equal(t, "/api/dashboard/articles/{correlationUUID}/{language_code}", article.path())

	// the path writes its parameters as the route does; a caller fills them in
	// under the names the rest of the API uses
	assert.Equal(t, []string{"correlationUUID", "language_code"}, article.placeholders())
	assert.Equal(t, []string{"correlation_uuid", "language_code"}, article.pathParams())

	listing := tool{name: "dashboard_articles_list", route: "GET /api/dashboard/articles"}
	assert.Empty(t, listing.placeholders())
	assert.Empty(t, listing.pathParams())
}

func TestTheParametersThatComeUpAgainAndAgain(t *testing.T) {
	t.Parallel()

	t.Run("a page is an integer that starts at one", func(t *testing.T) {
		parameter := page()

		assert.Equal(t, "page", parameter.name)
		assert.Equal(t, "integer", parameter.schema.Type)
		require.NotNil(t, parameter.schema.Minimum)
		assert.Equal(t, 1.0, *parameter.schema.Minimum)
		assert.Equal(t, "1", string(parameter.schema.Default))
	})

	t.Run("an uuid says whose it is", func(t *testing.T) {
		parameter := uuidOf("task")

		assert.Equal(t, "uuid", parameter.name)
		assert.Contains(t, parameter.description, "task")
	})

	t.Run("an article is named by what it keeps across its languages", func(t *testing.T) {
		assert.Equal(t, "correlation_uuid", correlationUUID().name)
		assert.Contains(t, correlationUUID().description, "correlation uuid")
		assert.Equal(t, "language_code", languageCode().name)
	})

	t.Run("reading a log takes where to start and how much", func(t *testing.T) {
		names := make([]string, 0, 3)
		for _, parameter := range logParams("task") {
			names = append(names, parameter.name)
			assert.NotEmpty(t, parameter.description, parameter.name)
		}

		assert.Equal(t, []string{"uuid", "after", "limit"}, names)
	})
}

// TestTheTableSaysWhatEachToolDoes holds the table to what the dispatcher and
// the client assume about it.
func TestTheTableSaysWhatEachToolDoes(t *testing.T) {
	t.Parallel()

	for _, tool := range tools() {
		t.Run(tool.name, func(t *testing.T) {
			// a description is what an agent chooses by, so it is not optional
			assert.NotEmpty(t, tool.description)
			assert.True(t, strings.HasSuffix(tool.description, "."), "a description reads as a sentence")

			// a GET carries no body: what it takes is in its path or its query
			if tool.method() == http.MethodGet {
				assert.Nil(t, tool.body, "a GET should not describe a body")
				assert.True(t, tool.readOnly, "a GET changes nothing")
				assert.False(t, tool.destructive)
			}

			// a delete changes something by definition
			if tool.method() == http.MethodDelete {
				assert.False(t, tool.readOnly, "a delete is not read-only")
			}

			// the two hints answer different questions, and a tool that
			// changes nothing cannot be destructive. A POST that only asks
			// something — bookmark_exists — is read-only, and is a POST
			// because the question travels in a body.
			assert.False(t, tool.readOnly && tool.destructive, "read-only and destructive at once")

			// a file goes up as a form and comes back as itself
			if tool.upload {
				require.NotNil(t, tool.body)
				assert.Contains(t, tool.body.Properties, "name")
				assert.Contains(t, tool.body.Properties, "content")
			}

			if tool.download {
				assert.Equal(t, http.MethodGet, tool.method())
			}

			// every parameter says what it is for and what shape it has
			for _, parameter := range tool.params {
				assert.NotEmpty(t, parameter.description, parameter.name)
				require.NotNil(t, parameter.schema, parameter.name)
			}
		})
	}
}

// TestTheTableIsOneToolPerRoute is what the rest of this package leans on: a
// name is a tool, and a tool is a route.
func TestTheTableIsOneToolPerRoute(t *testing.T) {
	t.Parallel()

	table := tools()
	require.NotEmpty(t, table)

	names := make(map[string]string, len(table))
	routes := make(map[string]string, len(table))

	for _, tool := range table {
		assert.Regexp(t, `^[a-z][a-z0-9_]*$`, tool.name)

		method, path, found := strings.Cut(tool.route, " ")
		require.True(t, found, "%q is not a \"METHOD /path\" pattern", tool.route)
		assert.Contains(t, []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}, method)
		assert.True(t, strings.HasPrefix(path, "/"), tool.route)

		if previous, taken := names[tool.name]; taken {
			t.Fatalf("%q names both %q and %q", tool.name, previous, tool.route)
		}
		names[tool.name] = tool.route

		if previous, taken := routes[tool.route]; taken {
			t.Fatalf("%q is reached by both %q and %q", tool.route, previous, tool.name)
		}
		routes[tool.route] = tool.name

		// and every one of them describes something a caller can fill in,
		// including everything its own path carries
		schema, err := inputSchema(tool)
		require.NoError(t, err, tool.name)

		for _, name := range tool.pathParams() {
			assert.Contains(t, schema.Properties, name, tool.name)
			assert.Contains(t, schema.Required, name, tool.name)
		}

		resolved, err := schema.Resolve(nil)
		require.NoError(t, err, tool.name)
		require.NotNil(t, resolved)
	}
}
