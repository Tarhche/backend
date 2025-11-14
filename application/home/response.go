package home

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain/article"
)

type Response struct {
	All      []articleResponse  `json:"all"`
	Popular  []articleResponse  `json:"popular"`
	Elements []element.Response `json:"elements"`
}

type articleResponse struct {
	UUID        string   `json:"uuid"`
	Cover       string   `json:"cover"`
	Title       string   `json:"title"`
	Author      author   `json:"author"`
	PublishedAt string   `json:"published_at"`
	Excerpt     string   `json:"excerpt"`
	Tags        []string `json:"tags"`
}

type author struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func NewResponse(all, popular []article.Article, elementsResponse []element.Response) *Response {
	return &Response{
		All:      toArticleResponse(all),
		Popular:  toArticleResponse(popular),
		Elements: elementsResponse,
	}
}

func toArticleResponse(a []article.Article) []articleResponse {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].Tags = a[i].Tags
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

		items[i].Author.Name = a[i].Author.Name
		items[i].Author.Avatar = a[i].Author.Avatar
	}

	return items
}
