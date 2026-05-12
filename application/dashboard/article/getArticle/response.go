package getarticle

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	UUID        string   `json:"uuid"`
	Cover       string   `json:"cover"`
	Video       string   `json:"video"`
	Title       string   `json:"title"`
	Excerpt     string   `json:"excerpt"`
	Body        string   `json:"body"`
	PublishedAt string   `json:"published_at"`
	Author      author   `json:"author"`
	Tags        []string `json:"tags"`
	ViewCount   uint     `json:"view_count"`
}

type author struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

func NewResponse(a article.Article, u user.User) *Response {
	tags := make([]string, len(a.Tags))
	copy(tags, a.Tags)

	return &Response{
		UUID:        a.UUID,
		Cover:       a.Cover,
		Video:       a.Video,
		Title:       a.Title,
		Excerpt:     a.Excerpt,
		Body:        a.Body,
		PublishedAt: a.PublishedAt.Format(time.RFC3339),
		Author: author{
			UUID:     u.UUID,
			Name:     u.Name,
			Avatar:   u.Avatar,
			Username: u.Username,
		},
		Tags:      tags,
		ViewCount: a.ViewCount,
	}
}
