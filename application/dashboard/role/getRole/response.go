package getrole

import (
	"github.com/khanzadimahdi/testproject/domain/role"
)

type Response struct {
	UUID        string   `json:"uuid"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	UserUUIDs   []string `json:"user_uuids"`
}

func NewResponse(a role.Role) *Response {
	response := Response{
		UUID:        a.UUID,
		Name:        a.Name,
		Description: a.Description,
		Permissions: make([]string, len(a.Permissions)),
		UserUUIDs:   make([]string, len(a.UserUUIDs)),
	}

	if len(a.Permissions) > 0 {
		response.Permissions = a.Permissions
	}

	if len(a.UserUUIDs) > 0 {
		response.UserUUIDs = a.UserUUIDs
	}

	return &response
}
