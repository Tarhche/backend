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

	// UsersImpersonate is permission to be seen as somebody else: the dashboard
	// opens a session that acts as them, saying who is behind it.
	UsersImpersonate = "users.impersonate"

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

	WorkloadTasksIndex  = "workload.tasks.index"
	WorkloadTasksCreate = "workload.tasks.create"
	WorkloadTasksShow   = "workload.tasks.show"
	WorkloadTasksDelete = "workload.tasks.delete"
	WorkloadTasksLogs   = "workload.tasks.logs"
	WorkloadTasksAttach = "workload.tasks.attach"

	// WorkloadTasksManage covers stopping, killing and restarting.
	WorkloadTasksManage = "workload.tasks.manage"

	WorkloadStacksIndex  = "workload.stacks.index"
	WorkloadStacksCreate = "workload.stacks.create"
	WorkloadStacksShow   = "workload.stacks.show"
	WorkloadStacksDelete = "workload.stacks.delete"
	WorkloadStacksManage = "workload.stacks.manage"
)

// user's self related accesses
const (
	SelfBookmarksIndex  = "self.bookmarks.index"
	SelfBookmarksDelete = "self.bookmarks.delete"

	SelfCommentsIndex  = "self.comments.index"
	SelfCommentsShow   = "self.comments.show"
	SelfCommentsUpdate = "self.comments.update"
	SelfCommentsDelete = "self.comments.delete"

	SelfArticlesIndex  = "self.articles.index"
	SelfArticlesShow   = "self.articles.show"
	SelfArticlesUpdate = "self.articles.update"
	SelfArticlesDelete = "self.articles.delete"

	SelfFilesIndex  = "self.files.index"
	SelfFilesDelete = "self.files.delete"

	SelfWorkloadTasksIndex  = "self.workload.tasks.index"
	SelfWorkloadTasksShow   = "self.workload.tasks.show"
	SelfWorkloadTasksLogs   = "self.workload.tasks.logs"
	SelfWorkloadTasksManage = "self.workload.tasks.manage"
	SelfWorkloadTasksAttach = "self.workload.tasks.attach"
	SelfWorkloadTasksDelete = "self.workload.tasks.delete"

	SelfWorkloadStacksIndex  = "self.workload.stacks.index"
	SelfWorkloadStacksShow   = "self.workload.stacks.show"
	SelfWorkloadStacksManage = "self.workload.stacks.manage"
	SelfWorkloadStacksDelete = "self.workload.stacks.delete"
)
