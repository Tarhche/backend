package getRoles

import "github.com/khanzadimahdi/testproject/domain/role"

type roleResponse struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Response struct {
	Items []roleResponse `json:"items"`
}

func NewResponse(r []role.Role) *Response {
	items := make([]roleResponse, len(r))

	for i := range r {
		items[i].UUID = r[i].UUID
		items[i].Name = r[i].Name
		items[i].Description = r[i].Description
	}

	return &Response{
		Items: items,
	}
}
