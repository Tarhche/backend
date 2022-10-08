package article

import (
	"bytes"
	"github.com/Tarhche/backend/domain/article"
	approvals "github.com/approvals/go-approval-tests"
	"io"
	"testing"
)

func TestRender(t *testing.T) {
	anArticle := article.Entity{
		ID:    "test-id",
		Title: "test-title",
		Body:  "test-body",
	}

	t.Run("it renders a single article to HTML", func(t *testing.T) {
		buf := bytes.Buffer{}
		renderer := NewHTMLArticleRenderer()

		if err := renderer.Render(&buf, anArticle); err != nil {
			t.Fatal(err)
		}

		approvals.VerifyString(t, buf.String())
	})

	t.Run("it renders an index of articles to HTML", func(t *testing.T) {
		buf := bytes.Buffer{}
		renderer := NewHTMLArticleRenderer()

		articles := []article.Entity{
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

		article := article.Entity{
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

		articles := []article.Entity{
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
