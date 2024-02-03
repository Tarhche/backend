package deleteelements

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/element"
)

type elementResponse struct {
	UUID      string    `json:"uuid"`
	Type      string    `json:"type"`
	Venues    []string  `json:"venues"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GetElementsResponse struct {
	Items      []elementResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewGetElementsReponse(a []element.Element, totalPages, currentPage uint) *GetElementsResponse {
	items := make([]elementResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Type = a[i].Type
		items[i].Venues = a[i].Venues
		items[i].CreatedAt = a[i].CreatedAt
		items[i].UpdatedAt = a[i].UpdatedAt
	}

	return &GetElementsResponse{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
