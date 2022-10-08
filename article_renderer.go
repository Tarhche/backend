package main

import (
	"embed"
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

type ArticleRenderer struct {
	tmpl *template.Template
}

func NewArticleRenderer() *ArticleRenderer {
	return &ArticleRenderer{
		tmpl: template.Must(template.ParseFS(articleTemplates, "template/*.tmpl")),
	}
}

func (a ArticleRenderer) Render(buf io.Writer, article Article) error {
	return a.tmpl.ExecuteTemplate(buf, articleTemplate, article)
}

func (a ArticleRenderer) RenderIndex(buf io.Writer, articles []Article) error {
	return a.tmpl.ExecuteTemplate(buf, articlesTemplate, articles)
}
