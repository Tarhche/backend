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
	dashboardRunStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/runStack"
	dashboardRunTask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/runTask"
	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
	"github.com/khanzadimahdi/testproject/application/dashboard/user/userchangepassword"
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

func taskUUID() Parameter {
	return uuidOf("task")
}

func stackUUID() Parameter {
	return uuidOf("stack")
}

func logParams(what string) []Parameter {
	return []Parameter{
		uuidOf(what),
		text("after", "only lines written after this moment, as an RFC 3339 timestamp"),
		integer("limit", "how many lines to return"),
	}
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
		taskTools(),
		stackTools(),
		configTools(),
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

// taskTools are the runner's: a task is one long-running container, described
// the way a compose service is.
func taskTools() []tool {
	return []tool{
		{
			name:        "dashboard_tasks_list",
			description: "A page of the tasks the runner is holding, whoever owns them.",
			route:       "GET /api/dashboard/runner/tasks",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_task_show",
			description: "One task, with the addresses its exposed ports are served at.",
			route:       "GET /api/dashboard/runner/tasks/{uuid}",
			params:      []Parameter{taskUUID()},
			readOnly:    true,
		},
		{
			name:        "dashboard_task_run",
			description: "Run a container from a compose service specification. It keeps running until it is stopped, and the ports it exposes are served under a name of its own.",
			route:       "POST /api/dashboard/runner/tasks",
			body:        body[dashboardRunTask.Request](),
		},
		{
			name:        "dashboard_task_logs",
			description: "What a task has written so far, from its first line onward. Following it as it writes is a websocket rather than a tool.",
			route:       "GET /api/dashboard/runner/tasks/{uuid}/logs",
			params:      logParams("task"),
			readOnly:    true,
		},
		{
			name:        "dashboard_task_stop",
			description: "Stop a task, giving it a moment to shut down on its own.",
			route:       "POST /api/dashboard/runner/tasks/{uuid}/stop",
			params:      []Parameter{taskUUID()},
			idempotent:  true,
		},
		{
			name:        "dashboard_task_kill",
			description: "Stop a task at once, without a grace period.",
			route:       "POST /api/dashboard/runner/tasks/{uuid}/kill",
			params:      []Parameter{taskUUID()},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "dashboard_task_restart",
			description: "Stop a task and start it again in place.",
			route:       "POST /api/dashboard/runner/tasks/{uuid}/restart",
			params:      []Parameter{taskUUID()},
		},
		{
			name:        "dashboard_task_delete",
			description: "Remove a task and everything it holds: its ports, its log and the container itself.",
			route:       "DELETE /api/dashboard/runner/tasks/{uuid}",
			params:      []Parameter{taskUUID()},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "my_tasks_list",
			description: "A page of the tasks this session's owner started.",
			route:       "GET /api/dashboard/my/runner/tasks",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "my_task_show",
			description: "One task this session's owner started.",
			route:       "GET /api/dashboard/my/runner/tasks/{uuid}",
			params:      []Parameter{taskUUID()},
			readOnly:    true,
		},
		{
			name:        "my_task_logs",
			description: "What one of this session owner's own tasks has written.",
			route:       "GET /api/dashboard/my/runner/tasks/{uuid}/logs",
			params:      logParams("task"),
			readOnly:    true,
		},
		{
			name:        "my_task_stop",
			description: "Stop one of this session owner's own tasks.",
			route:       "POST /api/dashboard/my/runner/tasks/{uuid}/stop",
			params:      []Parameter{taskUUID()},
			idempotent:  true,
		},
		{
			name:        "my_task_kill",
			description: "Stop one of this session owner's own tasks at once.",
			route:       "POST /api/dashboard/my/runner/tasks/{uuid}/kill",
			params:      []Parameter{taskUUID()},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "my_task_restart",
			description: "Restart one of this session owner's own tasks.",
			route:       "POST /api/dashboard/my/runner/tasks/{uuid}/restart",
			params:      []Parameter{taskUUID()},
		},
		{
			name:        "my_task_delete",
			description: "Remove one of this session owner's own tasks.",
			route:       "DELETE /api/dashboard/my/runner/tasks/{uuid}",
			params:      []Parameter{taskUUID()},
			destructive: true,
			idempotent:  true,
		},
	}
}

// stackTools are for several services run together, the way a compose file
// describes them: they share a private network and reach each other by the
// names they are keyed under.
func stackTools() []tool {
	return []tool{
		{
			name:        "dashboard_stacks_list",
			description: "A page of the stacks the runner is holding, whoever owns them.",
			route:       "GET /api/dashboard/runner/stacks",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "dashboard_stack_show",
			description: "One stack and the services in it.",
			route:       "GET /api/dashboard/runner/stacks/{uuid}",
			params:      []Parameter{stackUUID()},
			readOnly:    true,
		},
		{
			name:        "dashboard_stack_run",
			description: "Run a set of connected services from a compose specification. Each service needs at least an image, and they reach each other by the names they are keyed under.",
			route:       "POST /api/dashboard/runner/stacks",
			body:        body[dashboardRunStack.Request](),
		},
		{
			name:        "dashboard_stack_stop",
			description: "Stop every service of a stack, giving each a moment to shut down on its own.",
			route:       "POST /api/dashboard/runner/stacks/{uuid}/stop",
			params:      []Parameter{stackUUID()},
			idempotent:  true,
		},
		{
			name:        "dashboard_stack_kill",
			description: "Stop every service of a stack at once.",
			route:       "POST /api/dashboard/runner/stacks/{uuid}/kill",
			params:      []Parameter{stackUUID()},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "dashboard_stack_restart",
			description: "Restart every service of a stack.",
			route:       "POST /api/dashboard/runner/stacks/{uuid}/restart",
			params:      []Parameter{stackUUID()},
		},
		{
			name:        "dashboard_stack_delete",
			description: "Remove a stack and everything it holds: its services, their ports, their logs and the network they shared.",
			route:       "DELETE /api/dashboard/runner/stacks/{uuid}",
			params:      []Parameter{stackUUID()},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "my_stacks_list",
			description: "A page of the stacks this session's owner started.",
			route:       "GET /api/dashboard/my/runner/stacks",
			params:      []Parameter{page()},
			readOnly:    true,
		},
		{
			name:        "my_stack_show",
			description: "One stack this session's owner started.",
			route:       "GET /api/dashboard/my/runner/stacks/{uuid}",
			params:      []Parameter{stackUUID()},
			readOnly:    true,
		},
		{
			name:        "my_stack_stop",
			description: "Stop every service of one of this session owner's own stacks.",
			route:       "POST /api/dashboard/my/runner/stacks/{uuid}/stop",
			params:      []Parameter{stackUUID()},
			idempotent:  true,
		},
		{
			name:        "my_stack_kill",
			description: "Stop every service of one of this session owner's own stacks at once.",
			route:       "POST /api/dashboard/my/runner/stacks/{uuid}/kill",
			params:      []Parameter{stackUUID()},
			destructive: true,
			idempotent:  true,
		},
		{
			name:        "my_stack_restart",
			description: "Restart every service of one of this session owner's own stacks.",
			route:       "POST /api/dashboard/my/runner/stacks/{uuid}/restart",
			params:      []Parameter{stackUUID()},
		},
		{
			name:        "my_stack_delete",
			description: "Remove one of this session owner's own stacks and everything it holds.",
			route:       "DELETE /api/dashboard/my/runner/stacks/{uuid}",
			params:      []Parameter{stackUUID()},
			destructive: true,
			idempotent:  true,
		},
	}
}
