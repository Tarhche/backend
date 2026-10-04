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

	// The workload's permissions over anybody's VMs, snapshots, containers and
	// stacks. Create is only ever in this set, and creates for whoever asks.
	WorkloadVMsIndex  = "workload.vms.index"
	WorkloadVMsCreate = "workload.vms.create"
	WorkloadVMsShow   = "workload.vms.show"
	WorkloadVMsUpdate = "workload.vms.update"
	WorkloadVMsDelete = "workload.vms.delete"
	WorkloadVMsLogs   = "workload.vms.logs"
	WorkloadVMsAttach = "workload.vms.attach"

	// WorkloadVMsManage covers starting, stopping, restarting and restoring.
	WorkloadVMsManage = "workload.vms.manage"

	WorkloadSnapshotsIndex  = "workload.snapshots.index"
	WorkloadSnapshotsCreate = "workload.snapshots.create"
	WorkloadSnapshotsShow   = "workload.snapshots.show"
	WorkloadSnapshotsUpdate = "workload.snapshots.update"
	WorkloadSnapshotsDelete = "workload.snapshots.delete"

	// The container permissions cover a Docker VM's images, networks and
	// volumes too.
	WorkloadContainersIndex  = "workload.containers.index"
	WorkloadContainersCreate = "workload.containers.create"
	WorkloadContainersShow   = "workload.containers.show"
	WorkloadContainersDelete = "workload.containers.delete"
	WorkloadContainersLogs   = "workload.containers.logs"

	// WorkloadContainersManage covers starting, stopping and restarting a
	// container, and connecting it to a network or disconnecting it from one.
	WorkloadContainersManage = "workload.containers.manage"

	WorkloadStacksIndex  = "workload.stacks.index"
	WorkloadStacksCreate = "workload.stacks.create"
	WorkloadStacksShow   = "workload.stacks.show"
	WorkloadStacksDelete = "workload.stacks.delete"

	// WorkloadStacksManage covers starting, stopping and restarting.
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

	SelfWorkloadVMsIndex  = "self.workload.vms.index"
	SelfWorkloadVMsShow   = "self.workload.vms.show"
	SelfWorkloadVMsUpdate = "self.workload.vms.update"
	SelfWorkloadVMsDelete = "self.workload.vms.delete"
	SelfWorkloadVMsManage = "self.workload.vms.manage"
	SelfWorkloadVMsLogs   = "self.workload.vms.logs"
	SelfWorkloadVMsAttach = "self.workload.vms.attach"

	SelfWorkloadSnapshotsIndex  = "self.workload.snapshots.index"
	SelfWorkloadSnapshotsShow   = "self.workload.snapshots.show"
	SelfWorkloadSnapshotsUpdate = "self.workload.snapshots.update"
	SelfWorkloadSnapshotsDelete = "self.workload.snapshots.delete"

	SelfWorkloadContainersIndex  = "self.workload.containers.index"
	SelfWorkloadContainersShow   = "self.workload.containers.show"
	SelfWorkloadContainersDelete = "self.workload.containers.delete"
	SelfWorkloadContainersManage = "self.workload.containers.manage"
	SelfWorkloadContainersLogs   = "self.workload.containers.logs"

	SelfWorkloadStacksIndex  = "self.workload.stacks.index"
	SelfWorkloadStacksShow   = "self.workload.stacks.show"
	SelfWorkloadStacksManage = "self.workload.stacks.manage"
	SelfWorkloadStacksDelete = "self.workload.stacks.delete"
)
