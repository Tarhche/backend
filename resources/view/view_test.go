package view_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/template"
	"github.com/khanzadimahdi/testproject/resources/view"
)

// Every view is parsed alongside every other, and a block one of them defines
// answers for any that leaves the same name open. So a page written in one
// language can be drawn in another's — which is a thing to find out here
// rather than in somebody's inbox.
func TestViews_areDrawnInTheLanguageTheyAreWrittenIn(t *testing.T) {
	t.Parallel()

	for name, expected := range map[string]string{
		"mail/auth/register.en":       `<html lang="en" dir="ltr">`,
		"mail/auth/register.fa":       `<html lang="fa" dir="rtl">`,
		"mail/auth/reset-password.en": `<html lang="en" dir="ltr">`,
		"mail/auth/reset-password.fa": `<html lang="fa" dir="rtl">`,
		"runner/starting.en":          `<html lang="en" dir="ltr">`,
		"runner/starting.fa":          `<html lang="fa" dir="rtl">`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var drawn bytes.Buffer
			require.NoError(t, template.NewRenderer(view.Files, "tmpl").Render(&drawn, name, map[string]any{"Seconds": 2}))

			assert.True(t, strings.Contains(drawn.String(), expected),
				"%s should be drawn as %s", name, expected)
		})
	}
}
