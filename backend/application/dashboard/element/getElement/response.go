package getelement

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/element"
)

type Response struct {
	UUID      string    `json:"uuid"`
	Type      string    `json:"type"`
	Body      any       `json:"body"`
	Venues    []string  `json:"venues"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewResponse(e element.Element) *Response {
	return &Response{
		UUID:      e.UUID,
		Type:      e.Type,
		Body:      e.Body,
		Venues:    e.Venues,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}
