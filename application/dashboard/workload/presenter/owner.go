// Package presenter turns what the workload reports into what the dashboard
// shows. The shapes live in one place because a VM, a container or a stack
// looks the same whether it is listed, shown, created or changed, and whether
// it is asked for as anybody's or as one's own.
package presenter

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/user"
)

// Owner is who a VM, a snapshot or a stack belongs to, as the dashboard shows
// them beside it. Its uuid is the owner_uuid the record carries anyway; the
// rest is what puts a face to it.
type Owner struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Username string `json:"username,omitempty"`
}

// Owners are the people behind a page of records, so a listing asks who they
// are once rather than once per row.
type Owners map[string]user.User

func NewOwners(users []user.User) Owners {
	owners := make(Owners, len(users))
	for i := range users {
		owners[users[i].UUID] = users[i]
	}

	return owners
}

// Of is who that id belongs to, and nobody at all when it names no one the
// dashboard has: somebody who has since gone still leaves their records, and
// the owner_uuid beside them is all there is to say.
func (o Owners) Of(uuid string) *Owner {
	u, ok := o[uuid]
	if !ok {
		return nil
	}

	return &Owner{
		UUID:     u.UUID,
		Name:     u.Name,
		Avatar:   u.Avatar,
		Username: u.Username,
	}
}

// Directory is who the workload's records belong to.
//
// The workload keeps the id of whoever asked for a VM, and nothing else about
// them: a name to show beside it lives with the users. This is what puts the
// two together, a page's worth at a time.
type Directory struct {
	users user.Repository
}

func NewDirectory(users user.Repository) *Directory {
	return &Directory{users: users}
}

// Of looks up the people behind the given ids, each of them once. An id it
// cannot place is left out rather than refused.
func (d *Directory) Of(ctx context.Context, uuids ...string) (Owners, error) {
	wanted := make([]string, 0, len(uuids))
	seen := make(map[string]struct{}, len(uuids))

	for _, uuid := range uuids {
		if len(uuid) == 0 {
			continue
		}

		if _, asked := seen[uuid]; asked {
			continue
		}

		seen[uuid] = struct{}{}
		wanted = append(wanted, uuid)
	}

	if len(wanted) == 0 {
		return NewOwners(nil), nil
	}

	users, err := d.users.GetByUUIDs(ctx, wanted)
	if err != nil {
		return nil, err
	}

	return NewOwners(users), nil
}
