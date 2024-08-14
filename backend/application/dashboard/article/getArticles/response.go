package getarticles

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
)

type Response struct {
	Items      []articleResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type articleResponse struct {
	UUID        string    `json:"uuid"`
	Cover       string    `json:"cover"`
	Video       string    `json:"video"`
	Title       string    `json:"title"`
	PublishedAt time.Time `json:"published_at"`
	Author      author    `json:"author"`
}

type author struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(a []article.Article, totalPages, currentPage uint) *Response {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Video = a[i].Video
		items[i].Title = a[i].Title
		items[i].PublishedAt = a[i].PublishedAt

		items[i].Author.Name = a[i].Author.Name
		items[i].Author.Avatar = a[i].Author.Avatar
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
