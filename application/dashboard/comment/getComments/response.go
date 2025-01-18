package getComments

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/comment"
)

type commentResponse struct {
	UUID       string         `json:"uuid"`
	Body       string         `json:"body"`
	Author     authorResponse `json:"author"`
	ParentUUID string         `json:"parent_uuid,omitempty"`
	ObjectType string         `json:"object_type"`
	ObjectUUID string         `json:"object_uuid"`
	ApprovedAt string         `json:"approved_at,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

type authorResponse struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type Response struct {
	Items      []commentResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(c []comment.Comment, totalPages, currentPage uint) *Response {
	items := make([]commentResponse, len(c))

	for i := range c {
		items[i].UUID = c[i].UUID
		items[i].Body = c[i].Body
		items[i].ParentUUID = c[i].ParentUUID
		items[i].ObjectType = c[i].ObjectType
		items[i].ObjectUUID = c[i].ObjectUUID
		items[i].CreatedAt = c[i].CreatedAt.Format(time.RFC3339)
		items[i].ApprovedAt = c[i].ApprovedAt.Format(time.RFC3339)

		items[i].Author = authorResponse{
			UUID:   c[i].Author.UUID,
			Name:   c[i].Author.Name,
			Avatar: c[i].Author.Avatar,
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
