package markAsRead

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/contact"
)

type Response struct {
	UUID   string `json:"uuid"`
	ReadAt string `json:"read_at"`
}

func NewResponse(m contact.Message) *Response {
	return &Response{
		UUID:   m.UUID,
		ReadAt: m.ReadAt.Format(time.RFC3339),
	}
}
