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

type ArticleRenderer interface {
	Render(io.Writer, Article) error
	RenderIndex(io.Writer, []Article) error
}

type HTMLArticleRenderer struct {
	tmpl *template.Template
}

func NewHTMLArticleRenderer() *HTMLArticleRenderer {
	return &HTMLArticleRenderer{
		tmpl: template.Must(template.ParseFS(articleTemplates, "template/*.tmpl")),
	}
}

func (a HTMLArticleRenderer) Render(buf io.Writer, article Article) error {
	return a.tmpl.ExecuteTemplate(buf, articleTemplate, article)
}

func (a HTMLArticleRenderer) RenderIndex(buf io.Writer, articles []Article) error {
	return a.tmpl.ExecuteTemplate(buf, articlesTemplate, articles)
}
