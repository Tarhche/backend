package main

import (
	"bytes"
	"testing"
)

func TestRender(t *testing.T) {
	article := Article{
		ID:    "test-id",
		Title: "test-title",
		Body:  "test-body",
	}

	t.Run("converts a single article to HTML", func(t *testing.T) {
		buf := bytes.Buffer{}
		renderer := SingleArticleRenderer{}

		if err := renderer.Render(&buf, article); err != nil {
			t.Fatal(err)
		}

		got := buf.String()
		want := "<h2>test-title</h2><p>test-body</p>"
		if got != want {
			t.Errorf("got '%s' want '%s'", got, want)
		}
	})
}
