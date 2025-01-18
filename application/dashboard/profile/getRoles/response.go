package getRoles

import (
	"github.com/khanzadimahdi/testproject/domain/role"
)

type Response struct {
	Items []roleResponse `json:"items"`
}

type roleResponse struct {
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func NewResponse(r []role.Role) *Response {
	items := make([]roleResponse, len(r))

	for i := range r {
		items[i].UUID = r[i].UUID
		items[i].Name = r[i].Name
		items[i].Description = r[i].Description

		items[i].Permissions = make([]string, len(r[i].Permissions))
		for j := range r[i].Permissions {
			items[i].Permissions[j] = r[i].Permissions[j]
		}
	}

	return &Response{
		Items: items,
	}
}
