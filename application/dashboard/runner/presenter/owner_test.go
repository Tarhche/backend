package presenter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/user"
)

func TestOwners_Of(t *testing.T) {
	t.Parallel()

	owners := NewOwners([]user.User{{UUID: "owner-uuid", Name: "Mahdi", Username: "mahdi", Avatar: "avatar-uuid"}})

	t.Run("somebody the dashboard has", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, Owner{
			UUID:     "owner-uuid",
			Name:     "Mahdi",
			Username: "mahdi",
			Avatar:   "avatar-uuid",
		}, owners.Of("owner-uuid"))
	})

	t.Run("an id that names nobody is nobody", func(t *testing.T) {
		t.Parallel()

		// what the code runner puts on the containers it starts for whoever is
		// reading a page, and what is left of an owner who has since gone.
		assert.Equal(t, Owner{}, owners.Of("guest"))
		assert.Equal(t, Owner{}, owners.Of(""))
	})
}
