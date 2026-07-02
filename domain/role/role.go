package role

import "context"

type Role struct {
	UUID        string
	Name        string
	Description string
	Permissions []string
	UserUUIDs   []string
}

type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Role, error)
	GetByUUIDs(ctx context.Context, UUIDs []string) ([]Role, error)
	GetOne(ctx context.Context, UUID string) (Role, error)
	Save(ctx context.Context, r *Role) (uuid string, err error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)
	UserHasPermission(ctx context.Context, userUUID string, permission string) (bool, error)
	GetByUserUUID(ctx context.Context, userUUID string) ([]Role, error)
}
