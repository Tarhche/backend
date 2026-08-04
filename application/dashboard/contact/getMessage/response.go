package getMessage

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/contact"
)

type Response struct {
	UUID      string `json:"uuid"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	ReadAt    string `json:"read_at"`
	CreatedAt string `json:"created_at"`
}

func NewResponse(m contact.Message) *Response {
	return &Response{
		UUID:      m.UUID,
		Subject:   m.Subject,
		Body:      m.Body,
		Email:     m.Email,
		Phone:     m.Phone,
		ReadAt:    m.ReadAt.Format(time.RFC3339),
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
	}
}
