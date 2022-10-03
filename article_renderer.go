package main

import (
	"embed"
	"html/template"
	"io"
)

const (
	singleArticleTemplate = "single-article.tmpl"
)

var (
	//go:embed "template/*"
	articleTemplates embed.FS
)

type SingleArticleRenderer struct{}

func (a SingleArticleRenderer) Render(buf io.Writer, article Article) error {
	tmpl, err := template.New(singleArticleTemplate).ParseFS(articleTemplates, "template/*.tmpl")
	if err != nil {
		return err
	}

	return tmpl.Execute(buf, article)
}
