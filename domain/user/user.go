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
	CreatedAt    time.Time
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
