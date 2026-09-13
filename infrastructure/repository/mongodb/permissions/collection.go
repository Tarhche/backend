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

	// self articles
	{Name: "list of self articles", Value: permission.SelfArticlesIndex},
	{Name: "show a self article", Value: permission.SelfArticlesShow},
	{Name: "update a self article", Value: permission.SelfArticlesUpdate},
	{Name: "delete a self article", Value: permission.SelfArticlesDelete},

	// self files
	{Name: "list of self files", Value: permission.SelfFilesIndex},
	{Name: "delete a self file", Value: permission.SelfFilesDelete},

	// self tasks
	{Name: "list of self tasks", Value: permission.SelfRunnerTasksIndex},
	{Name: "show a self task", Value: permission.SelfRunnerTasksShow},
	{Name: "read a self task's logs", Value: permission.SelfRunnerTasksLogs},
	{Name: "stop, kill or restart a self task", Value: permission.SelfRunnerTasksManage},
	{Name: "open a terminal in a self task", Value: permission.SelfRunnerTasksAttach},
	{Name: "delete a self task", Value: permission.SelfRunnerTasksDelete},

	// self stacks
	{Name: "list of self stacks", Value: permission.SelfRunnerStacksIndex},
	{Name: "show a self stack", Value: permission.SelfRunnerStacksShow},
	{Name: "stop, kill or restart a self stack", Value: permission.SelfRunnerStacksManage},
	{Name: "delete a self stack", Value: permission.SelfRunnerStacksDelete},

	// runner tasks
	{Name: "list of tasks", Value: permission.RunnerTasksIndex},
	{Name: "run a task", Value: permission.RunnerTasksCreate},
	{Name: "show a task", Value: permission.RunnerTasksShow},
	{Name: "delete a task", Value: permission.RunnerTasksDelete},
	{Name: "read a task's logs", Value: permission.RunnerTasksLogs},
	{Name: "stop, kill or restart a task", Value: permission.RunnerTasksManage},
	{Name: "open a terminal in a task", Value: permission.RunnerTasksAttach},

	// runner stacks
	{Name: "list of stacks", Value: permission.RunnerStacksIndex},
	{Name: "run a stack", Value: permission.RunnerStacksCreate},
	{Name: "show a stack", Value: permission.RunnerStacksShow},
	{Name: "delete a stack", Value: permission.RunnerStacksDelete},
	{Name: "stop, kill or restart a stack", Value: permission.RunnerStacksManage},
}
