package providers

import (
	"context"
	"log"
	"net/http"
	"time"

	getArticle "github.com/khanzadimahdi/testproject/application/article/getArticle"
	getArticles "github.com/khanzadimahdi/testproject/application/article/getArticles"
	"github.com/khanzadimahdi/testproject/application/article/getArticlesByHashtag"
	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/auth/forgetpassword"
	"github.com/khanzadimahdi/testproject/application/auth/login"
	"github.com/khanzadimahdi/testproject/application/auth/refresh"
	"github.com/khanzadimahdi/testproject/application/auth/register"
	"github.com/khanzadimahdi/testproject/application/auth/resetpassword"
	"github.com/khanzadimahdi/testproject/application/auth/verify"
	"github.com/khanzadimahdi/testproject/application/bookmark/bookmarkExists"
	"github.com/khanzadimahdi/testproject/application/bookmark/updateBookmark"
	"github.com/khanzadimahdi/testproject/application/code/heartbeat"
	"github.com/khanzadimahdi/testproject/application/code/runCode"
	"github.com/khanzadimahdi/testproject/application/comment/createComment"
	"github.com/khanzadimahdi/testproject/application/comment/getComments"
	dashboardCreateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/createArticle"
	dashboardDeleteArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
	dashboardGetArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticle"
	dashboardGetArticles "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticles"
	dashboardUpdateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateArticle"
	dashboardDeleteUserBookmark "github.com/khanzadimahdi/testproject/application/dashboard/bookmark/deleteUserBookmark"
	dashboardGetUserBookmarks "github.com/khanzadimahdi/testproject/application/dashboard/bookmark/getUserBookmarks"
	dashboardCreateComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/createComment"
	dashboardDeleteComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteComment"
	dashboardDeleteUserComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteUserComment"
	dashboardGetComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/getComment"
	dashboardGetComments "github.com/khanzadimahdi/testproject/application/dashboard/comment/getComments"
	dashboardGetUserComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/getUserComment"
	dashboardGetUserComments "github.com/khanzadimahdi/testproject/application/dashboard/comment/getUserComments"
	dashboardUpdateComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/updateComment"
	dashboardUpdateUserComment "github.com/khanzadimahdi/testproject/application/dashboard/comment/updateUserComment"
	dashboardGetConfig "github.com/khanzadimahdi/testproject/application/dashboard/config/getConfig"
	dashboardUpdateConfig "github.com/khanzadimahdi/testproject/application/dashboard/config/updateConfig"
	dashboardCreateElement "github.com/khanzadimahdi/testproject/application/dashboard/element/createElement"
	dashboardDeleteElement "github.com/khanzadimahdi/testproject/application/dashboard/element/deleteElement"
	dashboardGetElement "github.com/khanzadimahdi/testproject/application/dashboard/element/getElement"
	dashboardGetElements "github.com/khanzadimahdi/testproject/application/dashboard/element/getElements"
	dashboardUpdateElement "github.com/khanzadimahdi/testproject/application/dashboard/element/updateElement"
	dashboardDeleteFile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteFile"
	dashboardDeleteUserFile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteUserFile"
	dashboardGetFile "github.com/khanzadimahdi/testproject/application/dashboard/file/getFile"
	dashboardGetFiles "github.com/khanzadimahdi/testproject/application/dashboard/file/getFiles"
	dashboardGetUserFiles "github.com/khanzadimahdi/testproject/application/dashboard/file/getUserFiles"
	dashboardUploadFile "github.com/khanzadimahdi/testproject/application/dashboard/file/uploadFile"
	dashboardGetPermissions "github.com/khanzadimahdi/testproject/application/dashboard/permission/getPermissions"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/changepassword"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/getRoles"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/getprofile"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/updateprofile"
	dashboardCreateRole "github.com/khanzadimahdi/testproject/application/dashboard/role/createRole"
	dashboardDeleteRole "github.com/khanzadimahdi/testproject/application/dashboard/role/deleteRole"
	dashboardGetRole "github.com/khanzadimahdi/testproject/application/dashboard/role/getRole"
	dashboardGetRoles "github.com/khanzadimahdi/testproject/application/dashboard/role/getRoles"
	dashboardUpdateRole "github.com/khanzadimahdi/testproject/application/dashboard/role/updateRole"
	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	getuser "github.com/khanzadimahdi/testproject/application/dashboard/user/getUser"
	getusers "github.com/khanzadimahdi/testproject/application/dashboard/user/getUsers"
	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
	"github.com/khanzadimahdi/testproject/application/dashboard/user/userchangepassword"
	"github.com/khanzadimahdi/testproject/application/element"
	getFile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/application/home"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/permission"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/cache"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	articlesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/articles"
	bookmarksrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/bookmarks"
	commentsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/comments"
	configrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/config"
	elementsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/elements"
	filesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/files"
	permissionsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/permissions"
	rolesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/roles"
	userrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/users"
	articleAPI "github.com/khanzadimahdi/testproject/presentation/http/api/article"
	authAPI "github.com/khanzadimahdi/testproject/presentation/http/api/auth"
	bookmarkAPI "github.com/khanzadimahdi/testproject/presentation/http/api/bookmark"
	commentAPI "github.com/khanzadimahdi/testproject/presentation/http/api/comment"
	dashboardArticleAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/article"
	dashboardBookmarkAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/bookmark"
	dashboardCommentAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/comment"
	dashboardConfigAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/config"
	dashboardElementAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/element"
	dashboardFileAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/file"
	dashboardPermissionAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/permission"
	"github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/profile"
	dashboardRoleAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/role"
	dashboardUserAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/user"
	fileAPI "github.com/khanzadimahdi/testproject/presentation/http/api/file"
	hashtagAPI "github.com/khanzadimahdi/testproject/presentation/http/api/hashtag"
	homeapi "github.com/khanzadimahdi/testproject/presentation/http/api/home"
	"github.com/khanzadimahdi/testproject/presentation/http/api/websocket"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	BlogSubscribers         = "blog:subscribers"
	BlogRequestReplyers     = "blog:requestReplyers"
	BlogHandler             = "blog:handler"
	BlogHTTPCacheBucketName = "blog_http_cache"

	WebSocketWriteWait        = 10 * time.Second
	WebSocketMaxMessageSize   = 256 * 1024 // 256KB
	WebSocketPongWait         = 60 * time.Second
	WebSocketPingPeriod       = (WebSocketPongWait * 9) / 10
	WebSocketCloseGracePeriod = 10 * time.Second
)

type blogProvider struct {
	dependencies []ioc.ServiceProvider
}

var blogDependencies = []ioc.ServiceProvider{
	NewMongodbProvider(),
	NewNatsProvider(),
	NewTranslationProvider(),
	NewValidationProvider(),
	NewEmailProvider(),
	NewHasherProvider(),
	NewJwtProvider(),
	NewAuthProvider(),
	NewStorageProvider(),
	NewTemplateProvider(),
	NewContainerProvider(),
}

var _ ioc.ServiceProvider = &blogProvider{}

func NewBlogProvider() *blogProvider {
	return &blogProvider{
		dependencies: blogDependencies,
	}
}

func (p *blogProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	for _, dependency := range p.dependencies {
		if err := dependency.Register(ctx, iocContainer); err != nil {
			return err
		}
	}

	return nil
}

func (p *blogProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	for _, dependency := range p.dependencies {
		if err := dependency.Boot(ctx, iocContainer); err != nil {
			return err
		}
	}

	return iocContainer.Singleton(blog, ioc.WithNameBinding(BlogHandler))
}

func (p *blogProvider) Terminate() error {
	for _, dependency := range p.dependencies {
		defer dependency.Terminate()
	}

	return nil
}

func blog(
	database *mongo.Database,
	jwt *jwt.JWT,
	hasher password.Hasher,
	asyncPublishSubscriber domain.PublishSubscriber,
	translator translatorContract.Translator,
	validator domain.Validator,
	fileStorage file.Storage,
	authorizer domain.Authorizer,
	mailer domain.Mailer,
	renderer domain.Renderer,
	iocContainer ioc.ServiceContainer,
) (http.Handler, error) {
	var mailFromAddress string
	if err := iocContainer.Resolve(&mailFromAddress, ioc.WithNameResolving("mailFromAddress")); err != nil {
		return nil, err
	}

	var jetStreamRequester domain.Requester
	if err := iocContainer.Resolve(&jetStreamRequester, ioc.WithNameResolving(BlogRequestReplyer)); err != nil {
		return nil, err
	}

	var natsConnection *nats.Conn
	if err := iocContainer.Resolve(&natsConnection); err != nil {
		return nil, err
	}

	httpCache, err := cache.NewNatsCache(
		natsConnection,
		BlogHTTPCacheBucketName,
		cache.WithTTL(1*time.Minute),
		cache.WithLimitMarkerTTL(1*time.Second),
		cache.WithCompression(true),
	)
	if err != nil {
		return nil, err
	}

	var asyncReplyChan chan *domain.Reply
	if err := iocContainer.Resolve(&asyncReplyChan, ioc.WithNameResolving(BlogRequestReplyerChannel)); err != nil {
		return nil, err
	}

	articlesRepository := articlesrepository.NewRepository(database)
	commentsRepository := commentsrepository.NewRepository(database)
	filesRepository := filesrepository.NewRepository(database)
	elementsRepository := elementsrepository.NewRepository(database)
	userRepository := userrepository.NewRepository(database)
	permissionRepository := permissionsrepository.NewRepository()
	rolesRepository := rolesrepository.NewRepository(database)
	bookmarkRepository := bookmarksrepository.NewRepository(database)
	configRepository := configrepository.NewRepository(database)

	authTokenGenerator := auth.NewTokenGenerator(jwt, rolesRepository)
	elementRetriever := element.NewRetriever(articlesRepository, elementsRepository)

	// ---- public ----
	homeUseCase := home.NewUseCase(articlesRepository, elementRetriever)

	loginUseCase := login.NewUseCase(userRepository, authTokenGenerator, hasher, translator, validator)
	refreshUseCase := refresh.NewUseCase(userRepository, jwt, authTokenGenerator, translator, validator)
	forgetPasswordUseCase := forgetpassword.NewUseCase(userRepository, asyncPublishSubscriber, translator, validator)
	resetPasswordUseCase := resetpassword.NewUseCase(userRepository, hasher, jwt, translator, validator)
	registerUseCase := register.NewUseCase(userRepository, asyncPublishSubscriber, translator, validator)
	verifyUseCase := verify.NewUseCase(userRepository, rolesRepository, configRepository, hasher, jwt, translator, validator)

	getArticleUsecase := getArticle.NewUseCase(articlesRepository, elementRetriever)
	getArticlesUsecase := getArticles.NewUseCase(articlesRepository)
	getArticlesByHashtagUseCase := getArticlesByHashtag.NewUseCase(articlesRepository, validator)
	getFileUseCase := getFile.NewUseCase(filesRepository, fileStorage)
	getCommentsUseCase := getComments.NewUseCase(commentsRepository, userRepository, validator)
	createCommentUseCase := createComment.NewUseCase(commentsRepository, validator)
	bookmarkExistsUseCase := bookmarkExists.NewUseCase(bookmarkRepository, validator)
	updateABookmark := updateBookmark.NewUseCase(bookmarkRepository, validator)

	// ---- dashboard ----
	getProfileUseCase := getprofile.NewUseCase(userRepository)
	updateProfileUseCase := updateprofile.NewUseCase(userRepository, validator, translator)
	dashboardProfileChangePasswordUseCase := changepassword.NewUseCase(userRepository, hasher, validator, translator)
	dashboardProfileGetRolesUseCase := getRoles.NewUseCase(rolesRepository)

	dashboardCreateArticleUsecase := dashboardCreateArticle.NewUseCase(articlesRepository, validator)
	dashboardDeleteArticleUsecase := dashboardDeleteArticle.NewUseCase(articlesRepository)
	dashboardGetArticleUsecase := dashboardGetArticle.NewUseCase(articlesRepository)
	dashboardGetArticlesUsecase := dashboardGetArticles.NewUseCase(articlesRepository)
	dashboardUpdateArticleUsecase := dashboardUpdateArticle.NewUseCase(articlesRepository, validator)

	dashboardCreateCommentUsecase := dashboardCreateComment.NewUseCase(commentsRepository, validator)
	dashboardDeleteCommentUsecase := dashboardDeleteComment.NewUseCase(commentsRepository)
	dashboardGetCommentUsecase := dashboardGetComment.NewUseCase(commentsRepository, userRepository)
	dashboardGetCommentsUsecase := dashboardGetComments.NewUseCase(commentsRepository, userRepository)
	dashboardUpdateCommentUsecase := dashboardUpdateComment.NewUseCase(commentsRepository, validator)

	dashboardDeleteUserCommentUsecase := dashboardDeleteUserComment.NewUseCase(commentsRepository)
	dashboardGetUserCommentUsecase := dashboardGetUserComment.NewUseCase(commentsRepository, userRepository)
	dashboardGetUserCommentsUsecase := dashboardGetUserComments.NewUseCase(commentsRepository, userRepository)
	dashboardUpdateUserCommentUsecase := dashboardUpdateUserComment.NewUseCase(commentsRepository, validator)

	dashboardDeleteUserBookmarkUsecase := dashboardDeleteUserBookmark.NewUseCase(bookmarkRepository, validator)
	dashboardGetUserBookmarksUsecase := dashboardGetUserBookmarks.NewUseCase(bookmarkRepository, validator)

	dashboardCreateUserUsecase := createuser.NewUseCase(userRepository, hasher, validator, translator)
	dashboardDeleteUserUsecase := deleteuser.NewUseCase(userRepository)
	dashboardGetUserUsecase := getuser.NewUseCase(userRepository)
	dashboardGetUsersUsecase := getusers.NewUseCase(userRepository)
	dashboardUpdateUserUsecase := updateuser.NewUseCase(userRepository, validator)
	dashboardUpdateUserChangePasswordUsecase := userchangepassword.NewUseCase(userRepository, hasher, validator)

	dashboardGetPermissionsUseCase := dashboardGetPermissions.NewUseCase(permissionRepository)

	dashboardCreateRoleUsecase := dashboardCreateRole.NewUseCase(rolesRepository, permissionRepository, validator, translator)
	dashboardDeleteRoleUsecase := dashboardDeleteRole.NewUseCase(rolesRepository)
	dashboardGetRoleUsecase := dashboardGetRole.NewUseCase(rolesRepository)
	dashboardGetRolesUsecase := dashboardGetRoles.NewUseCase(rolesRepository)
	dashboardUpdateRoleUsecase := dashboardUpdateRole.NewUseCase(rolesRepository, permissionRepository, validator, translator)

	dashboardGetFilesUseCase := dashboardGetFiles.NewUseCase(filesRepository)
	dashboardGetFileUseCase := dashboardGetFile.NewUseCase(filesRepository, fileStorage)
	dashboardUploadFileUseCase := dashboardUploadFile.NewUseCase(filesRepository, fileStorage, validator)
	dashboardDeleteFileUseCase := dashboardDeleteFile.NewUseCase(filesRepository, fileStorage)

	dashboardGetUserFilesUseCase := dashboardGetUserFiles.NewUseCase(filesRepository)
	dashboardDeleteUserFileUseCase := dashboardDeleteUserFile.NewUseCase(filesRepository, fileStorage)

	dashboardCreateElementUsecase := dashboardCreateElement.NewUseCase(elementsRepository, validator)
	dashboardDeleteElementUsecase := dashboardDeleteElement.NewUseCase(elementsRepository)
	dashboardGetElementUsecase := dashboardGetElement.NewUseCase(elementsRepository)
	dashboardGetElementsUsecase := dashboardGetElements.NewUseCase(elementsRepository)
	dashboardUpdateElementUsecase := dashboardUpdateElement.NewUseCase(elementsRepository, validator)

	dashboardGetConfigUsecase := dashboardGetConfig.NewUseCase(configRepository)
	dashboardUpdateConfigUsecase := dashboardUpdateConfig.NewUseCase(configRepository, validator)

	mux := http.NewServeMux()

	// ---- public HTTP API ----

	// websocket
	mux.Handle("GET /api/ws", websocket.NewWsHandler(
		WebSocketWriteWait,
		WebSocketMaxMessageSize,
		WebSocketPongWait,
		WebSocketPingPeriod,
		WebSocketCloseGracePeriod,
		asyncReplyChan,
		jetStreamRequester,
		translator,
	))

	// home
	mux.Handle("GET /api/home", middleware.NewCacheMiddleware(homeapi.NewHomeHandler(homeUseCase), httpCache))

	// auth
	mux.Handle("POST /api/auth/login", authAPI.NewLoginHandler(loginUseCase))
	mux.Handle("POST /api/auth/token/refresh", authAPI.NewRefreshHandler(refreshUseCase))
	mux.Handle("POST /api/auth/password/forget", authAPI.NewForgetPasswordHandler(forgetPasswordUseCase))
	mux.Handle("POST /api/auth/password/reset", authAPI.NewResetPasswordHandler(resetPasswordUseCase))
	mux.Handle("POST /api/auth/register", authAPI.NewRegisterHandler(registerUseCase))
	mux.Handle("POST /api/auth/verify", authAPI.NewVerifyHandler(verifyUseCase))

	// articles
	mux.Handle("GET /api/articles", middleware.NewCacheMiddleware(articleAPI.NewIndexHandler(getArticlesUsecase), httpCache))
	mux.Handle("GET /api/articles/{uuid}", middleware.NewCacheMiddleware(articleAPI.NewShowHandler(getArticleUsecase), httpCache))

	// comments
	mux.Handle("POST /api/comments", middleware.NewAuthenticateMiddleware(commentAPI.NewCreateHandler(createCommentUseCase), jwt, userRepository))
	mux.Handle("GET /api/comments", commentAPI.NewIndexHandler(getCommentsUseCase))

	// bookmark
	mux.Handle("POST /api/bookmarks/exists", middleware.NewAuthenticateMiddleware(bookmarkAPI.NewExistsHandler(bookmarkExistsUseCase), jwt, userRepository))
	mux.Handle("PUT /api/bookmarks", middleware.NewAuthenticateMiddleware(bookmarkAPI.NewUpdateHandler(updateABookmark), jwt, userRepository))

	// hashtags
	mux.Handle("GET /api/hashtags/{hashtag}", middleware.NewCacheMiddleware(hashtagAPI.NewShowHandler(getArticlesByHashtagUseCase), httpCache))

	// files
	mux.Handle("GET /files/{uuid}", middleware.NewCacheMiddleware(fileAPI.NewShowHandler(getFileUseCase), httpCache))

	// ---- dashboard HTTP API ----

	// profile
	mux.Handle("GET /api/dashboard/profile", middleware.NewAuthenticateMiddleware(profile.NewGetProfileHandler(getProfileUseCase), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/profile", middleware.NewAuthenticateMiddleware(profile.NewUpdateProfileHandler(updateProfileUseCase), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/password", middleware.NewAuthenticateMiddleware(profile.NewChangePasswordHandler(dashboardProfileChangePasswordUseCase), jwt, userRepository))
	mux.Handle("GET /api/dashboard/profile/roles", middleware.NewAuthenticateMiddleware(profile.NewGetRolesHandler(dashboardProfileGetRolesUseCase), jwt, userRepository))

	// user
	mux.Handle("POST /api/dashboard/users", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewCreateHandler(dashboardCreateUserUsecase), authorizer, permission.UsersCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/users/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewDeleteHandler(dashboardDeleteUserUsecase), authorizer, permission.UsersDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/users", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewIndexHandler(dashboardGetUsersUsecase), authorizer, permission.UsersIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/users/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewShowHandler(dashboardGetUserUsecase), authorizer, permission.UsersShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/users", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewUpdateHandler(dashboardUpdateUserUsecase), authorizer, permission.UsersUpdate), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/users/password", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewChangePasswordHandler(dashboardUpdateUserChangePasswordUsecase), authorizer, permission.UsersPasswordUpdate), jwt, userRepository))

	// permissions
	mux.Handle("GET /api/dashboard/permissions", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardPermissionAPI.NewIndexHandler(dashboardGetPermissionsUseCase), authorizer, permission.PermissionsIndex), jwt, userRepository))

	// roles
	mux.Handle("POST /api/dashboard/roles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewCreateHandler(dashboardCreateRoleUsecase), authorizer, permission.RolesCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/roles/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewDeleteHandler(dashboardDeleteRoleUsecase), authorizer, permission.RolesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/roles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewIndexHandler(dashboardGetRolesUsecase), authorizer, permission.RolesIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/roles/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewShowHandler(dashboardGetRoleUsecase), authorizer, permission.RolesShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/roles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewUpdateHandler(dashboardUpdateRoleUsecase), authorizer, permission.RolesUpdate), jwt, userRepository))

	// articles
	mux.Handle("POST /api/dashboard/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewCreateHandler(dashboardCreateArticleUsecase), authorizer, permission.ArticlesCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/articles/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewDeleteHandler(dashboardDeleteArticleUsecase), authorizer, permission.ArticlesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewIndexHandler(dashboardGetArticlesUsecase), authorizer, permission.ArticlesIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/articles/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewShowHandler(dashboardGetArticleUsecase), authorizer, permission.ArticlesShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewUpdateHandler(dashboardUpdateArticleUsecase), authorizer, permission.ArticlesUpdate), jwt, userRepository))

	// comments
	mux.Handle("POST /api/dashboard/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewCreateHandler(dashboardCreateCommentUsecase), authorizer, permission.CommentsCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewDeleteHandler(dashboardDeleteCommentUsecase), authorizer, permission.CommentsDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewIndexHandler(dashboardGetCommentsUsecase), authorizer, permission.CommentsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewShowHandler(dashboardGetCommentUsecase), authorizer, permission.CommentsShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewUpdateHandler(dashboardUpdateCommentUsecase), authorizer, permission.CommentsUpdate), jwt, userRepository))

	// self comments
	mux.Handle("DELETE /api/dashboard/my/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewDeleteUserCommentHandler(dashboardDeleteUserCommentUsecase), authorizer, permission.SelfCommentsDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewIndexUserCommentsHandler(dashboardGetUserCommentsUsecase), authorizer, permission.SelfCommentsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewShowUserCommentHandler(dashboardGetUserCommentUsecase), authorizer, permission.SelfCommentsShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/my/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewUpdateUserCommentHandler(dashboardUpdateUserCommentUsecase), authorizer, permission.SelfCommentsUpdate), jwt, userRepository))

	// self bookmarks
	mux.Handle("DELETE /api/dashboard/my/bookmarks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardBookmarkAPI.NewDeleteUserBookmarkHandler(dashboardDeleteUserBookmarkUsecase), authorizer, permission.SelfBookmarksDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/bookmarks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardBookmarkAPI.NewIndexUserBookmarksHandler(dashboardGetUserBookmarksUsecase), authorizer, permission.SelfBookmarksIndex), jwt, userRepository))

	// files
	mux.Handle("POST /api/dashboard/files", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewUploadHandler(dashboardUploadFileUseCase), authorizer, permission.FilesCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/files/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewDeleteHandler(dashboardDeleteFileUseCase), authorizer, permission.FilesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/files", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewIndexHandler(dashboardGetFilesUseCase), authorizer, permission.FilesIndex), jwt, userRepository))
	mux.Handle("GET /dashboard/files/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewShowHandler(dashboardGetFileUseCase), authorizer, permission.FilesShow), jwt, userRepository))

	// self files
	mux.Handle("DELETE /api/dashboard/my/files/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewDeleteUserHandler(dashboardDeleteUserFileUseCase), authorizer, permission.SelfFilesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/files", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewIndexUserHandler(dashboardGetUserFilesUseCase), authorizer, permission.SelfFilesIndex), jwt, userRepository))

	// elements
	mux.Handle("POST /api/dashboard/elements", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewCreateHandler(dashboardCreateElementUsecase), authorizer, permission.ElementsCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/elements/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewDeleteHandler(dashboardDeleteElementUsecase), authorizer, permission.ElementsDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/elements", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewIndexHandler(dashboardGetElementsUsecase), authorizer, permission.ElementsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/elements/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewShowHandler(dashboardGetElementUsecase), authorizer, permission.ElementsShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/elements", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewUpdateHandler(dashboardUpdateElementUsecase), authorizer, permission.ElementsUpdate), jwt, userRepository))

	// config
	mux.Handle("GET /api/dashboard/config", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardConfigAPI.NewShowHandler(dashboardGetConfigUsecase), authorizer, permission.ConfigShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/config", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardConfigAPI.NewUpdateHandler(dashboardUpdateConfigUsecase), authorizer, permission.ConfigUpdate), jwt, userRepository))

	handler := middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute))

	// request replyers
	requestReplyers := map[string]domain.Replyer{
		runCode.RunCodeRequest: runCode.NewRunCodeHandler(validator, asyncPublishSubscriber),
	}

	if err := iocContainer.Singleton(func() map[string]domain.Replyer {
		return requestReplyers
	}, ioc.WithNameBinding(BlogRequestReplyers)); err != nil {
		return nil, err
	}

	// subscribers
	subscribers := map[string]domain.MessageHandler{
		forgetpassword.SendForgetPasswordEmailName: forgetpassword.NewSendForgetPasswordEmailHandler(userRepository, authTokenGenerator, mailer, mailFromAddress, renderer),
		register.SendRegisterationEmailName:        register.NewSendRegisterationEmailHandler(authTokenGenerator, mailer, mailFromAddress, renderer),
		taskEvents.HeartbeatName:                   heartbeat.NewHeartbeatHandler(asyncReplyChan),
	}

	if err := iocContainer.Singleton(func() map[string]domain.MessageHandler {
		return subscribers
	}, ioc.WithNameBinding(BlogSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
