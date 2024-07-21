package role

type Role struct {
	UUID        string
	Name        string
	Description string
	Permissions []string
	UserUUIDs   []string
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]Role, error)
	GetOne(UUID string) (Role, error)
	Save(*Role) (uuid string, err error)
	Delete(UUID string) error
	Count() (uint, error)
	UserHasPermission(userUUID string, permission string) (bool, error)
}
