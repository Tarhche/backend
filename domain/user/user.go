package user

import (
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
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]User, error)
	GetByUUIDs(UUIDs []string) ([]User, error)
	GetOne(UUID string) (User, error)
	GetOneByIdentity(username string) (User, error)
	Save(*User) (uuid string, err error)
	Delete(UUID string) error
	Count() (uint, error)
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
