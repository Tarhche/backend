package getArticlesByAuthor

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Author     authorResponse     `json:"author"`
	Items      []articleResponse  `json:"items"`
	Pagination paginationResponse `json:"pagination"`
}

type articleResponse struct {
	UUID        string `json:"uuid"`
	Cover       string `json:"cover"`
	Video       string `json:"video"`
	Title       string `json:"title"`
	Excerpt     string `json:"excerpt"`
	PublishedAt string `json:"published_at"`
}

type authorResponse struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

type paginationResponse struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(author user.User, a []article.Article, totalPages, currentPage uint) *Response {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Video = a[i].Video
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)
	}

	return &Response{
		Author: authorResponse{
			UUID:      author.UUID,
			Name:      author.Name,
			Avatar:    author.Avatar,
			Username:  author.Username,
			CreatedAt: author.CreatedAt.Format(time.RFC3339),
		},
		Items: items,
		Pagination: paginationResponse{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
