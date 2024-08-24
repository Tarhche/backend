package getUserBookmarks

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

type Response struct {
	Items      []bookmarkResponse `json:"items"`
	Pagination pagination         `json:"pagination"`
}

type bookmarkResponse struct {
	ObjectUUID string    `json:"uuid"`
	ObjectType string    `json:"cover"`
	CreatedAt  time.Time `json:"video"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(b []bookmark.Bookmark, totalPages, currentPage uint) *Response {
	items := make([]bookmarkResponse, len(b))

	for i := range b {
		items[i].ObjectUUID = b[i].ObjectUUID
		items[i].ObjectType = b[i].ObjectType
		items[i].CreatedAt = b[i].CreatedAt
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
