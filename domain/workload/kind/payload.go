package kind

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/khanzadimahdi/testproject/domain"
)

// Validatable is a payload that says what is wrong with it, field by field,
// in the codes the validator translates. It is domain.Validatable, which every
// request in the repository already is.
type Validatable = domain.Validatable

// Codec reads one action's payload.
//
// A payload is decoded once, at the edge of the service that receives it, so
// a strategy is handed the action's own struct rather than bytes, and never
// one that is wrong: what cannot be read is an error, and what was read and
// is not valid comes back field by field, for whoever asked to correct.
type Codec interface {
	// Decode reads a payload as it arrived: the action's value, or what is
	// wrong with it, or why it cannot be read at all. Nothing at all, or
	// null, is the zero value, which is then validated like any other.
	Decode(raw []byte) (value any, invalid domain.ValidationErrors, err error)

	// Type is the payload's Go type, which tool schemas and the OpenAPI
	// document are described from, or nil for an action asked with nothing.
	Type() reflect.Type
}

// Payload is the codec of an action asked with a P, whose pointer is
// Validatable as the repository's requests are. It decodes to a P, never to
// a pointer to one, so a strategy asserts payload.(P).
func Payload[P any, V interface {
	*P
	Validatable
}]() Codec {
	return payload[P, V]{}
}

type payload[P any, V interface {
	*P
	Validatable
}] struct{}

func (payload[P, V]) Decode(raw []byte) (any, domain.ValidationErrors, error) {
	var value P

	if !absent(raw) {
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
		}
	}

	if invalid := V(&value).Validate(); len(invalid) > 0 {
		return nil, invalid, nil
	}

	return value, nil, nil
}

func (payload[P, V]) Type() reflect.Type {
	return reflect.TypeFor[P]()
}

// NoPayload is the codec of an action asked with nothing. It takes nothing,
// null or an empty object, and refuses anything else, which is a caller that
// meant to say something the action would not hear.
var NoPayload Codec = noPayload{}

type noPayload struct{}

func (noPayload) Decode(raw []byte) (any, domain.ValidationErrors, error) {
	if absent(raw) {
		return nil, nil, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) > 0 {
		return nil, nil, fmt.Errorf("%w: the action is asked with nothing", ErrInvalidPayload)
	}

	return nil, nil, nil
}

func (noPayload) Type() reflect.Type {
	return nil
}

// absent reports whether raw says nothing at all.
func absent(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)

	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
