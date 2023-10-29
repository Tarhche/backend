package getarticles

import (
	"time"

	"github.com/khanzadimahdi/testproject.git/domain/article"
)

type articleResponse struct {
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

type GetArticlesResponse struct {
	Items      []articleResponse
	Pagination struct {
		TotalPages  uint
		CurrentPage uint
	}
}

func NewGetArticlesReponse(a []article.Article, totalPages, currentPage uint) *GetArticlesResponse {
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

	return &GetArticlesResponse{
		Items: items,
		Pagination: struct {
			TotalPages  uint
			CurrentPage uint
		}{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
