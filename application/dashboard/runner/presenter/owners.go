package presenter

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/user"
)

// Directory is who the runner's tasks and stacks belong to.
//
// The runner keeps the id of whoever asked for a task, and nothing else
// about them: a name to show beside it lives with the users. This is what puts
// the two together, a page's worth at a time.
type Directory struct {
	users user.Repository
}

func NewDirectory(users user.Repository) *Directory {
	return &Directory{users: users}
}

// Of looks up the people behind the given ids, each of them once.
//
// An id it cannot place is left out rather than refused: a task the code
// runner started belongs to nobody in particular, and one whose owner has since
// been deleted is still a task to show.
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
