package getroles

import (
	"github.com/khanzadimahdi/testproject/domain/role"
)

type roleResponse struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Response struct {
	Items      []roleResponse `json:"items"`
	Pagination pagination     `json:"pagination"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(r []role.Role, totalPages, currentPage uint) *Response {
	items := make([]roleResponse, len(r))

	for i := range r {
		items[i].UUID = r[i].UUID
		items[i].Name = r[i].Name
		items[i].Description = r[i].Description
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
