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
	Title      string    `json:"title"`
	ObjectUUID string    `json:"object_uuid"`
	ObjectType string    `json:"object_type"`
	CreatedAt  time.Time `json:"created_at"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(b []bookmark.Bookmark, totalPages, currentPage uint) *Response {
	items := make([]bookmarkResponse, len(b))

	for i := range b {
		items[i].Title = b[i].Title
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
