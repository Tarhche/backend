package getArticlesByHashtag

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
)

type articleResponse struct {
	UUID        string    `json:"uuid"`
	Cover       string    `json:"cover"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	PublishedAt time.Time `json:"published_at"`
	Author      author    `json:"author"`
}

type GetArticlesByHashtagResponse struct {
	Items      []articleResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type author struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type pagination struct {
	CurrentPage uint `json:"current_page"`
}

func NewGetArticlesByHashtagReponse(a []article.Article, currentPage uint) *GetArticlesByHashtagResponse {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].PublishedAt = a[i].PublishedAt

		items[i].Author.Name = a[i].Author.Name
		items[i].Author.Avatar = a[i].Author.Avatar
	}

	return &GetArticlesByHashtagResponse{
		Items: items,
		Pagination: pagination{
			CurrentPage: currentPage,
		},
	}
}
