package main

import (
	"bytes"
	approvals "github.com/approvals/go-approval-tests"
	"io"
	"testing"
)

func TestRender(t *testing.T) {
	article := Article{
		ID:    "test-id",
		Title: "test-title",
		Body:  "test-body",
	}

	t.Run("it renders a single article to HTML", func(t *testing.T) {
		buf := bytes.Buffer{}
		renderer := NewHTMLArticleRenderer()

		if err := renderer.Render(&buf, article); err != nil {
			t.Fatal(err)
		}

		approvals.VerifyString(t, buf.String())
	})

	t.Run("it renders an index of articles to HTML", func(t *testing.T) {
		buf := bytes.Buffer{}
		renderer := NewHTMLArticleRenderer()

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

func BenchmarkRender(b *testing.B) {
	b.Run("it renders a single article", func(b *testing.B) {
		renderer := NewHTMLArticleRenderer()

		article := Article{
			ID:    "test-id",
			Title: "test-title",
			Body:  "test-body",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			renderer.Render(io.Discard, article)
		}
	})

	b.Run("it renders an index of articles", func(b *testing.B) {
		renderer := NewHTMLArticleRenderer()

		articles := []Article{
			{
				ID:    "test-id",
				Title: "test-title",
				Body:  "test-body",
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			renderer.RenderIndex(io.Discard, articles)
		}
	})
}
