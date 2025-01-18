package template

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderer(t *testing.T) {
	expected, err := os.ReadFile("testdata/page.txt")
	assert.NoError(t, err)

	fs := os.DirFS("testdata")
	extension := "tmpl"

	renderer := NewRenderer(fs, extension)

	var buffer bytes.Buffer
	err = renderer.Render(&buffer, "page", map[string]string{
		"head": "test head",
		"body": "test body",
	})

	assert.NoError(t, err)
	assert.Equal(t, string(expected), buffer.String())
}
