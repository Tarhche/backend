package article

import (
	"embed"
	"github.com/Tarhche/backend/domain/article"
	"html/template"
	"io"
)

const (
	articleTemplate  = "article.tmpl"
	articlesTemplate = "articles.tmpl"
)

var (
	//go:embed "template/*"
	articleTemplates embed.FS
)

type HTMLArticleRenderer struct {
	tmpl *template.Template
}

func NewHTMLArticleRenderer() *HTMLArticleRenderer {
	return &HTMLArticleRenderer{
		tmpl: template.Must(template.ParseFS(articleTemplates, "template/*.tmpl")),
	}
}

func (a HTMLArticleRenderer) Render(buf io.Writer, article article.Entity) error {
	return a.tmpl.ExecuteTemplate(buf, articleTemplate, article)
}

func (a HTMLArticleRenderer) RenderIndex(buf io.Writer, articles []article.Entity) error {
	return a.tmpl.ExecuteTemplate(buf, articlesTemplate, articles)
}
