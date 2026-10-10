package mcp

import (
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/khanzadimahdi/testproject/application/auth/forgetpassword"
	"github.com/khanzadimahdi/testproject/application/auth/login"
	"github.com/khanzadimahdi/testproject/application/auth/refresh"
	"github.com/khanzadimahdi/testproject/application/auth/register"
	"github.com/khanzadimahdi/testproject/application/auth/resetpassword"
	"github.com/khanzadimahdi/testproject/application/auth/verify"
	"github.com/khanzadimahdi/testproject/application/bookmark/bookmarkExists"
	"github.com/khanzadimahdi/testproject/application/bookmark/updateBookmark"
	"github.com/khanzadimahdi/testproject/application/comment/createComment"
	"github.com/khanzadimahdi/testproject/application/contact/createMessage"
	dashboardCreateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/createArticle"
	dashboardUpdateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateArticle"
	dashboardUpdateUserArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateUserArticle"
	dashboardDeleteUserBookmark "github.com/khanzadimahdi/testproject/application/dashboard/bookmark/deleteUserBookmark"
	dashboardCreateComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/createComment"
	dashboardUpdateComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/updateComment"
	dashboardUpdateUserComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/updateUserComment"
	dashboardUpdateConfig "github.com/khanzadimahdi/testproject/application/dashboard/config/updateConfig"
	dashboardMarkContactMessageAsRead "github.com/khanzadimahdi/testproject/application/dashboard/contact/markAsRead"
	dashboardCreateLanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/createLanguage"
	dashboardUpdateLanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/updateLanguage"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/changepassword"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/updateprofile"
	dashboardCreateRole "github.com/khanzadimahdi/testproject/application/dashboard/role/createRole"
	dashboardUpdateRole "github.com/khanzadimahdi/testproject/application/dashboard/role/updateRole"
	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
	"github.com/khanzadimahdi/testproject/application/dashboard/user/userchangepassword"
	dashboardConnectNetwork "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/connectNetwork"
	dashboardCreateContainer "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/createContainer"
	dashboardPullImage "github.com/khanzadimahdi/testproject/application/dashboard/workload/image/pullImage"
	dashboardCreateNetwork "github.com/khanzadimahdi/testproject/application/dashboard/workload/network/createNetwork"
	dashboardCreateSnapshot "github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/createSnapshot"
	dashboardRenameSnapshot "github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/renameSnapshot"
	dashboardCreateStack "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	dashboardCreateVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	dashboardRestoreVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restoreVM"
	dashboardUpdateVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/updateVM"
	dashboardCreateVolume "github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/createVolume"
)

// tool is one thing an MCP client can do, and the route it is done through.
//
// A tool has no use case of its own: it names a route, and calling it makes
// that request inside this process, through the same handler and the same
// middleware an HTTP client reaches. Whether the caller may is therefore
// answered exactly once, by the route, and nothing here can answer it
// differently.
type tool struct {
	name        string
	description string

	// route is the pattern the request is made against, written exactly as the
	// router registers it, so the two can be compared.
	route string

	// params are what the route's path and query string carry. A parameter
	// named in the path is required; the rest are not.
	params []Parameter

	// body describes the json the route reads, and is nil for a route that
	// reads none.
	body *jsonschema.Schema

	// upload says the route takes a file rather than json.
	upload bool

	// download says what comes back is a file rather than json.
	download bool

	readOnly    bool
	destructive bool
	idempotent  bool
}

func (t tool) method() string {
	method, _, _ := strings.Cut(t.route, " ")

	return method
}

func (t tool) path() string {
	_, path, _ := strings.Cut(t.route, " ")

	return path
}

// placeholders are the names the path writes its parameters under, in the
// order they appear.
func (t tool) placeholders() []string {
	var names []string

	for segment := range strings.SplitSeq(t.path(), "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			names = append(names, strings.Trim(segment, "{}"))
		}
	}

	return names
}

// pathParams are those same parameters under the names a caller fills them in
// by.
func (t tool) pathParams() []string {
	placeholders := t.placeholders()
	names := make([]string, len(placeholders))
	for i, placeholder := range placeholders {
		names[i] = snake(placeholder)
	}

	return names
}

// the parameters that come up again and again.
func uuidOf(what string) Parameter {
	return text("uuid", "the "+what+"'s uuid")
}

func correlationUUID() Parameter {
	return text("correlation_uuid", "the article's correlation uuid, which is the identity it keeps across languages")
}

func languageCode() Parameter {
	return text("language_code", "which language version, as its code, such as en or fa")
}

// tools is everything this API can do, as things an MCP client can do.
//
// There is one for every route the blog registers, which is checked when the
// server is built rather than believed: a route with no tool is a part of the
// API an agent cannot reach, and a tool with no route is a tool that would
// answer nothing.
func tools() []tool {
	return slices.Concat(
		publicTools(),
		authTools(),
		profileTools(),
		userTools(),
		roleTools(),
		languageTools(),
		articleTools(),
		commentTools(),
		bookmarkTools(),
		fileTools(),
		elementTools(),
		contactTools(),
		configTools(),
		vmTools(),
		snapshotTools(),
		containerTools(),
		dockerObjectTools(),
		stackTools(),
	)
}

func publicTools() []tool {
	return []tool{
		{
			name:        "health_check",
			description: "Report whether the API and the things it leans on — its database and its message broker — are answering.",
			route:       "GET /health",
			readOnly:    true,
		},
		{
			name:        "home_show",
			description: "The contents of the home page: the elements it is built from and the articles they point at.",
			route:       "GET /api/home",
			readOnly:    true,
		},
		{
			name:        "articles_list",
			description: "A page of the most recently published articles, newest first.",
			route:       "GET /api/articles",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "article_show",
			description: "One published article, by the correlation uuid a public listing gives. The language is the one the session resolves to.",
			route:       "GET /api/articles/{uuid}",
			params:      []Parameter{text("uuid", "the article's correlation uuid")},
			readOnly:    true,
		},
		{
			name:        "hashtag_articles_list",
			description: "A page of published articles carrying a hashtag.",
			route:       "GET /api/hashtags/{hashtag}",
			params:      []Parameter{text("hashtag", "the hashtag, without the leading #"), page()},
			readOnly:    true,
		},
		{
			name:        "author_articles_list",
			description: "A page of published articles by one author.",
			route:       "GET /api/authors/{identity}/articles",
			params:      []Parameter{text("identity", "the author's uuid or username"), page()},
			readOnly:    true,
		},
		{
			name:        "languages_list",
			description: "The languages this site is published in.",
			route:       "GET /api/languages",
			readOnly:    true,
		},
		{
			name:        "comments_list",
			description: "A page of the approved comments on something, such as an article.",
			route:       "GET /api/comments",
			params: []Parameter{
				page(),
				text("object_uuid", "what the comments are on, by its uuid; an article is named by its correlation uuid"),
				text("object_type", `what kind of thing that is, such as "article"`),
			},
			readOnly: true,
		},
		{
			name:        "comment_create",
			description: "Write a comment, as whoever this session is for. It is held until somebody approves it.",
			route:       "POST /api/comments",
			body:        body[createComment.Request](),
		},
		{
			name:        "contact_message_send",
			description: "Send a message to the people who keep this site. It needs no account.",
			route:       "POST /api/contact-us",
			body:        body[createMessage.Request](),
		},
		{
			name:        "bookmark_exists",
			description: "Whether this session's owner has bookmarked something.",
			route:       "POST /api/bookmarks/exists",
			body:        body[bookmarkExists.Request](),
			readOnly:    true,
		},
		{
			name:        "bookmark_update",
			description: "Bookmark something, or take the bookmark away by asking not to keep it.",
			route:       "PUT /api/bookmarks",
			body:        body[updateBookmark.Request](),
			idempotent:  true,
		},
		{
			name:        "file_download",
			description: "The contents of a published file, by its uuid.",
			route:       "GET /files/{uuid}",
			params:      []Parameter{uuidOf("file")},
			download:    true,
			readOnly:    true,
		},
	}
}

func authTools() []tool {
	return []tool{
		{
			name:        "auth_login",
			description: "Trade a username or email and a password for a session. An MCP client does not need this: it is given a session by the authorization server, and this is here because the API has it.",
			route:       "POST /api/auth/login",
			body:        body[login.Request](),
		},
		{
			name:        "auth_token_refresh",
			description: "Trade a refresh token for a new pair of tokens.",
			route:       "POST /api/auth/token/refresh",
			body:        body[refresh.Request](),
		},
		{
			name:        "auth_register",
			description: "Ask for an account. What comes back is an email carrying a registration token.",
			route:       "POST /api/auth/register",
			body:        body[register.Request](),
		},
		{
			name:        "auth_verify",
			description: "Finish registering: the token from the email, and the password the account will have.",
			route:       "POST /api/auth/verify",
			body:        body[verify.Request](),
		},
		{
			name:        "auth_password_forget",
			description: "Ask for a password reset email.",
			route:       "POST /api/auth/password/forget",
			body:        body[forgetpassword.Request](),
		},
		{
			name:        "auth_password_reset",
			description: "Set a new password, with the token the reset email carried.",
			route:       "POST /api/auth/password/reset",
			body:        body[resetpassword.Request](),
		},
	}
}

func profileTools() []tool {
	return []tool{
		{
			name:        "profile_show",
			description: "Who this session is for: name, username, email, avatar and language.",
			route:       "GET /api/dashboard/profile",
			readOnly:    true,
		},
		{
			name:        "profile_update",
			description: "Change this session owner's own name, username, email, avatar or language.",
			route:       "PUT /api/dashboard/profile",
			body:        body[updateprofile.Request](),
			idempotent:  true,
		},
		{
			name:        "profile_password_change",
			description: "Change this session owner's own password, which needs the current one.",
			route:       "PUT /api/dashboard/password",
			body:        body[changepassword.Request](),
		},
		{
			name:        "profile_roles_list",
			description: "The roles this session's owner holds, and what each of them allows.",
			route:       "GET /api/dashboard/profile/roles",
			readOnly:    true,
		},
	}
}

func userTools() []tool {
	return []tool{
		{
			name:        "dashboard_users_list",
			description: "A page of the people with accounts here.",
			route:       "GET /api/dashboard/users",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_user_show",
			description: "One person's account.",
			route:       "GET /api/dashboard/users/{uuid}",
			params:      []Parameter{uuidOf("user")},
			readOnly:    true,
		},
		{
			name:        "dashboard_user_create",
			description: "Make an account for somebody, with the password it starts with.",
			route:       "POST /api/dashboard/users",
			body:        body[createuser.Request](),
		},
		{
			name:        "dashboard_user_update",
			description: "Change somebody's account. A banned_at in the past or now is what bans them; the zero time lifts it.",
			route:       "PUT /api/dashboard/users",
			body:        body[updateuser.Request](),
			idempotent:  true,
		},
		{
			name:        "dashboard_user_delete",
			description: "Remove somebody's account.",
			route:       "DELETE /api/dashboard/users/{uuid}",
			params:      []Parameter{uuidOf("user")},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "dashboard_user_password_change",
			description: "Set somebody else's password, without being asked for theirs.",
			route:       "PUT /api/dashboard/users/password",
			body:        body[userchangepassword.Request](),
		},
		{
			name:        "dashboard_user_impersonate",
			description: "Open a session that acts as somebody else and says who is behind it. What comes back is a pair of tokens for them, not for you.",
			route:       "POST /api/dashboard/users/{uuid}/impersonate",
			params:      []Parameter{uuidOf("user")},
		},
		{
			name:        "dashboard_permissions_list",
			description: "Every permission a role can be given, and what each one covers.",
			route:       "GET /api/dashboard/permissions",
			readOnly:    true,
		},
	}
}

func roleTools() []tool {
	return []tool{
		{
			name:        "dashboard_roles_list",
			description: "A page of the roles permissions are given through.",
			route:       "GET /api/dashboard/roles",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_role_show",
			description: "One role, with the permissions it carries and the people who hold it.",
			route:       "GET /api/dashboard/roles/{uuid}",
			params:      []Parameter{uuidOf("role")},
			readOnly:    true,
		},
		{
			name:        "dashboard_role_create",
			description: "Make a role out of a set of permissions, and give it to people.",
			route:       "POST /api/dashboard/roles",
			body:        body[dashboardCreateRole.Request](),
		},
		{
			name:        "dashboard_role_update",
			description: "Change what a role allows or who holds it. The permissions and the people sent replace the ones it had.",
			route:       "PUT /api/dashboard/roles",
			body:        body[dashboardUpdateRole.Request](),
			idempotent:  true,
		},
		{
			name:        "dashboard_role_delete",
			description: "Remove a role, taking what it allowed from everybody who held it.",
			route:       "DELETE /api/dashboard/roles/{uuid}",
			params:      []Parameter{uuidOf("role")},
			destructive: true,
			idempotent:  true,
		},
	}
}

func languageTools() []tool {
	return []tool{
		{
			name:        "dashboard_languages_list",
			description: "A page of the languages this site is published in.",
			route:       "GET /api/dashboard/languages",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_language_show",
			description: "One language, by its code.",
			route:       "GET /api/dashboard/languages/{code}",
			params:      []Parameter{text("code", "the language's code, such as en or fa")},
			readOnly:    true,
		},
		{
			name:        "dashboard_language_create",
			description: "Add a language the site may be published in.",
			route:       "POST /api/dashboard/languages",
			body:        body[dashboardCreateLanguage.Request](),
		},
		{
			name:        "dashboard_language_update",
			description: "Rename a language.",
			route:       "PUT /api/dashboard/languages",
			body:        body[dashboardUpdateLanguage.Request](),
			idempotent:  true,
		},
		{
			name:        "dashboard_language_delete",
			description: "Remove a language.",
			route:       "DELETE /api/dashboard/languages/{code}",
			params:      []Parameter{text("code", "the language's code, such as en or fa")},
			destructive: true,
			idempotent:  true,
		},
	}
}

func articleTools() []tool {
	return []tool{
		{
			name:        "dashboard_articles_list",
			description: "A page of every article, published or not, grouped by the correlation uuid its language versions share.",
			route:       "GET /api/dashboard/articles",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_article_show",
			description: "One language version of an article.",
			route:       "GET /api/dashboard/articles/{correlationUUID}/{language_code}",
			params:      []Parameter{correlationUUID(), languageCode()},
			readOnly:    true,
		},
		{
			name:        "dashboard_article_create",
			description: "Write an article, as whoever this session is for. Leaving correlation_uuid out starts a new article; giving the correlation uuid of one that exists adds a language version of it.",
			route:       "POST /api/dashboard/articles",
			body:        body[dashboardCreateArticle.Request](),
		},
		{
			name:        "dashboard_article_update",
			description: "Change a language version of an article. The author is left as it was.",
			route:       "PUT /api/dashboard/articles",
			body:        body[dashboardUpdateArticle.Request](),
			idempotent:  true,
		},
		{
			name:        "dashboard_article_delete",
			description: "Remove one language version of an article. The other languages stay.",
			route:       "DELETE /api/dashboard/articles/{correlationUUID}/{language_code}",
			params:      []Parameter{correlationUUID(), languageCode()},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "my_articles_list",
			description: "A page of the articles this session's owner wrote, grouped by correlation uuid.",
			route:       "GET /api/dashboard/my/articles",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "my_article_show",
			description: "One language version of an article this session's owner wrote.",
			route:       "GET /api/dashboard/my/articles/{correlationUUID}/{language_code}",
			params:      []Parameter{correlationUUID(), languageCode()},
			readOnly:    true,
		},
		{
			name:        "my_article_update",
			description: "Change a language version of an article this session's owner wrote.",
			route:       "PUT /api/dashboard/my/articles",
			body:        body[dashboardUpdateUserArticle.Request](),
			idempotent:  true,
		},
		{
			name:        "my_article_delete",
			description: "Remove one language version of an article this session's owner wrote.",
			route:       "DELETE /api/dashboard/my/articles/{correlationUUID}/{language_code}",
			params:      []Parameter{correlationUUID(), languageCode()},
			destructive: true,
			idempotent:  true,
		},
	}
}

func commentTools() []tool {
	return []tool{
		{
			name:        "dashboard_comments_list",
			description: "A page of every comment, approved or not.",
			route:       "GET /api/dashboard/comments",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_comment_show",
			description: "One comment.",
			route:       "GET /api/dashboard/comments/{uuid}",
			params:      []Parameter{uuidOf("comment")},
			readOnly:    true,
		},
		{
			name:        "dashboard_comment_create",
			description: "Write a comment as whoever this session is for. An approved_at in the past publishes it at once.",
			route:       "POST /api/dashboard/comments",
			body:        body[dashboardCreateComment.Request]("author_uuid"),
		},
		{
			name:        "dashboard_comment_update",
			description: "Change a comment, or approve one by giving it an approved_at.",
			route:       "PUT /api/dashboard/comments",
			body:        body[dashboardUpdateComment.Request]("author_uuid"),
			idempotent:  true,
		},
		{
			name:        "dashboard_comment_delete",
			description: "Remove a comment.",
			route:       "DELETE /api/dashboard/comments/{uuid}",
			params:      []Parameter{uuidOf("comment")},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "my_comments_list",
			description: "A page of the comments this session's owner wrote.",
			route:       "GET /api/dashboard/my/comments",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "my_comment_show",
			description: "One comment this session's owner wrote.",
			route:       "GET /api/dashboard/my/comments/{uuid}",
			params:      []Parameter{uuidOf("comment")},
			readOnly:    true,
		},
		{
			name:        "my_comment_update",
			description: "Change the text of a comment this session's owner wrote.",
			route:       "PUT /api/dashboard/my/comments",
			body:        body[dashboardUpdateUserComment.Request](),
			idempotent:  true,
		},
		{
			name:        "my_comment_delete",
			description: "Remove a comment this session's owner wrote.",
			route:       "DELETE /api/dashboard/my/comments/{uuid}",
			params:      []Parameter{uuidOf("comment")},
			destructive: true,
			idempotent:  true,
		},
	}
}

func bookmarkTools() []tool {
	return []tool{
		{
			name:        "my_bookmarks_list",
			description: "A page of what this session's owner has bookmarked.",
			route:       "GET /api/dashboard/my/bookmarks",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "my_bookmark_delete",
			description: "Take away one of this session owner's bookmarks.",
			route:       "DELETE /api/dashboard/my/bookmarks",
			body:        body[dashboardDeleteUserBookmark.Request](),
			destructive: true,
			idempotent:  true,
		},
	}
}

func fileTools() []tool {
	return []tool{
		{
			name:        "dashboard_files_list",
			description: "A page of every uploaded file.",
			route:       "GET /api/dashboard/files",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_file_download",
			description: "The contents of an uploaded file, by its uuid.",
			route:       "GET /dashboard/files/{uuid}",
			params:      []Parameter{uuidOf("file")},
			download:    true,
			readOnly:    true,
		},
		{
			name:        "dashboard_file_upload",
			description: "Upload a file, as whoever this session is for. The content is base64, and what comes back is the uuid it is served under.",
			route:       "POST /api/dashboard/files",
			upload:      true,
			body: object(map[string]*jsonschema.Schema{
				"name":    {Type: "string", Description: "what the file is called, with its extension"},
				"content": {Type: "string", ContentEncoding: "base64", Description: "the file itself, base64 encoded"},
			}, "name", "content"),
		},
		{
			name:        "dashboard_file_delete",
			description: "Remove an uploaded file, whoever uploaded it.",
			route:       "DELETE /api/dashboard/files/{uuid}",
			params:      []Parameter{uuidOf("file")},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "my_files_list",
			description: "A page of the files this session's owner uploaded.",
			route:       "GET /api/dashboard/my/files",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "my_file_delete",
			description: "Remove a file this session's owner uploaded.",
			route:       "DELETE /api/dashboard/my/files/{uuid}",
			params:      []Parameter{uuidOf("file")},
			destructive: true,
			idempotent:  true,
		},
	}
}

// elementComponent describes what an element may be built out of. The json a
// caller sends is not the shape of the struct it is read into — which
// component it is decides how the rest of it is read — so it is written here
// rather than inferred.
func elementComponent() *jsonschema.Schema {
	item := func(description string) *jsonschema.Schema {
		return &jsonschema.Schema{
			Type:        "object",
			Description: description,
			Properties: map[string]*jsonschema.Schema{
				"type":         {Type: "string", Enum: []any{"item"}},
				"content_uuid": {Type: "string", Description: "what it points at; an article is named by its correlation uuid"},
				"content_type": {Type: "string", Description: `what kind of thing that is, such as "article"`},
			},
			Required: []string{"type", "content_uuid", "content_type"},
		}
	}

	// each of these is built afresh: a schema has to be a tree, and a node
	// used in two places is not one.
	items := func() *jsonschema.Schema {
		return &jsonschema.Schema{Type: "array", Items: item("one thing the component points at")}
	}

	return &jsonschema.Schema{
		Description: "the component this element is, which its type decides the shape of",
		OneOf: []*jsonschema.Schema{
			item("one thing on its own"),
			{
				Type:        "object",
				Description: "one thing, shown large",
				Properties: map[string]*jsonschema.Schema{
					"type": {Type: "string", Enum: []any{"jumbotron"}},
					"item": item("what is shown"),
				},
				Required: []string{"type", "item"},
			},
			{
				Type:        "object",
				Description: "one thing with others beside it",
				Properties: map[string]*jsonschema.Schema{
					"type":  {Type: "string", Enum: []any{"featured"}},
					"main":  item("the one in front"),
					"aside": items(),
				},
				Required: []string{"type", "main"},
			},
			{
				Type:        "object",
				Description: "a row of cards, which may be a carousel",
				Properties: map[string]*jsonschema.Schema{
					"type":        {Type: "string", Enum: []any{"cards"}},
					"title":       {Type: "string"},
					"is_carousel": {Type: "boolean"},
					"items":       items(),
				},
				Required: []string{"type"},
			},
			{
				Type:        "object",
				Description: "a stack the current page sits in, with its neighbours around it",
				Properties: map[string]*jsonschema.Schema{
					"type":              {Type: "string", Enum: []any{"stack"}},
					"highlight_current": {Type: "boolean"},
					"visible_neighbors": {Type: "integer", Minimum: new(0.0)},
					"items":             items(),
				},
				Required: []string{"type"},
			},
		},
	}
}

func elementVenues() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "array",
		Description: `the pages this element appears on, as glob patterns matched against a path: * is one segment, ** is any number of them and ? is one character, as in "/en/articles/**"`,
		Items:       &jsonschema.Schema{Type: "string"},
	}
}

func elementTools() []tool {
	return []tool{
		{
			name:        "dashboard_elements_list",
			description: "A page of the elements pages are built out of. An element is not language-scoped; only what it points at is.",
			route:       "GET /api/dashboard/elements",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_element_show",
			description: "One element, with the component it holds and the venues it appears on.",
			route:       "GET /api/dashboard/elements/{uuid}",
			params:      []Parameter{uuidOf("element")},
			readOnly:    true,
		},
		{
			name:        "dashboard_element_create",
			description: "Put an element on some pages.",
			route:       "POST /api/dashboard/elements",
			body: object(map[string]*jsonschema.Schema{
				"venues": elementVenues(),
				"body":   elementComponent(),
			}, "body"),
		},
		{
			name:        "dashboard_element_update",
			description: "Change what an element holds or where it appears.",
			route:       "PUT /api/dashboard/elements",
			body: object(map[string]*jsonschema.Schema{
				"uuid":   {Type: "string", Description: "the element's uuid"},
				"venues": elementVenues(),
				"body":   elementComponent(),
			}, "uuid", "body"),
			idempotent: true,
		},
		{
			name:        "dashboard_element_delete",
			description: "Take an element off every page it was on.",
			route:       "DELETE /api/dashboard/elements/{uuid}",
			params:      []Parameter{uuidOf("element")},
			destructive: true,
			idempotent:  true,
		},
	}
}

func contactTools() []tool {
	return []tool{
		{
			name:        "dashboard_contact_messages_list",
			description: "A page of the messages sent through the contact form.",
			route:       "GET /api/dashboard/contact-us",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_contact_message_show",
			description: "One message sent through the contact form.",
			route:       "GET /api/dashboard/contact-us/{uuid}",
			params:      []Parameter{uuidOf("message")},
			readOnly:    true,
		},
		{
			name:        "dashboard_contact_message_mark_read",
			description: "Mark a contact message as read, or put it back to unread. The moment it was read is stamped here.",
			route:       "PUT /api/dashboard/contact-us/{uuid}/read",
			params:      []Parameter{uuidOf("message")},
			body:        body[dashboardMarkContactMessageAsRead.Request](),
			idempotent:  true,
		},
		{
			name:        "dashboard_contact_message_delete",
			description: "Remove a message sent through the contact form.",
			route:       "DELETE /api/dashboard/contact-us/{uuid}",
			params:      []Parameter{uuidOf("message")},
			destructive: true,
			idempotent:  true,
		},
	}
}

func configTools() []tool {
	return []tool{
		{
			name:        "dashboard_config_show",
			description: "The site's settings: the roles a new account is given and the language it falls back to.",
			route:       "GET /api/dashboard/config",
			readOnly:    true,
		},
		{
			name:        "dashboard_config_update",
			description: "Change the site's settings.",
			route:       "PUT /api/dashboard/config",
			body:        body[dashboardUpdateConfig.Request](),
			idempotent:  true,
		},
	}
}

// both is a tool on the workload's routes, which act on anybody's things under
// the workload's permissions, and its twin on the my routes, which act on this
// session owner's own under the self ones: the same route under
// /api/dashboard/my/workload, named my_ rather than dashboard_, and said the
// way its owner would ask for it.
func both(anybody tool, own string) []tool {
	mine := anybody
	mine.name = "my_" + strings.TrimPrefix(anybody.name, "dashboard_")
	mine.route = strings.Replace(anybody.route, " /api/dashboard/workload/", " /api/dashboard/my/workload/", 1)
	mine.description = own

	return []tool{anybody, mine}
}

// choice is a parameter that is one of a few words.
func choice(name string, description string, values ...string) Parameter {
	enum := make([]any, len(values))
	for i, value := range values {
		enum[i] = value
	}

	return Parameter{name: name, description: description, schema: &jsonschema.Schema{Type: "string", Enum: enum}}
}

func vmUUID() Parameter {
	return uuidOf("VM")
}

func containerID() Parameter {
	return text("id", "the container's id or its name")
}

// logParams are what reading a log takes: from when, and how much of its end.
func logParams() []Parameter {
	return []Parameter{
		text("since", "only lines written from this moment on, as an RFC 3339 timestamp; what a reader following a log asks with, from the last line it has"),
		integer("tail", "only the last this many lines"),
	}
}

// vmTools are a person's virtual machines: a machine booted from an OS image,
// or a Docker VM, which is what containers and stacks are run in.
func vmTools() []tool {
	return slices.Concat(
		both(tool{
			name:        "dashboard_vms_list",
			description: "A page of the VMs the workload holds, whoever owns them. The code runner's snippets are among them while they run, as the guest's machines with managed_by code-runner.",
			route:       "GET /api/dashboard/workload/vms",
			params:      []Parameter{page()},
			readOnly:    true,
		}, "A page of this session owner's own VMs."),
		[]tool{{
			name:        "dashboard_vm_create",
			description: "Create a VM for this session's owner: a machine from an OS image, or a Docker VM to run containers and stacks in, or one restored from a snapshot of theirs. Resources are whole vCPUs and bytes; the workload boots it in its own time, and its state says how that is going.",
			route:       "POST /api/dashboard/workload/vms",
			body:        body[dashboardCreateVM.Request](),
		}},
		both(tool{
			name:        "dashboard_vm_show",
			description: "One VM, whoever owns it, with the addresses its ports are served on while its ingress allows it; a code runner's snippet, while it runs, is one too, with managed_by code-runner.",
			route:       "GET /api/dashboard/workload/vms/{uuid}",
			params:      []Parameter{vmUUID()},
			readOnly:    true,
		}, "One of this session owner's own VMs, with the addresses its ports are served on."),
		both(tool{
			name:        "dashboard_vm_update",
			description: "Change a VM, whoever owns it: only what is sent changes. Changing its ports, network or resources (sent whole) restarts it unless it is stopped; its disk only grows, and its kind and image never change.",
			route:       "PATCH /api/dashboard/workload/vms/{uuid}",
			params:      []Parameter{vmUUID()},
			body:        body[dashboardUpdateVM.Request](),
			idempotent:  true,
		}, "Change one of this session owner's own VMs: only what is sent changes, and changing its ports, network or resources restarts it unless it is stopped."),
		both(tool{
			name:        "dashboard_vm_delete",
			description: "Delete a VM and its disk, whoever owns it. Its snapshots stay. A code runner's snippet is taken away at once, running or not.",
			route:       "DELETE /api/dashboard/workload/vms/{uuid}",
			params:      []Parameter{vmUUID()},
			destructive: true,
			idempotent:  true,
		}, "Delete one of this session owner's own VMs and its disk. Its snapshots stay."),
		both(tool{
			name:        "dashboard_vm_start",
			description: "Start a stopped VM, whoever owns it.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/start",
			params:      []Parameter{vmUUID()},
			idempotent:  true,
		}, "Start one of this session owner's own VMs."),
		both(tool{
			name:        "dashboard_vm_stop",
			description: "Stop a VM, whoever owns it. Its disk is kept, and is as it was left when it starts again only if the disk is persistent. A code runner's snippet stopped is gone once it has; it can only be stopped, deleted and read.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/stop",
			params:      []Parameter{vmUUID()},
			idempotent:  true,
		}, "Stop one of this session owner's own VMs."),
		both(tool{
			name:        "dashboard_vm_restart",
			description: "Stop a VM, whoever owns it, and start it again in place.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/restart",
			params:      []Parameter{vmUUID()},
		}, "Restart one of this session owner's own VMs."),
		both(tool{
			name:        "dashboard_vm_restore",
			description: "Replace a VM's disk with a snapshot's, whoever owns it: one of its owner's snapshots, of the same kind and engine and no larger than its disk. The VM is stopped, restored and started again, keeping its uuid, its slug and its ports.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/restore",
			params:      []Parameter{vmUUID()},
			body:        body[dashboardRestoreVM.Request](),
			destructive: true,
		}, "Replace the disk of one of this session owner's own VMs with one of their snapshots'. What was on the disk is gone."),
		both(tool{
			name:        "dashboard_vm_logs",
			description: "The tail of what a VM has written, whoever owns it, read from its node as it is now, or what a code runner's snippet has printed so far. Opening a terminal in a VM is a stream on the workload's ingress rather than a tool.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/logs",
			params:      append([]Parameter{vmUUID()}, logParams()...),
			readOnly:    true,
		}, "The tail of what one of this session owner's own VMs has written."),
	)
}

// snapshotTools are VMs' disks, kept as archives. A snapshot outlives the VM
// it was taken of.
func snapshotTools() []tool {
	return slices.Concat(
		both(tool{
			name:        "dashboard_snapshots_list",
			description: "A page of the snapshots the workload keeps, whoever owns them, narrowed to one VM's with vm.",
			route:       "GET /api/dashboard/workload/snapshots",
			params:      []Parameter{page(), text("vm", "only the snapshots of the VM with this uuid")},
			readOnly:    true,
		}, "A page of this session owner's own snapshots, narrowed to one VM's with vm."),
		[]tool{{
			name:        "dashboard_snapshot_create",
			description: "Take a snapshot of one of this session owner's own VMs, which has to be running or stopped. It is being created until its archive is stored.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/snapshots",
			params:      []Parameter{vmUUID()},
			body:        body[dashboardCreateSnapshot.Request](),
		}},
		both(tool{
			name:        "dashboard_snapshot_show",
			description: "One snapshot, whoever owns it.",
			route:       "GET /api/dashboard/workload/snapshots/{uuid}",
			params:      []Parameter{uuidOf("snapshot")},
			readOnly:    true,
		}, "One of this session owner's own snapshots."),
		both(tool{
			name:        "dashboard_snapshot_rename",
			description: "Rename a snapshot, whoever owns it.",
			route:       "PATCH /api/dashboard/workload/snapshots/{uuid}",
			params:      []Parameter{uuidOf("snapshot")},
			body:        body[dashboardRenameSnapshot.Request](),
			idempotent:  true,
		}, "Rename one of this session owner's own snapshots."),
		both(tool{
			name:        "dashboard_snapshot_delete",
			description: "Delete a snapshot and the archive it kept, whoever owns it.",
			route:       "DELETE /api/dashboard/workload/snapshots/{uuid}",
			params:      []Parameter{uuidOf("snapshot")},
			destructive: true,
			idempotent:  true,
		}, "Delete one of this session owner's own snapshots and the archive it kept."),
	)
}

// containerTools are the containers in Docker VMs, read from each VM's
// dockerd as they are now. A container is named by the VM it is in and its
// own id or name.
func containerTools() []tool {
	containerParams := []Parameter{vmUUID(), containerID()}

	return slices.Concat(
		both(tool{
			name:        "dashboard_containers_list",
			description: "The containers of every running Docker VM, whoever owns it, each with the VM it is in, narrowed to one VM with vm.",
			route:       "GET /api/dashboard/workload/containers",
			params:      []Parameter{text("vm", "only the containers of the Docker VM with this uuid")},
			readOnly:    true,
		}, "The containers of this session owner's own running Docker VMs, each with the VM it is in."),
		[]tool{{
			name:        "dashboard_container_create",
			description: "Create and start a container for this session's owner, pulling its image first when the VM lacks it: in the Docker VM vm_uuid names, in a new one vm describes, or with neither in a new one made with the defaults. The answer says which VM it went into and whether it was made for it.",
			route:       "POST /api/dashboard/workload/containers",
			body:        body[dashboardCreateContainer.Request](),
		}},
		both(tool{
			name:        "dashboard_vm_containers_list",
			description: "Every container of one Docker VM, whoever owns it, stopped ones too.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/containers",
			params:      []Parameter{vmUUID()},
			readOnly:    true,
		}, "Every container of one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_container_show",
			description: "One container of a Docker VM, whoever owns it.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/containers/{id}",
			params:      containerParams,
			readOnly:    true,
		}, "One container of one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_container_delete",
			description: "Remove a container from a Docker VM, whoever owns it; one that is running only with force. Its volumes stay.",
			route:       "DELETE /api/dashboard/workload/vms/{uuid}/containers/{id}",
			params:      append(slices.Clone(containerParams), boolean("force", "remove it even while it runs")),
			destructive: true,
			idempotent:  true,
		}, "Remove a container from one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_container_start",
			description: "Start a container that is not running, whoever owns its VM.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/containers/{id}/start",
			params:      containerParams,
			idempotent:  true,
		}, "Start a container in one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_container_stop",
			description: "Stop a container, whoever owns its VM, giving it docker's grace period to shut down on its own first.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/containers/{id}/stop",
			params:      containerParams,
			idempotent:  true,
		}, "Stop a container in one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_container_restart",
			description: "Stop a container and start it again, whoever owns its VM.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/containers/{id}/restart",
			params:      containerParams,
		}, "Restart a container in one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_container_logs",
			description: "The tail of what a container has written, whoever owns its VM.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/containers/{id}/logs",
			params:      append(slices.Clone(containerParams), logParams()...),
			readOnly:    true,
		}, "The tail of what a container in one of this session owner's own Docker VMs has written."),
		both(tool{
			name:        "dashboard_container_stats",
			description: "One sample of what a container uses, whoever owns its VM: CPU, and memory, network and block counters in bytes.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/containers/{id}/stats",
			params:      containerParams,
			readOnly:    true,
		}, "One sample of what a container in one of this session owner's own Docker VMs uses."),
		both(tool{
			name:        "dashboard_container_network_connect",
			description: "Attach a container to another docker network of its VM, whoever owns it, under the aliases its neighbours there reach it by.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/containers/{id}/networks",
			params:      containerParams,
			body:        body[dashboardConnectNetwork.Request](),
		}, "Attach a container in one of this session owner's own Docker VMs to another network of the VM."),
		both(tool{
			name:        "dashboard_container_network_disconnect",
			description: "Detach a container from one of its VM's docker networks, whoever owns it.",
			route:       "DELETE /api/dashboard/workload/vms/{uuid}/containers/{id}/networks/{network}",
			params:      append(slices.Clone(containerParams), text("network", "the network's id or its name")),
			idempotent:  true,
		}, "Detach a container in one of this session owner's own Docker VMs from one of the VM's networks."),
	)
}

// dockerObjectTools are a Docker VM's images, networks and volumes, which its
// containers are made from, meet on and keep what they write in. None of them
// reaches past the VM.
func dockerObjectTools() []tool {
	return slices.Concat(
		both(tool{
			name:        "dashboard_images_list",
			description: "The images a Docker VM holds, whoever owns it.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/images",
			params:      []Parameter{vmUUID()},
			readOnly:    true,
		}, "The images one of this session owner's own Docker VMs holds."),
		both(tool{
			name:        "dashboard_image_pull",
			description: "Pull an image into a Docker VM, whoever owns it. A pull takes as long as the registry does, and carries on after whoever asked has stopped waiting.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/images",
			params:      []Parameter{vmUUID()},
			body:        body[dashboardPullImage.Request](),
			idempotent:  true,
		}, "Pull an image into one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_image_delete",
			description: "Remove an image from a Docker VM, whoever owns it; one a container was created from only with force.",
			route:       "DELETE /api/dashboard/workload/vms/{uuid}/images/{id}",
			params:      []Parameter{vmUUID(), text("id", "the image's id or one of its tags"), boolean("force", "remove it even while a container uses it")},
			destructive: true,
			idempotent:  true,
		}, "Remove an image from one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_networks_list",
			description: "The docker networks of a Docker VM, whoever owns it.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/networks",
			params:      []Parameter{vmUUID()},
			readOnly:    true,
		}, "The docker networks of one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_network_create",
			description: "Create a docker network in a Docker VM, whoever owns it, for its containers to meet on.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/networks",
			params:      []Parameter{vmUUID()},
			body:        body[dashboardCreateNetwork.Request](),
		}, "Create a docker network in one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_network_delete",
			description: "Remove a docker network from a Docker VM, whoever owns it; one a container is attached to is refused.",
			route:       "DELETE /api/dashboard/workload/vms/{uuid}/networks/{id}",
			params:      []Parameter{vmUUID(), text("id", "the network's id or its name")},
			destructive: true,
			idempotent:  true,
		}, "Remove a docker network from one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_volumes_list",
			description: "The volumes of a Docker VM, whoever owns it.",
			route:       "GET /api/dashboard/workload/vms/{uuid}/volumes",
			params:      []Parameter{vmUUID()},
			readOnly:    true,
		}, "The volumes of one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_volume_create",
			description: "Create a volume in a Docker VM, whoever owns it, for its containers to keep what they write in.",
			route:       "POST /api/dashboard/workload/vms/{uuid}/volumes",
			params:      []Parameter{vmUUID()},
			body:        body[dashboardCreateVolume.Request](),
		}, "Create a volume in one of this session owner's own Docker VMs."),
		both(tool{
			name:        "dashboard_volume_delete",
			description: "Remove a volume, and what was kept in it, from a Docker VM, whoever owns it; one a container mounts only with force.",
			route:       "DELETE /api/dashboard/workload/vms/{uuid}/volumes/{name}",
			params:      []Parameter{vmUUID(), text("name", "the volume's name"), boolean("force", "remove it even while a container uses it")},
			destructive: true,
			idempotent:  true,
		}, "Remove a volume, and what was kept in it, from one of this session owner's own Docker VMs."),
	)
}

// stackTools are compose projects deployed into Docker VMs. A stack is never
// edited: a different compose file is a new stack.
func stackTools() []tool {
	return slices.Concat(
		both(tool{
			name:        "dashboard_stacks_list",
			description: "A page of the stacks the workload holds, whoever owns them, without their compose files, narrowed to one VM's with vm.",
			route:       "GET /api/dashboard/workload/stacks",
			params:      []Parameter{page(), text("vm", "only the stacks deployed into the VM with this uuid")},
			readOnly:    true,
		}, "A page of this session owner's own stacks, narrowed to one VM's with vm."),
		[]tool{{
			name:        "dashboard_stack_create",
			description: "Deploy a compose project for this session's owner: in the Docker VM vm_uuid names, in a new one vm describes, or with neither in a new one made with the defaults. The deploy happens after the answer, which says which VM it went into and whether it was made for it.",
			route:       "POST /api/dashboard/workload/stacks",
			body:        body[dashboardCreateStack.Request](),
		}},
		both(tool{
			name:        "dashboard_stack_show",
			description: "One stack, whoever owns it, with its compose file, the output of its last compose command and its containers as its VM lists them now.",
			route:       "GET /api/dashboard/workload/stacks/{uuid}",
			params:      []Parameter{uuidOf("stack")},
			readOnly:    true,
		}, "One of this session owner's own stacks, with its containers."),
		both(tool{
			name:        "dashboard_stack_delete",
			description: "Take a stack down and remove it, whoever owns it; its volumes go too only with volumes.",
			route:       "DELETE /api/dashboard/workload/stacks/{uuid}",
			params:      []Parameter{uuidOf("stack"), boolean("volumes", "remove the stack's volumes too")},
			destructive: true,
			idempotent:  true,
		}, "Take one of this session owner's own stacks down and remove it."),
		both(tool{
			name:        "dashboard_stack_start",
			description: "Start a stopped stack's containers again, whoever owns it.",
			route:       "POST /api/dashboard/workload/stacks/{uuid}/start",
			params:      []Parameter{uuidOf("stack")},
			idempotent:  true,
		}, "Start one of this session owner's own stacks."),
		both(tool{
			name:        "dashboard_stack_stop",
			description: "Stop a stack's containers, and keep them, whoever owns it.",
			route:       "POST /api/dashboard/workload/stacks/{uuid}/stop",
			params:      []Parameter{uuidOf("stack")},
			idempotent:  true,
		}, "Stop one of this session owner's own stacks."),
		both(tool{
			name:        "dashboard_stack_restart",
			description: "Restart a stack's containers, whoever owns it.",
			route:       "POST /api/dashboard/workload/stacks/{uuid}/restart",
			params:      []Parameter{uuidOf("stack")},
		}, "Restart one of this session owner's own stacks."),
	)
}
