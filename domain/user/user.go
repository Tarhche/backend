package user

import (
	"context"
	"regexp"
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
	CreatedAt    time.Time
	// BannedAt is the moment an administrator banned the user. The zero value
	// means the account is in good standing.
	BannedAt time.Time
}

// IsBanned reports whether the user is banned and must be kept out of the
// application.
func (u User) IsBanned() bool {
	return !u.BannedAt.IsZero()
}

// SetBanned bans or lifts the ban on the user. Banning keeps the moment the ban
// started, so saving an already banned user doesn't move the date.
func (u *User) SetBanned(banned bool) {
	switch {
	case !banned:
		u.BannedAt = time.Time{}
	case !u.IsBanned():
		u.BannedAt = time.Now()
	}
}

type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]User, error)
	GetByUUIDs(ctx context.Context, UUIDs []string) ([]User, error)
	GetOne(ctx context.Context, UUID string) (User, error)
	GetOneByIdentity(ctx context.Context, username string) (User, error)
	Save(ctx context.Context, u *User) (uuid string, err error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)
}

var (
	usernameRegex = regexp.MustCompile(`^[a-z0-9._-]*[a-z0-9][a-z0-9._-]*$`)
	emailRegex    = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
)

// IsValidUsername reports whether s is a valid username: lowercase English
// letters, digits, dots, dashes and underscores only, with at least one
// alphanumeric character.
func IsValidUsername(s string) bool {
	return usernameRegex.MatchString(s)
}

// IsValidEmail reports whether s is a syntactically valid email address.
func IsValidEmail(s string) bool {
	return emailRegex.MatchString(s)
}
