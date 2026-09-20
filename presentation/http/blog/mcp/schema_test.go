package mcp

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
)

// articleRequest is a request in the shape this codebase writes them: what a
// handler fills in is kept out of the json, and what the use case cannot do
// without is said by Validate.
type articleRequest struct {
	Title    string   `json:"title"`
	Excerpt  string   `json:"excerpt,omitempty"`
	Tags     []string `json:"tags"`
	Author   string   `json:"-"`
	Approved bool     `json:"approved"`
}

var _ domain.Validatable = &articleRequest{}

func (r *articleRequest) Validate() domain.ValidationErrors {
	errors := make(domain.ValidationErrors)

	if len(r.Title) == 0 {
		errors["title"] = "required_field"
	}

	if len(r.Author) == 0 {
		errors["author"] = "required_field"
	}

	if len(r.Tags) > 10 {
		errors["tags"] = "too_many"
	}

	return errors
}

// composeRequest stands in for the runner's own requests, which carry the
// fields a compose file may write in more than one shape.
type composeRequest struct {
	Name string `json:"name"`

	spec.Service
}

var _ domain.Validatable = &composeRequest{}

func (r *composeRequest) Validate() domain.ValidationErrors {
	errors := r.Service.Validate("")

	if len(r.Name) == 0 {
		errors["name"] = "required_field"
	}

	return errors
}

func TestBody(t *testing.T) {
	t.Parallel()

	t.Run("what a handler fills in is never asked for", func(t *testing.T) {
		schema := body[articleRequest]()

		assert.Contains(t, schema.Properties, "title")
		assert.Contains(t, schema.Properties, "approved")
		assert.NotContains(t, schema.Properties, "author")
	})

	t.Run("what is required is what the use case says it cannot do without", func(t *testing.T) {
		schema := body[articleRequest]()

		// the use case also cannot do without an author, but a caller is never
		// asked for one, so it is not in the list either
		assert.Equal(t, []string{"title"}, schema.Required)
	})

	t.Run("only what is missing counts, not everything Validate reports", func(t *testing.T) {
		schema := body[articleRequest]()

		// "tags" is a field Validate has something to say about, but what it
		// says is not that it is required
		assert.NotContains(t, schema.Required, "tags")
	})

	t.Run("a field named in omit is left out", func(t *testing.T) {
		schema := body[articleRequest]("approved")

		assert.NotContains(t, schema.Properties, "approved")
		assert.Contains(t, schema.Properties, "title")
	})

	t.Run("the compose shapes are described as they may be written", func(t *testing.T) {
		schema := body[composeRequest]()

		// a command is a string or a list of them, not only the list it is
		// normalised into
		require.Contains(t, schema.Properties, "command")
		assert.Len(t, schema.Properties["command"].AnyOf, 2)

		// a port is a number or compose's own "8080:80", never the struct it
		// is read into
		require.Contains(t, schema.Properties, "ports")
		require.NotNil(t, schema.Properties["ports"].Items)
		assert.Len(t, schema.Properties["ports"].Items.AnyOf, 2)
		assert.NotContains(t, schema.Properties["ports"].Items.Properties, "Task")

		// an image is required, and it is a service's own requirement rather
		// than the request's
		assert.Contains(t, schema.Required, "image")
		assert.Contains(t, schema.Required, "name")
	})

	t.Run("a shape written by hand says what it requires itself", func(t *testing.T) {
		schema := object(map[string]*jsonschema.Schema{
			"name":    {Type: "string"},
			"content": {Type: "string"},
		}, "name")

		assert.Equal(t, "object", schema.Type)
		assert.Equal(t, []string{"name"}, schema.Required)
	})
}

func TestInputSchema(t *testing.T) {
	t.Parallel()

	t.Run("a path, a query and a body become one thing to fill in", func(t *testing.T) {
		schema, err := inputSchema(tool{
			name:   "thing_update",
			route:  "PUT /api/things/{thingUUID}",
			params: []Parameter{text("thing_uuid", "which thing"), page()},
			body:   object(map[string]*jsonschema.Schema{"title": {Type: "string"}}, "title"),
		})
		require.NoError(t, err)

		assert.Equal(t, "object", schema.Type)
		for _, name := range []string{"thing_uuid", "page", "title"} {
			assert.Contains(t, schema.Properties, name)
		}

		// what the path carries is required, and so is what the body says it
		// needs; a query parameter is not
		assert.Equal(t, []string{"thing_uuid", "title"}, schema.Required)

		// and nothing else may be sent
		require.NotNil(t, schema.AdditionalProperties)
		assert.NotNil(t, schema.AdditionalProperties.Not)
	})

	t.Run("a parameter keeps the description it was given", func(t *testing.T) {
		schema, err := inputSchema(tool{
			name:   "things_list",
			route:  "GET /api/things",
			params: []Parameter{page(), boolean("unread", "only the ones nobody has read")},
		})
		require.NoError(t, err)

		assert.Equal(t, "integer", schema.Properties["page"].Type)
		assert.NotEmpty(t, schema.Properties["page"].Description)
		assert.Equal(t, "1", string(schema.Properties["page"].Default))

		assert.Equal(t, "boolean", schema.Properties["unread"].Type)
		assert.Equal(t, "only the ones nobody has read", schema.Properties["unread"].Description)
	})

	t.Run("a path parameter nobody described is refused", func(t *testing.T) {
		_, err := inputSchema(tool{
			name:  "thing_show",
			route: "GET /api/things/{uuid}",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "uuid")
	})

	t.Run("something asked for twice is refused", func(t *testing.T) {
		_, err := inputSchema(tool{
			name:   "thing_update",
			route:  "PUT /api/things/{uuid}",
			params: []Parameter{uuidOf("thing")},
			body:   object(map[string]*jsonschema.Schema{"uuid": {Type: "string"}}),
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "twice")
	})
}

func TestSnake(t *testing.T) {
	t.Parallel()

	for name, expected := range map[string]string{
		"uuid":            "uuid",
		"language_code":   "language_code",
		"correlationUUID": "correlation_uuid",
		"hashtag":         "hashtag",
		"identity":        "identity",
		"thingUUID":       "thing_uuid",
		"UUID":            "uuid",
		"httpPort":        "http_port",
	} {
		assert.Equal(t, expected, snake(name), name)
	}
}
