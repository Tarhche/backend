package getComment

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	UUID       string `json:"uuid"`
	Body       string `json:"body"`
	Author     author `json:"author"`
	ParentUUID string `json:"parent_uuid,omitempty"`
	ObjectType string `json:"object_type"`
	ObjectUUID string `json:"object_uuid"`
	CreatedAt  string `json:"created_at"`
	ApprovedAt string `json:"approved_at"`
}

type author struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

func NewResponse(c comment.Comment, u user.User) *Response {
	return &Response{
		UUID: c.UUID,
		Body: c.Body,
		Author: author{
			UUID:     u.UUID,
			Name:     u.Name,
			Avatar:   u.Avatar,
			Username: u.Username,
		},
		ParentUUID: c.ParentUUID,
		ObjectType: c.ObjectType,
		ObjectUUID: c.ObjectUUID,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		ApprovedAt: c.ApprovedAt.Format(time.RFC3339),
	}
}
