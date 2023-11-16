package getarticle

import (
	"time"

	"github.com/khanzadimahdi/testproject.git/domain/article"
)

type GetArticleResponse struct {
	UUID        string    `json:"uuid"`
	Cover       string    `json:"cover"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Author      author    `json:"avatar"`
}

type author struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func NewGetArticleReponse(a article.Article) *GetArticleResponse {
	return &GetArticleResponse{
		UUID:        a.UUID,
		Cover:       a.Cover,
		Title:       a.Title,
		Body:        a.Body,
		PublishedAt: a.PublishedAt,
		Author: author{
			Name:   a.Author.Name,
			Avatar: a.Author.Avatar,
		},
	}
}
