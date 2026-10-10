package kind

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestPayload(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		codec   Codec
		raw     string
		value   any
		invalid domain.ValidationErrors
		err     bool
	}{
		"a payload valid by value": {
			codec: Payload[resizePayload](),
			raw:   `{"size": 3}`,
			value: resizePayload{Size: 3},
		},
		"and one valid by pointer, decoded to a value all the same": {
			codec: Payload[logsPayload](),
			raw:   `{"tail": 20}`,
			value: logsPayload{Tail: 20},
		},
		"what it does not know is not its business": {
			codec: Payload[logsPayload](),
			raw:   `{"tail": 20, "follow": true}`,
			value: logsPayload{Tail: 20},
		},
		"nothing at all is the zero value": {
			codec: Payload[logsPayload](),
			raw:   ``,
			value: logsPayload{},
		},
		"and so is null": {
			codec: Payload[logsPayload](),
			raw:   ` null `,
			value: logsPayload{},
		},
		"which is validated like any other": {
			codec:   Payload[resizePayload](),
			raw:     ``,
			invalid: domain.ValidationErrors{"size": "required_field"},
		},
		"a payload that is not valid says what is wrong with it": {
			codec:   Payload[logsPayload](),
			raw:     `{"tail": 5000}`,
			invalid: domain.ValidationErrors{"tail": "too_many"},
		},
		"one that is not its shape cannot be read": {
			codec: Payload[logsPayload](),
			raw:   `{"tail": "all"}`,
			err:   true,
		},
		"nor can one that is not json": {
			codec: Payload[resizePayload](),
			raw:   `size=3`,
			err:   true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			value, invalid, err := tt.codec.Decode([]byte(tt.raw))

			if tt.err {
				assert.ErrorIs(t, err, ErrInvalidPayload)
				assert.Nil(t, value)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.invalid, invalid)
			assert.Equal(t, tt.value, value)
		})
	}

	t.Run("its type is the payload's, never a pointer to it", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, reflect.TypeFor[resizePayload](), Payload[resizePayload]().Type())
		assert.Equal(t, reflect.TypeFor[logsPayload](), Payload[logsPayload]().Type())
	})
}

func TestNoPayload(t *testing.T) {
	t.Parallel()

	for raw, refused := range map[string]bool{
		``:            false,
		`null`:        false,
		`{}`:          false,
		` { } `:       false,
		`{"size": 3}`: true,
		`[]`:          true,
		`"something"`: true,
		`not json`:    true,
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			value, invalid, err := NoPayload.Decode([]byte(raw))

			assert.Nil(t, value)
			assert.Nil(t, invalid)

			if refused {
				assert.ErrorIs(t, err, ErrInvalidPayload)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	t.Run("it has no type", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, NoPayload.Type())
	})
}

func TestInvalidError(t *testing.T) {
	t.Parallel()

	err := &InvalidError{Errors: domain.ValidationErrors{"tail": "too_many", "size": "required_field"}}

	assert.Equal(t, "invalid payload: size: required_field, tail: too_many", err.Error(), "fields in an order that does not change")
	assert.ErrorIs(t, err, ErrInvalidPayload)
	assert.False(t, errors.Is(err, ErrUnknownAction))
}
