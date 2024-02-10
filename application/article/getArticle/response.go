package getarticle

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
)

type GetArticleResponse struct {
	UUID        string            `json:"uuid"`
	Cover       string            `json:"cover"`
	Title       string            `json:"title"`
	Excerpt     string            `json:"excerpt"`
	Body        string            `json:"body"`
	PublishedAt time.Time         `json:"published_at"`
	Author      authorResponse    `json:"avatar"`
	Tags        []string          `json:"tags"`
	ViewCount   uint              `json:"view_count"`
	Elements    []elementResponse `json:"elements"`
}

type elementResponse struct {
	Type string `json:"type"`
	Body any    `json:"body"`
}

type authorResponse struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func NewGetArticleReponse(a article.Article, e []element.Element) *GetArticleResponse {
	tags := make([]string, len(a.Tags))
	copy(tags, a.Tags)

	elements := make([]elementResponse, len(e))
	for i := range e {
		elements[i] = elementResponse{
			Type: e[i].Type,
			Body: e[i].Body,
		}
	}

	return &GetArticleResponse{
		UUID:        a.UUID,
		Cover:       a.Cover,
		Title:       a.Title,
		Excerpt:     a.Excerpt,
		Body:        a.Body,
		PublishedAt: a.PublishedAt,
		Author: authorResponse{
			Name:   a.Author.Name,
			Avatar: a.Author.Avatar,
		},
		Tags:      tags,
		ViewCount: a.ViewCount,
		Elements:  elements,
	}
}
