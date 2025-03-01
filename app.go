package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	getArticle "github.com/khanzadimahdi/testproject/application/article/getArticle"
	getArticles "github.com/khanzadimahdi/testproject/application/article/getArticles"
	"github.com/khanzadimahdi/testproject/application/article/getArticlesByHashtag"
	"github.com/khanzadimahdi/testproject/application/auth/forgetpassword"
	"github.com/khanzadimahdi/testproject/application/auth/login"
	"github.com/khanzadimahdi/testproject/application/auth/refresh"
	"github.com/khanzadimahdi/testproject/application/auth/register"
	"github.com/khanzadimahdi/testproject/application/auth/resetpassword"
	"github.com/khanzadimahdi/testproject/application/auth/verify"
	"github.com/khanzadimahdi/testproject/application/bookmark/bookmarkExists"
	"github.com/khanzadimahdi/testproject/application/bookmark/updateBookmark"
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
	getFile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/application/home"
	managerGetNode "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNode"
	managerGetNodes "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNodes"
	managerHeartbeatNode "github.com/khanzadimahdi/testproject/application/runner/manager/node/heartbeatNode"
	managerDeleteTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	managerGetTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	managerGetTasks "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTasks"
	managerHeartbeatTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/heartbeatTask"
	managerRunTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/runTask"
	managerStopTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/stopTask"
	workerDeleteTask "github.com/khanzadimahdi/testproject/application/runner/worker/task/deleteTask"
	workergettasks "github.com/khanzadimahdi/testproject/application/runner/worker/task/getTasks"
	workerruntask "github.com/khanzadimahdi/testproject/application/runner/worker/task/runTask"
	workerstoptask "github.com/khanzadimahdi/testproject/application/runner/worker/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/role"
	containerContract "github.com/khanzadimahdi/testproject/domain/runner/container"
	nodeEvents "github.com/khanzadimahdi/testproject/domain/runner/node/events"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/email"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream"
	articlesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/articles"
	bookmarksrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/bookmarks"
	commentsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/comments"
	configrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/config"
	elementsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/elements"
	filesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/files"
	permissionsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/permissions"
	rolesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/roles"
	noderepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/nodes"
	taskrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/runner/tasks"
	userrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/users"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/container"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/scheduler/roundrobin"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/presentation/commands/blog"
	"github.com/khanzadimahdi/testproject/presentation/commands/runner/manager"
	"github.com/khanzadimahdi/testproject/presentation/commands/runner/worker"
	articleAPI "github.com/khanzadimahdi/testproject/presentation/http/api/article"
	"github.com/khanzadimahdi/testproject/presentation/http/api/auth"
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
	managerNodeAPI "github.com/khanzadimahdi/testproject/presentation/http/api/runner/manager/node"
	managerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/api/runner/manager/task"
	workerTaskAPI "github.com/khanzadimahdi/testproject/presentation/http/api/runner/worker/task"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/resources/translation"
	"github.com/nats-io/nats.go"
)

//go:embed resources/view
var files embed.FS

// Container contains services
type Container struct {
	MongoClient                *mongo.Client
	Database                   *mongo.Database
	FileStorage                *minio.MinIO
	NATsConnection             *nats.Conn
	JetStreamPublishSubscriber domain.PublishSubscriber
	Translator                 translatorContract.Translator
	Validator                  domain.Validator
	Hasher                     password.Hasher
	TemplateRenderer           domain.Renderer
	JWT                        *jwt.JWT
	SMTPMailer                 domain.Mailer
	MailFromAddress            string
	ContainerManager           containerContract.Manager
	RoleRepository             role.Repository
	Authorizer                 domain.Authorizer
}

func NewContainer(ctx context.Context) (*Container, func()) {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	uri := fmt.Sprintf(
		"%s://%s:%s@%s:%s",
		os.Getenv("MONGO_SCHEME"),
		os.Getenv("MONGO_USERNAME"),
		os.Getenv("MONGO_PASSWORD"),
		os.Getenv("MONGO_HOST"),
		os.Getenv("MONGO_PORT"),
	)

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	database := mongoClient.Database(os.Getenv("MONGO_DATABASE_NAME"))

	useSSL, err := strconv.ParseBool(os.Getenv("S3_USE_SSL"))
	if err != nil {
		panic(err)
	}

	fileStorage, err := minio.New(minio.Options{
		Endpoint:   os.Getenv("S3_ENDPOINT"),
		AccessKey:  os.Getenv("S3_ACCESS_KEY"),
		SecretKey:  os.Getenv("S3_SECRET_KEY"),
		UseSSL:     useSSL,
		BucketName: os.Getenv("S3_BUCKET_NAME"),
	})
	if err != nil {
		panic(err)
	}

	natsConnection, err := nats.Connect(os.Getenv("NATS_URL"))
	if err != nil {
		panic(err)
	}

	jetstreamPublishSubscriber, err := jetstream.NewPublishSubscriber(natsConnection)
	if err != nil {
		panic(err)
	}

	translator := translator.New(translation.Translations, translation.FA)
	validator := validator.New(translator)

	privateKeyData := []byte(os.Getenv("PRIVATE_KEY"))
	privateKey, err := ecdsa.ParsePrivateKey(privateKeyData)
	if err != nil {
		panic(err)
	}

	j := jwt.NewJWT(privateKey, privateKey.Public())
	hasher := argon2.NewArgon2id(2, 64*1024, 2, 64)

	templateRenderer := template.NewRenderer(files, "tmpl")

	mailFromAddress := os.Getenv("MAIL_SMTP_FROM")
	mailer := email.NewSMTP(email.Config{
		Auth: email.Auth{
			Username: os.Getenv("MAIL_SMTP_USERNAME"),
			Password: os.Getenv("MAIL_SMTP_PASSWORD"),
		},
		Host: os.Getenv("MAIL_SMTP_HOST"),
		Port: os.Getenv("MAIL_SMTP_PORT"),
	})

	containerManager, err := container.NewDockerManager("tcp://docker:2375")
	if err != nil {
		panic(err)
	}

	roleRepository := rolesrepository.NewRepository(database)
	roleBasedAccessControl := domain.NewRoleBasedAccessControl(roleRepository)

	container := &Container{
		MongoClient:                mongoClient,
		Database:                   database,
		FileStorage:                fileStorage,
		NATsConnection:             natsConnection,
		JetStreamPublishSubscriber: jetstreamPublishSubscriber,
		Translator:                 translator,
		Validator:                  validator,
		Hasher:                     hasher,
		TemplateRenderer:           templateRenderer,
		JWT:                        j,
		SMTPMailer:                 mailer,
		MailFromAddress:            mailFromAddress,
		ContainerManager:           containerManager,
		RoleRepository:             roleRepository,
		Authorizer:                 roleBasedAccessControl,
	}

	shutdown := func() {
		jetstreamPublishSubscriber.Wait()
	}

	return container, shutdown
}

func Blog(container *Container) *blog.ServeCommand {
	articlesRepository := articlesrepository.NewRepository(container.Database)
	commentsRepository := commentsrepository.NewRepository(container.Database)
	filesRepository := filesrepository.NewRepository(container.Database)
	elementsRepository := elementsrepository.NewRepository(container.Database)
	userRepository := userrepository.NewRepository(container.Database)
	permissionRepository := permissionsrepository.NewRepository()
	rolesRepository := rolesrepository.NewRepository(container.Database)
	bookmarkRepository := bookmarksrepository.NewRepository(container.Database)
	configRepository := configrepository.NewRepository(container.Database)

	homeUseCase := home.NewUseCase(articlesRepository, elementsRepository)

	loginUseCase := login.NewUseCase(userRepository, container.JWT, container.Hasher, container.Translator, container.Validator)
	refreshUseCase := refresh.NewUseCase(userRepository, container.JWT, container.Translator, container.Validator)
	forgetPasswordUseCase := forgetpassword.NewUseCase(userRepository, container.JetStreamPublishSubscriber, container.Translator, container.Validator)
	resetPasswordUseCase := resetpassword.NewUseCase(userRepository, container.Hasher, container.JWT, container.Translator, container.Validator)
	registerUseCase := register.NewUseCase(userRepository, container.JetStreamPublishSubscriber, container.Translator, container.Validator)
	verifyUseCase := verify.NewUseCase(userRepository, rolesRepository, configRepository, container.Hasher, container.JWT, container.Translator, container.Validator)

	getArticleUsecase := getArticle.NewUseCase(articlesRepository, elementsRepository)
	getArticlesUsecase := getArticles.NewUseCase(articlesRepository)
	getArticlesByHashtagUseCase := getArticlesByHashtag.NewUseCase(articlesRepository, container.Validator)
	getFileUseCase := getFile.NewUseCase(filesRepository, container.FileStorage)
	getCommentsUseCase := getComments.NewUseCase(commentsRepository, userRepository, container.Validator)
	createCommentUseCase := createComment.NewUseCase(commentsRepository, container.Validator)
	bookmarkExistsUseCase := bookmarkExists.NewUseCase(bookmarkRepository, container.Validator)
	updateABookmark := updateBookmark.NewUseCase(bookmarkRepository, container.Validator)

	mux := http.NewServeMux()

	// home
	mux.Handle("GET /api/home", homeapi.NewHomeHandler(homeUseCase))

	// auth
	mux.Handle("POST /api/auth/login", auth.NewLoginHandler(loginUseCase))
	mux.Handle("POST /api/auth/token/refresh", auth.NewRefreshHandler(refreshUseCase))
	mux.Handle("POST /api/auth/password/forget", auth.NewForgetPasswordHandler(forgetPasswordUseCase))
	mux.Handle("POST /api/auth/password/reset", auth.NewResetPasswordHandler(resetPasswordUseCase))
	mux.Handle("POST /api/auth/register", auth.NewRegisterHandler(registerUseCase))
	mux.Handle("POST /api/auth/verify", auth.NewVerifyHandler(verifyUseCase))

	// articles
	mux.Handle("GET /api/articles", articleAPI.NewIndexHandler(getArticlesUsecase))
	mux.Handle("GET /api/articles/{uuid}", articleAPI.NewShowHandler(getArticleUsecase))

	// comments
	mux.Handle("POST /api/comments", middleware.NewAuthoriseMiddleware(commentAPI.NewCreateHandler(createCommentUseCase), container.JWT, userRepository))
	mux.Handle("GET /api/comments", commentAPI.NewIndexHandler(getCommentsUseCase))

	// bookmark
	mux.Handle("POST /api/bookmarks/exists", middleware.NewAuthoriseMiddleware(bookmarkAPI.NewExistsHandler(bookmarkExistsUseCase), container.JWT, userRepository))
	mux.Handle("PUT /api/bookmarks", middleware.NewAuthoriseMiddleware(bookmarkAPI.NewUpdateHandler(updateABookmark), container.JWT, userRepository))

	// hashtags
	mux.Handle("GET /api/hashtags/{hashtag}", hashtagAPI.NewShowHandler(getArticlesByHashtagUseCase))

	// files
	mux.Handle("GET /files/{uuid}", fileAPI.NewShowHandler(getFileUseCase))

	// -------------------- dashboard -------------------- //
	getProfileUseCase := getprofile.NewUseCase(userRepository)
	updateProfileUseCase := updateprofile.NewUseCase(userRepository, container.Validator, container.Translator)
	dashboardProfileChangePasswordUseCase := changepassword.NewUseCase(userRepository, container.Hasher, container.Validator, container.Translator)
	dashboardProfileGetRolesUseCase := getRoles.NewUseCase(rolesRepository)

	dashboardCreateArticleUsecase := dashboardCreateArticle.NewUseCase(articlesRepository, container.Validator)
	dashboardDeleteArticleUsecase := dashboardDeleteArticle.NewUseCase(articlesRepository)
	dashboardGetArticleUsecase := dashboardGetArticle.NewUseCase(articlesRepository)
	dashboardGetArticlesUsecase := dashboardGetArticles.NewUseCase(articlesRepository)
	dashboardUpdateArticleUsecase := dashboardUpdateArticle.NewUseCase(articlesRepository, container.Validator)

	dashboardCreateCommentUsecase := dashboardCreateComment.NewUseCase(commentsRepository, container.Validator)
	dashboardDeleteCommentUsecase := dashboardDeleteComment.NewUseCase(commentsRepository)
	dashboardGetCommentUsecase := dashboardGetComment.NewUseCase(commentsRepository, userRepository)
	dashboardGetCommentsUsecase := dashboardGetComments.NewUseCase(commentsRepository, userRepository)
	dashboardUpdateCommentUsecase := dashboardUpdateComment.NewUseCase(commentsRepository, container.Validator)

	dashboardDeleteUserCommentUsecase := dashboardDeleteUserComment.NewUseCase(commentsRepository)
	dashboardGetUserCommentUsecase := dashboardGetUserComment.NewUseCase(commentsRepository, userRepository)
	dashboardGetUserCommentsUsecase := dashboardGetUserComments.NewUseCase(commentsRepository, userRepository)
	dashboardUpdateUserCommentUsecase := dashboardUpdateUserComment.NewUseCase(commentsRepository, container.Validator)

	dashboardDeleteUserBookmarkUsecase := dashboardDeleteUserBookmark.NewUseCase(bookmarkRepository, container.Validator)
	dashboardGetUserBookmarksUsecase := dashboardGetUserBookmarks.NewUseCase(bookmarkRepository, container.Validator)

	dashboardCreateUserUsecase := createuser.NewUseCase(userRepository, container.Hasher, container.Validator, container.Translator)
	dashboardDeleteUserUsecase := deleteuser.NewUseCase(userRepository)
	dashboardGetUserUsecase := getuser.NewUseCase(userRepository)
	dashboardGetUsersUsecase := getusers.NewUseCase(userRepository)
	dashboardUpdateUserUsecase := updateuser.NewUseCase(userRepository, container.Validator)
	dashboardUpdateUserChangePasswordUsecase := userchangepassword.NewUseCase(userRepository, container.Hasher, container.Validator)

	dashboardGetPermissionsUseCase := dashboardGetPermissions.NewUseCase(permissionRepository)

	dashboardCreateRoleUsecase := dashboardCreateRole.NewUseCase(rolesRepository, permissionRepository, container.Validator, container.Translator)
	dashboardDeleteRoleUsecase := dashboardDeleteRole.NewUseCase(rolesRepository)
	dashboardGetRoleUsecase := dashboardGetRole.NewUseCase(rolesRepository)
	dashboardGetRolesUsecase := dashboardGetRoles.NewUseCase(rolesRepository)
	dashboardUpdateRoleUsecase := dashboardUpdateRole.NewUseCase(rolesRepository, permissionRepository, container.Validator, container.Translator)

	dashboardGetFilesUseCase := dashboardGetFiles.NewUseCase(filesRepository)
	dashboardGetFileUseCase := dashboardGetFile.NewUseCase(filesRepository, container.FileStorage)
	dashboardUploadFileUseCase := dashboardUploadFile.NewUseCase(filesRepository, container.FileStorage, container.Validator)
	dashboardDeleteFileUseCase := dashboardDeleteFile.NewUseCase(filesRepository, container.FileStorage)

	dashboardGetUserFilesUseCase := dashboardGetUserFiles.NewUseCase(filesRepository)
	dashboardDeleteUserFileUseCase := dashboardDeleteUserFile.NewUseCase(filesRepository, container.FileStorage)

	dashboardCreateElementUsecase := dashboardCreateElement.NewUseCase(elementsRepository)
	dashboardDeleteElementUsecase := dashboardDeleteElement.NewUseCase(elementsRepository)
	dashboardGetElementUsecase := dashboardGetElement.NewUseCase(elementsRepository)
	dashboardGetElementsUsecase := dashboardGetElements.NewUseCase(elementsRepository)
	dashboardUpdateElementUsecase := dashboardUpdateElement.NewUseCase(elementsRepository)

	dashboardGetConfigUsecase := dashboardGetConfig.NewUseCase(configRepository)
	dashboardUpdateConfigUsecase := dashboardUpdateConfig.NewUseCase(configRepository, container.Validator)

	// profile
	mux.Handle("GET /api/dashboard/profile", middleware.NewAuthoriseMiddleware(profile.NewGetProfileHandler(getProfileUseCase), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/profile", middleware.NewAuthoriseMiddleware(profile.NewUpdateProfileHandler(updateProfileUseCase), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/password", middleware.NewAuthoriseMiddleware(profile.NewChangePasswordHandler(dashboardProfileChangePasswordUseCase), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/profile/roles", middleware.NewAuthoriseMiddleware(profile.NewGetRolesHandler(dashboardProfileGetRolesUseCase), container.JWT, userRepository))

	// user
	mux.Handle("POST /api/dashboard/users", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewCreateHandler(dashboardCreateUserUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("DELETE /api/dashboard/users/{uuid}", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewDeleteHandler(dashboardDeleteUserUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/users", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewIndexHandler(dashboardGetUsersUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/users/{uuid}", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewShowHandler(dashboardGetUserUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/users", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewUpdateHandler(dashboardUpdateUserUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/users/password", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewChangePasswordHandler(dashboardUpdateUserChangePasswordUsecase, container.Authorizer), container.JWT, userRepository))

	// permissions
	mux.Handle("GET /api/dashboard/permissions", middleware.NewAuthoriseMiddleware(dashboardPermissionAPI.NewIndexHandler(dashboardGetPermissionsUseCase, container.Authorizer), container.JWT, userRepository))

	// roles
	mux.Handle("POST /api/dashboard/roles", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewCreateHandler(dashboardCreateRoleUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("DELETE /api/dashboard/roles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewDeleteHandler(dashboardDeleteRoleUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/roles", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewIndexHandler(dashboardGetRolesUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/roles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewShowHandler(dashboardGetRoleUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/roles", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewUpdateHandler(dashboardUpdateRoleUsecase, container.Authorizer), container.JWT, userRepository))

	// articles
	mux.Handle("POST /api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewCreateHandler(dashboardCreateArticleUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("DELETE /api/dashboard/articles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewDeleteHandler(dashboardDeleteArticleUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewIndexHandler(dashboardGetArticlesUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/articles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewShowHandler(dashboardGetArticleUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewUpdateHandler(dashboardUpdateArticleUsecase, container.Authorizer), container.JWT, userRepository))

	// comments
	mux.Handle("POST /api/dashboard/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewCreateHandler(dashboardCreateCommentUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("DELETE /api/dashboard/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewDeleteHandler(dashboardDeleteCommentUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewIndexHandler(dashboardGetCommentsUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewShowHandler(dashboardGetCommentUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewUpdateHandler(dashboardUpdateCommentUsecase, container.Authorizer), container.JWT, userRepository))

	// self comments
	mux.Handle("DELETE /api/dashboard/my/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewDeleteUserCommentHandler(dashboardDeleteUserCommentUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/my/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewIndexUserCommentsHandler(dashboardGetUserCommentsUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/my/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewShowUserCommentHandler(dashboardGetUserCommentUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/my/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewUpdateUserCommentHandler(dashboardUpdateUserCommentUsecase, container.Authorizer), container.JWT, userRepository))

	// self bookmarks
	mux.Handle("DELETE /api/dashboard/my/bookmarks", middleware.NewAuthoriseMiddleware(dashboardBookmarkAPI.NewDeleteUserBookmarkHandler(dashboardDeleteUserBookmarkUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/my/bookmarks", middleware.NewAuthoriseMiddleware(dashboardBookmarkAPI.NewIndexUserBookmarksHandler(dashboardGetUserBookmarksUsecase, container.Authorizer), container.JWT, userRepository))

	// files
	mux.Handle("POST /api/dashboard/files", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewUploadHandler(dashboardUploadFileUseCase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("DELETE /api/dashboard/files/{uuid}", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewDeleteHandler(dashboardDeleteFileUseCase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/files", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewIndexHandler(dashboardGetFilesUseCase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /dashboard/files/{uuid}", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewShowHandler(dashboardGetFileUseCase, container.Authorizer), container.JWT, userRepository))

	// self files
	mux.Handle("DELETE /api/dashboard/my/files/{uuid}", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewDeleteUserHandler(dashboardDeleteUserFileUseCase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/my/files", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewIndexUserHandler(dashboardGetUserFilesUseCase, container.Authorizer), container.JWT, userRepository))

	// elements
	mux.Handle("POST /api/dashboard/elements", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewCreateHandler(dashboardCreateElementUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("DELETE /api/dashboard/elements/{uuid}", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewDeleteHandler(dashboardDeleteElementUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/elements", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewIndexHandler(dashboardGetElementsUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("GET /api/dashboard/elements/{uuid}", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewShowHandler(dashboardGetElementUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/elements", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewUpdateHandler(dashboardUpdateElementUsecase, container.Authorizer), container.JWT, userRepository))

	// config
	mux.Handle("GET /api/dashboard/config", middleware.NewAuthoriseMiddleware(dashboardConfigAPI.NewShowHandler(dashboardGetConfigUsecase, container.Authorizer), container.JWT, userRepository))
	mux.Handle("PUT /api/dashboard/config", middleware.NewAuthoriseMiddleware(dashboardConfigAPI.NewUpdateHandler(dashboardUpdateConfigUsecase, container.Authorizer), container.JWT, userRepository))

	handler := middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute))

	subscribers := map[string]domain.MessageHandler{
		forgetpassword.SendForgetPasswordEmailName: forgetpassword.NewSendForgetPasswordEmailHandler(userRepository, container.JWT, container.SMTPMailer, container.MailFromAddress, container.TemplateRenderer),
		register.SendRegisterationEmailName:        register.NewSendRegisterationEmailHandler(container.JWT, container.SMTPMailer, container.MailFromAddress, container.TemplateRenderer),
	}

	return blog.NewServeCommand(
		handler,
		container.JetStreamPublishSubscriber,
		subscribers,
	)
}

func RunnerMannager(container *Container) *manager.ServeCommand {
	taskScheduler := roundrobin.New()

	taskRepository := taskrepository.NewRepository(container.Database)
	nodeRepository := noderepository.NewRepository(container.Database)

	managerRunTaskUseCase := managerRunTask.NewUseCase(taskRepository, container.JetStreamPublishSubscriber, container.Validator)
	managerDeleteTaskUseCase := managerDeleteTask.NewUseCase(taskRepository, container.JetStreamPublishSubscriber, container.Translator)
	managerStopTaskUseCase := managerStopTask.NewUseCase(taskRepository, container.JetStreamPublishSubscriber, container.Translator)
	managerGetTaskUseCase := managerGetTask.NewUseCase(taskRepository)
	managerGetTasksUseCase := managerGetTasks.NewUseCase(taskRepository)

	managerGetNodeUseCase := managerGetNode.NewUseCase(nodeRepository)
	managerGetNodesUseCase := managerGetNodes.NewUseCase(nodeRepository)

	mux := http.NewServeMux()

	mux.Handle("GET /api/runner/manager/tasks", managerTaskAPI.NewIndexHandler(managerGetTasksUseCase))
	mux.Handle("GET /api/runner/manager/tasks/{uuid}", managerTaskAPI.NewShowHandler(managerGetTaskUseCase))
	mux.Handle("DELETE /api/runner/manager/tasks/{uuid}", managerTaskAPI.NewDeleteHandler(managerDeleteTaskUseCase))
	mux.Handle("POST /api/runner/manager/tasks/run", managerTaskAPI.NewRunHandler(managerRunTaskUseCase))
	mux.Handle("POST /api/runner/manager/tasks/{uuid}/stop", managerTaskAPI.NewStopHandler(managerStopTaskUseCase))

	mux.Handle("GET /api/runner/manager/nodes", managerNodeAPI.NewIndexHandler(managerGetNodesUseCase))
	mux.Handle("GET /api/runner/manager/nodes/{name}", managerNodeAPI.NewShowHandler(managerGetNodeUseCase))

	handler := middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute))

	subscribers := map[string]domain.MessageHandler{
		nodeEvents.HeartbeatName:     managerHeartbeatNode.NewHeartbeatHandler(nodeRepository),
		taskEvents.HeartbeatName:     managerHeartbeatTask.NewHeartbeatHandler(taskRepository, container.JetStreamPublishSubscriber),
		taskEvents.TaskCreatedName:   managerRunTask.NewTaskCreated(taskRepository, nodeRepository, taskScheduler, container.JetStreamPublishSubscriber),
		taskEvents.TaskRanName:       managerRunTask.NewTaskRan(taskRepository),
		taskEvents.TaskCompletedName: managerRunTask.NewTaskCompleted(taskRepository),
		taskEvents.TaskFailedName:    managerRunTask.NewTaskFailed(taskRepository),
		taskEvents.TaskStoppedName:   managerStopTask.NewTaskStopped(taskRepository),
	}

	return manager.NewServeCommand(handler, container.JetStreamPublishSubscriber, subscribers)
}

func RunnerWorker(container *Container) *worker.ServeCommand {
	getTasksUseCase := workergettasks.NewUseCase(container.ContainerManager, "node1")
	runTaskUseCase := workerruntask.NewUseCase(container.ContainerManager, container.Validator, "node1")
	stopTaskUseCase := workerstoptask.NewUseCase(container.ContainerManager, container.Validator)
	deleteTaskUseCase := workerDeleteTask.NewUseCase(container.ContainerManager, container.Validator)

	mux := http.NewServeMux()

	mux.Handle("GET /api/runner/worker/tasks", workerTaskAPI.NewIndexHandler(getTasksUseCase))
	mux.Handle("POST /api/runner/worker/tasks/run", workerTaskAPI.NewRunHandler(runTaskUseCase))
	mux.Handle("POST /api/runner/worker/tasks/{uuid}/stop", workerTaskAPI.NewStopHandler(stopTaskUseCase))

	handler := middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute))

	subscribers := map[string]domain.MessageHandler{
		taskEvents.TaskScheduledName:         workerruntask.NewTaskScheduled(runTaskUseCase, "node1"),
		taskEvents.TaskStoppageRequestedName: workerstoptask.NewStoppageTaskHandler(stopTaskUseCase),
		taskEvents.TaskDeletedName:           workerDeleteTask.NewDeleteTaskHandler(deleteTaskUseCase),
	}

	return worker.NewServeCommand(handler, container.JetStreamPublishSubscriber, subscribers)
}
