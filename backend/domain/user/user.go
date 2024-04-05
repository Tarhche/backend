package user

import "github.com/khanzadimahdi/testproject/domain/password"

type User struct {
	UUID         string
	Name         string
	Avatar       string
	Email        string
	Username     string
	PasswordHash password.Hash
}

type Repository interface {
	GetOne(UUID string) (User, error)
	GetOneByIdentity(username string) (User, error)
	Save(*User) error
}
