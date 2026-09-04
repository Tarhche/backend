package permission

import "context"

type Permission struct {
	Name  string
	Value string
}

type Repository interface {
	GetAll(ctx context.Context) []Permission
	Get(ctx context.Context, values []string) ([]Permission, error)
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

	ContactUsIndex      = "contactus.index"
	ContactUsShow       = "contactus.show"
	ContactUsDelete     = "contactus.delete"
	ContactUsMarkAsRead = "contactus.markAsRead"

	ConfigShow   = "config.show"
	ConfigUpdate = "config.update"

	LanguagesIndex  = "languages.index"
	LanguagesCreate = "languages.create"
	LanguagesShow   = "languages.show"
	LanguagesUpdate = "languages.update"
	LanguagesDelete = "languages.delete"

	RunnerContainersIndex  = "runner.containers.index"
	RunnerContainersCreate = "runner.containers.create"
	RunnerContainersShow   = "runner.containers.show"
	RunnerContainersDelete = "runner.containers.delete"
	RunnerContainersLogs   = "runner.containers.logs"

	// RunnerContainersManage covers stopping, killing and restarting. They are
	// one permission because they are one decision: whether somebody may
	// change what a container is doing.
	RunnerContainersManage = "runner.containers.manage"

	// RunnerContainersAttach is a shell inside somebody's container, which is
	// the strongest thing the dashboard offers, so it is never implied by any
	// of the others.
	RunnerContainersAttach = "runner.containers.attach"

	RunnerStacksIndex  = "runner.stacks.index"
	RunnerStacksCreate = "runner.stacks.create"
	RunnerStacksShow   = "runner.stacks.show"
	RunnerStacksDelete = "runner.stacks.delete"
	RunnerStacksManage = "runner.stacks.manage"
)

// user's self related accesses
const (
	SelfBookmarksIndex  = "self.bookmarks.index"
	SelfBookmarksDelete = "self.bookmarks.delete"

	SelfCommentsIndex  = "self.comments.index"
	SelfCommentsShow   = "self.comments.show"
	SelfCommentsUpdate = "self.comments.update"
	SelfCommentsDelete = "self.comments.delete"

	SelfFilesIndex  = "self.files.index"
	SelfFilesDelete = "self.files.delete"

	SelfArticlesIndex  = "self.articles.index"
	SelfArticlesShow   = "self.articles.show"
	SelfArticlesUpdate = "self.articles.update"
	SelfArticlesDelete = "self.articles.delete"

	SelfRunnerContainersIndex  = "self.runner.containers.index"
	SelfRunnerContainersShow   = "self.runner.containers.show"
	SelfRunnerContainersLogs   = "self.runner.containers.logs"
	SelfRunnerContainersManage = "self.runner.containers.manage"
	SelfRunnerContainersAttach = "self.runner.containers.attach"
	SelfRunnerContainersDelete = "self.runner.containers.delete"

	SelfRunnerStacksIndex  = "self.runner.stacks.index"
	SelfRunnerStacksShow   = "self.runner.stacks.show"
	SelfRunnerStacksManage = "self.runner.stacks.manage"
	SelfRunnerStacksDelete = "self.runner.stacks.delete"
)
