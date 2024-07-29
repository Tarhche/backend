package getpermissions

import "github.com/khanzadimahdi/testproject/domain/permission"

type permissionResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Response struct {
	Items []permissionResponse `json:"items"`
}

func NewResponse(a []permission.Permission) *Response {
	items := make([]permissionResponse, len(a))

	for i := range a {
		items[i].Name = a[i].Name
		items[i].Value = a[i].Value
	}

	return &Response{
		Items: items,
	}
}
