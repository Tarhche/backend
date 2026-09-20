package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
)

// requiredField is what a use case says about a field it cannot do without.
// Asking a request what it needs, rather than writing it down again here, is
// what keeps a tool's schema and the use case behind it from drifting apart.
const requiredField = "required_field"

// composeSchemas describe the fields a compose file writes in more than one
// shape. Inferring them from the Go types would describe what they are
// normalised into rather than what may be sent, so a caller would be told that
// a command must be a list and that a port is an object with a Task in it.
var composeSchemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[spec.StringOrSlice](): {
		Description: "a command line, either as one string or as its arguments",
		AnyOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
		},
	},
	reflect.TypeFor[spec.Environment](): {
		Description: `the environment, either as {"KEY": "value"} or as a list of "KEY=value"`,
		AnyOf: []*jsonschema.Schema{
			{Type: "object", AdditionalProperties: &jsonschema.Schema{Type: "string"}},
			{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
		},
	},
	reflect.TypeFor[spec.Port](): {
		Description: `a port the task listens on, as a number or in compose's own "8080:80" form; the runner picks the host side itself`,
		AnyOf: []*jsonschema.Schema{
			{Type: "integer", Minimum: new(1.0), Maximum: new(65535.0)},
			{Type: "string"},
		},
	},
	reflect.TypeFor[spec.Decimal](): {
		Description: "a number, or the same number written as a string",
		AnyOf: []*jsonschema.Schema{
			{Type: "number"},
			{Type: "string"},
		},
	},
	reflect.TypeFor[spec.ByteSize](): {
		Description: `a size in bytes, or with a unit the way compose writes one, such as "256M"`,
		AnyOf: []*jsonschema.Schema{
			{Type: "integer"},
			{Type: "string"},
		},
	},
}

// body describes what a use case's request carries, from the request itself.
//
// What is required comes from asking an empty request what is missing: fields
// the handler fills in — whoever is asking, the language the request arrived
// in — are not in the schema at all, because they are not part of the json a
// caller sends, and so they are never asked for.
func body[T any](omit ...string) *jsonschema.Schema {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: composeSchemas})
	if err != nil {
		panic(fmt.Errorf("mcp: describing %T: %w", *new(T), err))
	}

	for _, name := range omit {
		delete(schema.Properties, name)
	}

	var request T
	required := make([]string, 0, len(schema.Required))
	if validatable, ok := any(&request).(domain.Validatable); ok {
		for field, failure := range validatable.Validate() {
			if failure != requiredField {
				continue
			}

			if _, asked := schema.Properties[field]; asked {
				required = append(required, field)
			}
		}

		slices.Sort(required)
		schema.Required = required
	}

	return schema
}

// object is a schema written by hand, for a request whose json is not the
// shape of the struct it is read into.
func object(properties map[string]*jsonschema.Schema, required ...string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

// Parameter is something a route reads out of its path or its query string.
type Parameter struct {
	name        string
	description string
	schema      *jsonschema.Schema
}

func text(name string, description string) Parameter {
	return Parameter{name: name, description: description, schema: &jsonschema.Schema{Type: "string"}}
}

func integer(name string, description string) Parameter {
	return Parameter{name: name, description: description, schema: &jsonschema.Schema{Type: "integer", Minimum: new(1.0)}}
}

func boolean(name string, description string) Parameter {
	return Parameter{name: name, description: description, schema: &jsonschema.Schema{Type: "boolean"}}
}

// page is the parameter nearly every listing takes.
func page() Parameter {
	parameter := integer("page", "which page of the listing to return, starting at 1")
	parameter.schema.Default = json.RawMessage("1")

	return parameter
}

// inputSchema is everything a tool takes, in one flat object: what its path
// carries, what its query string carries and what its body carries, so a
// caller has one thing to fill in rather than three.
func inputSchema(t tool) (*jsonschema.Schema, error) {
	schema := &jsonschema.Schema{
		Type:                 "object",
		Properties:           map[string]*jsonschema.Schema{},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}

	declared := make(map[string]struct{}, len(t.params))
	for _, parameter := range t.params {
		declared[parameter.name] = struct{}{}

		property := *parameter.schema
		property.Description = parameter.description
		schema.Properties[parameter.name] = &property

		if slices.Contains(t.pathParams(), parameter.name) {
			schema.Required = append(schema.Required, parameter.name)
		}
	}

	for _, name := range t.pathParams() {
		if _, ok := declared[name]; !ok {
			return nil, fmt.Errorf("mcp: tool %q does not describe %q, which its path carries", t.name, name)
		}
	}

	if t.body != nil {
		for name, property := range t.body.Properties {
			if _, taken := schema.Properties[name]; taken {
				return nil, fmt.Errorf("mcp: tool %q describes %q twice", t.name, name)
			}

			schema.Properties[name] = property
		}

		schema.Required = append(schema.Required, t.body.Required...)
	}

	slices.Sort(schema.Required)

	return schema, nil
}

// snake is what a path's parameter is called to whoever fills it in: the rest
// of the API is written in snake case, and a caller should not have to know
// that one route spells a parameter differently.
func snake(name string) string {
	var out strings.Builder

	runes := []rune(name)
	for i, r := range runes {
		upper := r >= 'A' && r <= 'Z'
		if upper && i > 0 {
			previous := runes[i-1]
			next := ' '
			if i+1 < len(runes) {
				next = runes[i+1]
			}

			previousIsLower := previous >= 'a' && previous <= 'z' || previous >= '0' && previous <= '9'
			nextIsLower := next >= 'a' && next <= 'z'
			if previousIsLower || nextIsLower {
				out.WriteRune('_')
			}
		}

		if upper {
			r += 'a' - 'A'
		}

		out.WriteRune(r)
	}

	return out.String()
}

//go:fix inline
func ptr[T any](value T) *T {
	return new(value)
}
