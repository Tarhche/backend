package getArticlesByHashtag

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Items      []articleResponse  `json:"items"`
	Pagination paginationResponse `json:"paginationResponse"`
}

type articleResponse struct {
	UUID        string         `json:"uuid"`
	Cover       string         `json:"cover"`
	Video       string         `json:"video"`
	Title       string         `json:"title"`
	Excerpt     string         `json:"excerpt"`
	PublishedAt string         `json:"published_at"`
	Author      authorResponse `json:"authorResponse"`
}

type authorResponse struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type paginationResponse struct {
	CurrentPage uint `json:"current_page"`
}

func NewResponse(a []article.Article, currentPage uint) *Response {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Video = a[i].Video
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

		items[i].Author.Name = a[i].Author.Name
		items[i].Author.Avatar = a[i].Author.Avatar
	}

	return &Response{
		Items: items,
		Pagination: paginationResponse{
			CurrentPage: currentPage,
		},
	}
}
