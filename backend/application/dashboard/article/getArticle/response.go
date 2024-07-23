package getarticle

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
)

type GetArticleResponse struct {
	UUID        string    `json:"uuid"`
	Cover       string    `json:"cover"`
	Video       string    `json:"video"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Author      author    `json:"avatar"`
	Tags        []string  `json:"tags"`
	ViewCount   uint      `json:"view_count"`
}

type author struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func NewGetArticleReponse(a article.Article) *GetArticleResponse {
	tags := make([]string, len(a.Tags))
	copy(tags, a.Tags)

	return &GetArticleResponse{
		UUID:        a.UUID,
		Cover:       a.Cover,
		Video:       a.Video,
		Title:       a.Title,
		Excerpt:     a.Excerpt,
		Body:        a.Body,
		PublishedAt: a.PublishedAt,
		Author: author{
			Name:   a.Author.Name,
			Avatar: a.Author.Avatar,
		},
		Tags:      tags,
		ViewCount: a.ViewCount,
	}
}
