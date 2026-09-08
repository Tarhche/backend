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

// A template is parsed alongside every other, and which one is asked for
// decides only whose definitions win. One of them is parsed last to arrange
// that, and the one it changes places with has to stay in the set: a layout
// that sorts after the page using it is still a layout that page needs.
func TestRenderer_keepsEveryTemplateInTheSet(t *testing.T) {
	renderer := NewRenderer(os.DirFS("testdata"), "tmpl")

	var buffer bytes.Buffer
	err := renderer.Render(&buffer, "uses-last", map[string]string{"body": "test body"})

	assert.NoError(t, err)
	assert.Contains(t, buffer.String(), "<footer>test body</footer>")
}
