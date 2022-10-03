package main

import (
	"html/template"
	"io"
)

const (
	postTemplate = "<h2>{{.Title}}<h2><p>{{.Body}}</p>"
)

type ArticleRenderer struct{}

func (a ArticleRenderer) Render(buf io.Writer, article Article) error {
	tmpl, err := template.New("article").Parse(postTemplate)
	if err != nil {
		return err
	}

	return tmpl.Execute(buf, article)
}
