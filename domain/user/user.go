package user

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/password"
)

type User struct {
	UUID         string
	Name         string
	Avatar       string
	Email        string
	Username     string
	LanguageCode string
	PasswordHash password.Hash
	Identities   []Identity
	CreatedAt    time.Time
	BannedAt     time.Time
}

// Identity is an account elsewhere that stands for this user: a Google account,
// a GitHub account. Somebody who signs in with one is the same person every
// time, whatever they later call themselves here, because the provider's own id
// for them is what is kept rather than the address they gave it.
type Identity struct {
	Provider string
	ID       string
}

// HasIdentity reports whether this user already signs in that way.
func (u User) HasIdentity(provider string, id string) bool {
	for _, identity := range u.Identities {
		if identity.Provider == provider && identity.ID == id {
			return true
		}
	}

	return false
}

// IsBanned reports whether the user's ban has taken effect.
func (u User) IsBanned() bool {
	return !u.BannedAt.IsZero() && !u.BannedAt.After(time.Now())
}

type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]User, error)
	GetByUUIDs(ctx context.Context, UUIDs []string) ([]User, error)
	GetOne(ctx context.Context, UUID string) (User, error)
	GetOneByIdentity(ctx context.Context, username string) (User, error)
	// GetOneByProviderIdentity is whoever signs in as that account elsewhere.
	GetOneByProviderIdentity(ctx context.Context, provider string, id string) (User, error)
	Save(ctx context.Context, u *User) (uuid string, err error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)
}
