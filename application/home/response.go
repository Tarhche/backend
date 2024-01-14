package home

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
)

type Response struct {
	Featured []articleResponse `json:"featured,omitempty"`
	All      []articleResponse `json:"all,omitempty"`
	Popular  []articleResponse `json:"popular,omitempty"`
}

type articleResponse struct {
	UUID        string    `json:"uuid"`
	Cover       string    `json:"cover"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Author      author    `json:"author"`
	PublishedAt time.Time `json:"published_at"`
	Excerpt     string    `json:"excerpt"`
	Tags        []string  `json:"tags"`
}

type author struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func NewResponse(featured, popular, all []article.Article) *Response {
	return &Response{
		Featured: toArticleResponse(featured),
		Popular:  toArticleResponse(popular),
		All:      toArticleResponse(all),
	}
}

func toArticleResponse(a []article.Article) []articleResponse {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Title = a[i].Title
		items[i].Body = a[i].Body
		items[i].PublishedAt = a[i].PublishedAt

		items[i].Author.Name = a[i].Author.Name
		items[i].Author.Avatar = a[i].Author.Avatar
	}

	return items
}
