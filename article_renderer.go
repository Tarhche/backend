package main

import (
	"fmt"
	"io"
)

type ArticleRenderer struct{}

func (a ArticleRenderer) Render(buf io.Writer, article Article) error {
	_, err := fmt.Fprintf(buf, "<h2>%s<h2><p>%s</p>", article.Title, article.Body)

	return err
}
