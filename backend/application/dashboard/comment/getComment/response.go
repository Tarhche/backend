package getComment

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/comment"
)

type Response struct {
	UUID       string    `json:"uuid"`
	Body       string    `json:"body"`
	Author     author    `json:"author"`
	ParentUUID string    `json:"parent_uuid,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ApprovedAt time.Time `json:"approved_at"`
}

type author struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func NewResponse(c comment.Comment) *Response {
	return &Response{
		UUID: c.UUID,
		Body: c.Body,
		Author: author{
			UUID:   c.Author.UUID,
			Name:   c.Author.Name,
			Avatar: c.Author.Avatar,
		},
		ParentUUID: c.ParentUUID,
		CreatedAt:  c.CreatedAt,
		ApprovedAt: c.ApprovedAt,
	}
}
