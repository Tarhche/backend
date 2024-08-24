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

// global accesses
const (
	ArticlesIndex  = "articles.index"
	ArticlesCreate = "articles.create"
	ArticlesShow   = "articles.show"
	ArticlesUpdate = "articles.update"
	ArticlesDelete = "articles.delete"

	CommentsIndex  = "comments.index"
	CommentsCreate = "comments.create"
	CommentsShow   = "comments.show"
	CommentsUpdate = "comments.update"
	CommentsDelete = "comments.delete"

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

// user's self related accesses
const (
	SelfBookmarksIndex  = "self.bookmarks.index"
	SelfBookmarksDelete = "self.bookmarks.delete"

	SelfCommentsIndex  = "self.comments.index"
	SelfCommentsShow   = "self.comments.show"
	SelfCommentsUpdate = "self.comments.update"
	SelfCommentsDelete = "self.comments.delete"
)
