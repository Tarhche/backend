package permissions

import "github.com/khanzadimahdi/testproject/domain/permission"

var collection []permission.Permission = []permission.Permission{
	// articles
	{Name: "list of articles", Value: permission.ArticlesIndex},
	{Name: "create an article", Value: permission.ArticlesCreate},
	{Name: "show an article", Value: permission.ArticlesShow},
	{Name: "update an article", Value: permission.ArticlesUpdate},
	{Name: "delete an article", Value: permission.ArticlesDelete},

	// comments
	{Name: "list of comments", Value: permission.CommentsIndex},
	{Name: "create an comment", Value: permission.CommentsCreate},
	{Name: "show an comment", Value: permission.CommentsShow},
	{Name: "update an comment", Value: permission.CommentsUpdate},
	{Name: "delete an comment", Value: permission.CommentsDelete},

	// elements
	{Name: "list of elements", Value: permission.ElementsIndex},
	{Name: "create an element", Value: permission.ElementsCreate},
	{Name: "show an element", Value: permission.ElementsShow},
	{Name: "update an element", Value: permission.ElementsUpdate},
	{Name: "delete an element", Value: permission.ElementsDelete},

	// files
	{Name: "list of files", Value: permission.FilesIndex},
	{Name: "create a file", Value: permission.FilesCreate},
	{Name: "show a file", Value: permission.FilesShow},
	{Name: "delete a file", Value: permission.FilesDelete},

	// users
	{Name: "list of users", Value: permission.UsersIndex},
	{Name: "create a user", Value: permission.UsersCreate},
	{Name: "show a user", Value: permission.UsersShow},
	{Name: "update a user", Value: permission.UsersUpdate},
	{Name: "delete a user", Value: permission.UsersDelete},
	{Name: "update a user's password", Value: permission.UsersPasswordUpdate},
	{Name: "sign in as a user", Value: permission.UsersImpersonate},

	// permissions
	{Name: "list of permissions", Value: permission.PermissionsIndex},

	// roles
	{Name: "list of roles", Value: permission.RolesIndex},
	{Name: "create a role", Value: permission.RolesCreate},
	{Name: "show a role", Value: permission.RolesShow},
	{Name: "update a role", Value: permission.RolesUpdate},
	{Name: "delete a role", Value: permission.RolesDelete},

	// contact us
	{Name: "list of contact-us messages", Value: permission.ContactUsIndex},
	{Name: "show a contact-us message", Value: permission.ContactUsShow},
	{Name: "delete a contact-us message", Value: permission.ContactUsDelete},
	{Name: "mark a contact-us message as read", Value: permission.ContactUsMarkAsRead},

	// config
	{Name: "show configuration", Value: permission.ConfigShow},
	{Name: "update configuration", Value: permission.ConfigUpdate},

	// languages
	{Name: "list of languages", Value: permission.LanguagesIndex},
	{Name: "create a language", Value: permission.LanguagesCreate},
	{Name: "show a language", Value: permission.LanguagesShow},
	{Name: "update a language", Value: permission.LanguagesUpdate},
	{Name: "delete a language", Value: permission.LanguagesDelete},

	// self bookmarks
	{Name: "list of self bookmarks", Value: permission.SelfBookmarksIndex},
	{Name: "delete a self bookmark", Value: permission.SelfBookmarksDelete},

	// self comments
	{Name: "list of self comments", Value: permission.SelfCommentsIndex},
	{Name: "show a self comment", Value: permission.SelfCommentsShow},
	{Name: "update a self comment", Value: permission.SelfCommentsUpdate},
	{Name: "delete a self comment", Value: permission.SelfCommentsDelete},

	// self articles
	{Name: "list of self articles", Value: permission.SelfArticlesIndex},
	{Name: "show a self article", Value: permission.SelfArticlesShow},
	{Name: "update a self article", Value: permission.SelfArticlesUpdate},
	{Name: "delete a self article", Value: permission.SelfArticlesDelete},

	// self files
	{Name: "list of self files", Value: permission.SelfFilesIndex},
	{Name: "delete a self file", Value: permission.SelfFilesDelete},

	// self vms
	{Name: "list of self vms", Value: permission.SelfWorkloadVMsIndex},
	{Name: "show a self vm", Value: permission.SelfWorkloadVMsShow},
	{Name: "update a self vm", Value: permission.SelfWorkloadVMsUpdate},
	{Name: "delete a self vm", Value: permission.SelfWorkloadVMsDelete},
	{Name: "start, stop, restart or restore a self vm", Value: permission.SelfWorkloadVMsManage},
	{Name: "read a self vm's logs", Value: permission.SelfWorkloadVMsLogs},
	{Name: "open a terminal in a self vm", Value: permission.SelfWorkloadVMsAttach},

	// self snapshots
	{Name: "list of self snapshots", Value: permission.SelfWorkloadSnapshotsIndex},
	{Name: "show a self snapshot", Value: permission.SelfWorkloadSnapshotsShow},
	{Name: "rename a self snapshot", Value: permission.SelfWorkloadSnapshotsUpdate},
	{Name: "delete a self snapshot", Value: permission.SelfWorkloadSnapshotsDelete},

	// self containers, and the images, networks and volumes of self docker vms
	{Name: "list of self containers", Value: permission.SelfWorkloadContainersIndex},
	{Name: "show a self container", Value: permission.SelfWorkloadContainersShow},
	{Name: "delete a self container", Value: permission.SelfWorkloadContainersDelete},
	{Name: "start, stop, restart or connect a self container", Value: permission.SelfWorkloadContainersManage},
	{Name: "read a self container's logs", Value: permission.SelfWorkloadContainersLogs},

	// self stacks
	{Name: "list of self stacks", Value: permission.SelfWorkloadStacksIndex},
	{Name: "show a self stack", Value: permission.SelfWorkloadStacksShow},
	{Name: "start, stop or restart a self stack", Value: permission.SelfWorkloadStacksManage},
	{Name: "delete a self stack", Value: permission.SelfWorkloadStacksDelete},

	// workload vms
	{Name: "list of vms", Value: permission.WorkloadVMsIndex},
	{Name: "create a vm", Value: permission.WorkloadVMsCreate},
	{Name: "show a vm", Value: permission.WorkloadVMsShow},
	{Name: "update a vm", Value: permission.WorkloadVMsUpdate},
	{Name: "delete a vm", Value: permission.WorkloadVMsDelete},
	{Name: "start, stop, restart or restore a vm", Value: permission.WorkloadVMsManage},
	{Name: "read a vm's logs", Value: permission.WorkloadVMsLogs},
	{Name: "open a terminal in a vm", Value: permission.WorkloadVMsAttach},

	// workload snapshots
	{Name: "list of snapshots", Value: permission.WorkloadSnapshotsIndex},
	{Name: "take a snapshot", Value: permission.WorkloadSnapshotsCreate},
	{Name: "show a snapshot", Value: permission.WorkloadSnapshotsShow},
	{Name: "rename a snapshot", Value: permission.WorkloadSnapshotsUpdate},
	{Name: "delete a snapshot", Value: permission.WorkloadSnapshotsDelete},

	// workload containers, and the images, networks and volumes of docker vms
	{Name: "list of containers", Value: permission.WorkloadContainersIndex},
	{Name: "create a container", Value: permission.WorkloadContainersCreate},
	{Name: "show a container", Value: permission.WorkloadContainersShow},
	{Name: "delete a container", Value: permission.WorkloadContainersDelete},
	{Name: "start, stop, restart or connect a container", Value: permission.WorkloadContainersManage},
	{Name: "read a container's logs", Value: permission.WorkloadContainersLogs},

	// workload stacks
	{Name: "list of stacks", Value: permission.WorkloadStacksIndex},
	{Name: "deploy a stack", Value: permission.WorkloadStacksCreate},
	{Name: "show a stack", Value: permission.WorkloadStacksShow},
	{Name: "delete a stack", Value: permission.WorkloadStacksDelete},
	{Name: "start, stop or restart a stack", Value: permission.WorkloadStacksManage},
}
