package main

import (
	"bytes"
	approvals "github.com/approvals/go-approval-tests"
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

		approvals.VerifyString(t, buf.String())
	})

	t.Run("it renders an index of articles to HTML", func(t *testing.T) {
		buf := bytes.Buffer{}
		renderer := SingleArticleRenderer{}

		articles := []Article{
			{
				ID:    "test-id",
				Title: "test-title",
				Body:  "test-body",
			},
			{
				ID:    "test-id",
				Title: "test-title",
				Body:  "test-body",
			},
		}

		if err := renderer.RenderIndex(&buf, articles); err != nil {
			t.Fatal(err)
		}

		approvals.VerifyString(t, buf.String())
	})
}