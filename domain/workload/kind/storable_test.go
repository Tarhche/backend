package kind

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyable(t *testing.T) {
	t.Parallel()

	for key, keyable := range map[string]bool{
		"com.example.tier": true,
		"$date":            true,
		"":                 true,
		"a\x00b":           false,
		"\x00":             false,
	} {
		assert.Equal(t, keyable, Keyable(key), "%q", key)
	}
}

func TestValidLabels(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		labels map[string]string
		valid  bool
	}{
		"none":                         {valid: true},
		"of whatever keys and values":  {labels: map[string]string{"com.example.tier": "web", "$numberLong": "5", "": ""}, valid: true},
		"but a key with a NUL in it":   {labels: map[string]string{"tier": "web", "a\x00b": "c"}},
		"or a value, held to the same": {labels: map[string]string{"tier": "w\x00eb"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.valid, ValidLabels(tt.labels))
		})
	}
}
