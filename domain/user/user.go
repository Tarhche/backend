package user

type User struct {
	UUID     string
	Name     string
	Avatar   string
	Username string
	Password string
}

type Repository interface {
	GetOne(UUID string) (User, error)
	Save(*User) error
}
