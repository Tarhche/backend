package getarticles

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	Items      []articleResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type articleResponse struct {
	UUID        string `json:"uuid"`
	Cover       string `json:"cover"`
	Video       string `json:"video"`
	Title       string `json:"title"`
	PublishedAt string `json:"published_at"`
	Author      author `json:"author"`
}

type author struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(a []article.Article, authors []user.User, totalPages, currentPage uint) *Response {
	authorByUUID := make(map[string]user.User, len(authors))
	for i := range authors {
		authorByUUID[authors[i].UUID] = authors[i]
	}

	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Video = a[i].Video
		items[i].Title = a[i].Title
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

		if u, ok := authorByUUID[a[i].AuthorUUID]; ok {
			items[i].Author.UUID = u.UUID
			items[i].Author.Name = u.Name
			items[i].Author.Avatar = u.Avatar
			items[i].Author.Username = u.Username
		}
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
