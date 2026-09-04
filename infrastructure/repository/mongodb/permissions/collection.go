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

	// self files
	{Name: "list of self files", Value: permission.SelfFilesIndex},
	{Name: "delete a self file", Value: permission.SelfFilesDelete},

	// self articles
	{Name: "list of self articles", Value: permission.SelfArticlesIndex},
	{Name: "show a self article", Value: permission.SelfArticlesShow},
	{Name: "update a self article", Value: permission.SelfArticlesUpdate},
	{Name: "delete a self article", Value: permission.SelfArticlesDelete},

	// self containers
	{Name: "list of self containers", Value: permission.SelfRunnerContainersIndex},
	{Name: "show a self container", Value: permission.SelfRunnerContainersShow},
	{Name: "read a self container's logs", Value: permission.SelfRunnerContainersLogs},
	{Name: "stop, kill or restart a self container", Value: permission.SelfRunnerContainersManage},
	{Name: "open a terminal in a self container", Value: permission.SelfRunnerContainersAttach},
	{Name: "delete a self container", Value: permission.SelfRunnerContainersDelete},

	// self stacks
	{Name: "list of self stacks", Value: permission.SelfRunnerStacksIndex},
	{Name: "show a self stack", Value: permission.SelfRunnerStacksShow},
	{Name: "stop, kill or restart a self stack", Value: permission.SelfRunnerStacksManage},
	{Name: "delete a self stack", Value: permission.SelfRunnerStacksDelete},

	// runner containers
	{Name: "list of containers", Value: permission.RunnerContainersIndex},
	{Name: "run a container", Value: permission.RunnerContainersCreate},
	{Name: "show a container", Value: permission.RunnerContainersShow},
	{Name: "delete a container", Value: permission.RunnerContainersDelete},
	{Name: "read a container's logs", Value: permission.RunnerContainersLogs},
	{Name: "stop, kill or restart a container", Value: permission.RunnerContainersManage},
	{Name: "open a terminal in a container", Value: permission.RunnerContainersAttach},

	// runner stacks
	{Name: "list of stacks", Value: permission.RunnerStacksIndex},
	{Name: "run a stack", Value: permission.RunnerStacksCreate},
	{Name: "show a stack", Value: permission.RunnerStacksShow},
	{Name: "delete a stack", Value: permission.RunnerStacksDelete},
	{Name: "stop, kill or restart a stack", Value: permission.RunnerStacksManage},
}
