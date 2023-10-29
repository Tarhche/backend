package getarticle

import (
	"time"

	"github.com/khanzadimahdi/testproject.git/domain/article"
)

type GetArticleResponse struct {
	UUID        string
	Cover       string
	Title       string
	Body        string
	PublishedAt time.Time
	Author      struct {
		Name   string
		Avatar string
	}
}

func NewGetArticleReponse(a article.Article) *GetArticleResponse {
	return &GetArticleResponse{
		UUID:        a.UUID,
		Cover:       a.Cover,
		Title:       a.Title,
		Body:        a.Body,
		PublishedAt: a.PublishedAt,
		Author: struct {
			Name   string
			Avatar string
		}{
			Name:   a.Author.Name,
			Avatar: a.Author.Avatar,
		},
	}
}
