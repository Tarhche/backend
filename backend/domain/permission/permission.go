package permission

type Permission struct {
	Name  string
	Value string
}

type Repository interface {
	GetAll() []Permission
	GetOne(value string) (Permission, error)
	Get(values []string) ([]Permission, error)
}

const (
	ArticlesIndex  = "articles.index"
	ArticlesCreate = "articles.create"
	ArticlesShow   = "articles.show"
	ArticlesUpdate = "articles.update"
	ArticlesDelete = "articles.delete"

	ElementsIndex  = "elements.index"
	ElementsCreate = "elements.create"
	ElementsShow   = "elements.show"
	ElementsUpdate = "elements.update"
	ElementsDelete = "elements.delete"

	FilesIndex  = "files.index"
	FilesCreate = "files.create"
	FilesShow   = "files.show"
	FilesDelete = "files.delete"

	UsersIndex          = "users.index"
	UsersCreate         = "users.create"
	UsersShow           = "users.show"
	UsersUpdate         = "users.update"
	UsersDelete         = "users.delete"
	UsersPasswordUpdate = "users.password.update"

	PermissionsIndex = "permissions.index"

	RolesIndex  = "roles.index"
	RolesCreate = "roles.create"
	RolesShow   = "roles.show"
	RolesUpdate = "roles.update"
	RolesDelete = "roles.delete"
)
