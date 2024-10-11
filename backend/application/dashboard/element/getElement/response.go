package getelement

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/element"
)

type Response struct {
	UUID      string   `json:"uuid"`
	Type      string   `json:"type"`
	Body      any      `json:"body"`
	Venues    []string `json:"venues"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func NewResponse(e element.Element) *Response {
	return &Response{
		UUID:      e.UUID,
		Type:      e.Type,
		Body:      e.Body,
		Venues:    e.Venues,
		CreatedAt: e.CreatedAt.Format(time.RFC3339),
		UpdatedAt: e.UpdatedAt.Format(time.RFC3339),
	}
}
