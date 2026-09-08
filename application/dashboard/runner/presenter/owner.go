package presenter

import (
	"github.com/khanzadimahdi/testproject/domain/user"
)

// Owner is who a container or a stack belongs to, as the dashboard shows it.
//
// It is empty for a container that belongs to nobody: the code runner on the
// public pages starts one for whoever is reading, signed in or not, and an id
// that names no one names no one whether it was never set, set to a guest, or
// left behind by somebody who has since gone.
type Owner struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Username string `json:"username,omitempty"`
}

// Owners are the people behind a page of containers or stacks, so a listing
// asks who they are once rather than once per row.
type Owners map[string]user.User

func NewOwners(users []user.User) Owners {
	owners := make(Owners, len(users))
	for i := range users {
		owners[users[i].UUID] = users[i]
	}

	return owners
}

// Of is who that id belongs to, and nobody at all when it belongs to no one.
func (o Owners) Of(uuid string) Owner {
	u, ok := o[uuid]
	if !ok {
		return Owner{}
	}

	return Owner{
		UUID:     u.UUID,
		Name:     u.Name,
		Avatar:   u.Avatar,
		Username: u.Username,
	}
}
