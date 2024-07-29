package permissions

import "github.com/khanzadimahdi/testproject/domain/permission"

var collection []permission.Permission = []permission.Permission{
	// articles
	{Name: "list of articles", Value: permission.ArticlesIndex},
	{Name: "create an article", Value: permission.ArticlesCreate},
	{Name: "show an article", Value: permission.ArticlesShow},
	{Name: "update an article", Value: permission.ArticlesUpdate},
	{Name: "delete an article", Value: permission.ArticlesDelete},

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
}
