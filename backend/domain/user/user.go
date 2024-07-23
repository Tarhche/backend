package user

import (
	"github.com/khanzadimahdi/testproject/domain/password"
)

type User struct {
	UUID         string
	Name         string
	Avatar       string
	Email        string
	Username     string
	PasswordHash password.Hash
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
