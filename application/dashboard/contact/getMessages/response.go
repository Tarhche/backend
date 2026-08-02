package getMessages

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/contact"
)

type messageResponse struct {
	UUID      string `json:"uuid"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	ReadAt    string `json:"read_at"`
	CreatedAt string `json:"created_at"`
}

type Response struct {
	Items      []messageResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(m []contact.Message, totalPages, currentPage uint) *Response {
	items := make([]messageResponse, len(m))

	for i := range m {
		items[i].UUID = m[i].UUID
		items[i].Subject = m[i].Subject
		items[i].Body = m[i].Body
		items[i].Email = m[i].Email
		items[i].Phone = m[i].Phone
		items[i].ReadAt = m[i].ReadAt.Format(time.RFC3339)
		items[i].CreatedAt = m[i].CreatedAt.Format(time.RFC3339)
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
