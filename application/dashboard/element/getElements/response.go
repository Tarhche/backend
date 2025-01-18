package deleteelements

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/element"
)

type Response struct {
	Items      []elementResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type elementResponse struct {
	UUID      string   `json:"uuid"`
	Type      string   `json:"type"`
	Venues    []string `json:"venues"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(a []element.Element, totalPages, currentPage uint) *Response {
	items := make([]elementResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Type = a[i].Type
		items[i].Venues = a[i].Venues
		items[i].CreatedAt = a[i].CreatedAt.Format(time.RFC3339)
		items[i].UpdatedAt = a[i].UpdatedAt.Format(time.RFC3339)
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
