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
	"github.com/khanzadimahdi/testproject/domain"
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
	userrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/users"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
	"github.com/khanzadimahdi/testproject/infrastructure/template"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
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
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/resources/translation"
	"github.com/nats-io/nats.go"
)

//go:embed resources/view
var files embed.FS

func App(ctx context.Context) (http.Handler, func()) {
	uri := fmt.Sprintf(
		"%s://%s:%s@%s:%s",
		os.Getenv("MONGO_SCHEME"),
		os.Getenv("MONGO_USERNAME"),
		os.Getenv("MONGO_PASSWORD"),
		os.Getenv("MONGO_HOST"),
		os.Getenv("MONGO_PORT"),
	)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	database := client.Database(os.Getenv("MONGO_DATABASE_NAME"))

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

	publishSubscriber, err := jetstream.NewPublishSubscriber(natsConnection)
	if err != nil {
		panic(err)
	}

	translator := translator.New(translation.Translations, translation.FA)
	validator := validator.New(translator)
	_ = validator

	articlesRepository := articlesrepository.NewRepository(database)
	commentsRepository := commentsrepository.NewRepository(database)
	filesRepository := filesrepository.NewRepository(database)
	elementsRepository := elementsrepository.NewRepository(database)
	userRepository := userrepository.NewRepository(database)
	permissionRepository := permissionsrepository.NewRepository()
	rolesRepository := rolesrepository.NewRepository(database)
	bookmarkRepository := bookmarksrepository.NewRepository(database)
	configRepository := configrepository.NewRepository(database)

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

	if err := publishSubscriber.Subscribe(ctx, "0", forgetpassword.SendForgetPasswordEmailName, forgetpassword.NewSendForgetPasswordEmailHandler(userRepository, j, mailer, mailFromAddress, templateRenderer)); err != nil {
		panic(err)
	}

	if err := publishSubscriber.Subscribe(ctx, "0", register.SendRegisterationEmailName, register.NewSendRegisterationEmailHandler(j, mailer, mailFromAddress, templateRenderer)); err != nil {
		panic(err)
	}

	authorization := domain.NewRoleBasedAccessControl(rolesRepository)

	homeUseCase := home.NewUseCase(articlesRepository, elementsRepository)

	log.SetFlags(log.LstdFlags | log.Llongfile)
	loginUseCase := login.NewUseCase(userRepository, j, hasher, translator, validator)
	refreshUseCase := refresh.NewUseCase(userRepository, j, translator, validator)
	forgetPasswordUseCase := forgetpassword.NewUseCase(userRepository, publishSubscriber, translator, validator)
	resetPasswordUseCase := resetpassword.NewUseCase(userRepository, hasher, j, translator, validator)
	registerUseCase := register.NewUseCase(userRepository, publishSubscriber, translator, validator)
	verifyUseCase := verify.NewUseCase(userRepository, rolesRepository, configRepository, hasher, j, translator, validator)

	getArticleUsecase := getArticle.NewUseCase(articlesRepository, elementsRepository)
	getArticlesUsecase := getArticles.NewUseCase(articlesRepository)
	getArticlesByHashtagUseCase := getArticlesByHashtag.NewUseCase(articlesRepository, validator)
	getFileUseCase := getFile.NewUseCase(filesRepository, fileStorage)
	getCommentsUseCase := getComments.NewUseCase(commentsRepository, userRepository, validator)
	createCommentUseCase := createComment.NewUseCase(commentsRepository, validator)
	bookmarkExistsUseCase := bookmarkExists.NewUseCase(bookmarkRepository, validator)
	updateABookmark := updateBookmark.NewUseCase(bookmarkRepository, validator)

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
	mux.Handle("POST /api/comments", middleware.NewAuthoriseMiddleware(commentAPI.NewCreateHandler(createCommentUseCase), j, userRepository))
	mux.Handle("GET /api/comments", commentAPI.NewIndexHandler(getCommentsUseCase))

	// bookmark
	mux.Handle("POST /api/bookmarks/exists", middleware.NewAuthoriseMiddleware(bookmarkAPI.NewExistsHandler(bookmarkExistsUseCase), j, userRepository))
	mux.Handle("PUT /api/bookmarks", middleware.NewAuthoriseMiddleware(bookmarkAPI.NewUpdateHandler(updateABookmark), j, userRepository))

	// hashtags
	mux.Handle("GET /api/hashtags/{hashtag}", hashtagAPI.NewShowHandler(getArticlesByHashtagUseCase))

	// files
	mux.Handle("GET /files/{uuid}", fileAPI.NewShowHandler(getFileUseCase))

	// -------------------- dashboard -------------------- //
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

	dashboardCreateElementUsecase := dashboardCreateElement.NewUseCase(elementsRepository)
	dashboardDeleteElementUsecase := dashboardDeleteElement.NewUseCase(elementsRepository)
	dashboardGetElementUsecase := dashboardGetElement.NewUseCase(elementsRepository)
	dashboardGetElementsUsecase := dashboardGetElements.NewUseCase(elementsRepository)
	dashboardUpdateElementUsecase := dashboardUpdateElement.NewUseCase(elementsRepository)

	dashboardGetConfigUsecase := dashboardGetConfig.NewUseCase(configRepository)
	dashboardUpdateConfigUsecase := dashboardUpdateConfig.NewUseCase(configRepository, validator)

	// profile
	mux.Handle("GET /api/dashboard/profile", middleware.NewAuthoriseMiddleware(profile.NewGetProfileHandler(getProfileUseCase), j, userRepository))
	mux.Handle("PUT /api/dashboard/profile", middleware.NewAuthoriseMiddleware(profile.NewUpdateProfileHandler(updateProfileUseCase), j, userRepository))
	mux.Handle("PUT /api/dashboard/password", middleware.NewAuthoriseMiddleware(profile.NewChangePasswordHandler(dashboardProfileChangePasswordUseCase), j, userRepository))
	mux.Handle("GET /api/dashboard/profile/roles", middleware.NewAuthoriseMiddleware(profile.NewGetRolesHandler(dashboardProfileGetRolesUseCase), j, userRepository))

	// user
	mux.Handle("POST /api/dashboard/users", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewCreateHandler(dashboardCreateUserUsecase, authorization), j, userRepository))
	mux.Handle("DELETE /api/dashboard/users/{uuid}", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewDeleteHandler(dashboardDeleteUserUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/users", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewIndexHandler(dashboardGetUsersUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/users/{uuid}", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewShowHandler(dashboardGetUserUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/users", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewUpdateHandler(dashboardUpdateUserUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/users/password", middleware.NewAuthoriseMiddleware(dashboardUserAPI.NewChangePasswordHandler(dashboardUpdateUserChangePasswordUsecase, authorization), j, userRepository))

	// permissions
	mux.Handle("GET /api/dashboard/permissions", middleware.NewAuthoriseMiddleware(dashboardPermissionAPI.NewIndexHandler(dashboardGetPermissionsUseCase, authorization), j, userRepository))

	// roles
	mux.Handle("POST /api/dashboard/roles", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewCreateHandler(dashboardCreateRoleUsecase, authorization), j, userRepository))
	mux.Handle("DELETE /api/dashboard/roles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewDeleteHandler(dashboardDeleteRoleUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/roles", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewIndexHandler(dashboardGetRolesUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/roles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewShowHandler(dashboardGetRoleUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/roles", middleware.NewAuthoriseMiddleware(dashboardRoleAPI.NewUpdateHandler(dashboardUpdateRoleUsecase, authorization), j, userRepository))

	// articles
	mux.Handle("POST /api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewCreateHandler(dashboardCreateArticleUsecase, authorization), j, userRepository))
	mux.Handle("DELETE /api/dashboard/articles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewDeleteHandler(dashboardDeleteArticleUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewIndexHandler(dashboardGetArticlesUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/articles/{uuid}", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewShowHandler(dashboardGetArticleUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewUpdateHandler(dashboardUpdateArticleUsecase, authorization), j, userRepository))

	// comments
	mux.Handle("POST /api/dashboard/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewCreateHandler(dashboardCreateCommentUsecase, authorization), j, userRepository))
	mux.Handle("DELETE /api/dashboard/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewDeleteHandler(dashboardDeleteCommentUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewIndexHandler(dashboardGetCommentsUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewShowHandler(dashboardGetCommentUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewUpdateHandler(dashboardUpdateCommentUsecase, authorization), j, userRepository))

	// self comments
	mux.Handle("DELETE /api/dashboard/my/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewDeleteUserCommentHandler(dashboardDeleteUserCommentUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/my/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewIndexUserCommentsHandler(dashboardGetUserCommentsUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/my/comments/{uuid}", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewShowUserCommentHandler(dashboardGetUserCommentUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/my/comments", middleware.NewAuthoriseMiddleware(dashboardCommentAPI.NewUpdateUserCommentHandler(dashboardUpdateUserCommentUsecase, authorization), j, userRepository))

	// self bookmarks
	mux.Handle("DELETE /api/dashboard/my/bookmarks", middleware.NewAuthoriseMiddleware(dashboardBookmarkAPI.NewDeleteUserBookmarkHandler(dashboardDeleteUserBookmarkUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/my/bookmarks", middleware.NewAuthoriseMiddleware(dashboardBookmarkAPI.NewIndexUserBookmarksHandler(dashboardGetUserBookmarksUsecase, authorization), j, userRepository))

	// files
	mux.Handle("POST /api/dashboard/files", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewUploadHandler(dashboardUploadFileUseCase, authorization), j, userRepository))
	mux.Handle("DELETE /api/dashboard/files/{uuid}", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewDeleteHandler(dashboardDeleteFileUseCase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/files", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewIndexHandler(dashboardGetFilesUseCase, authorization), j, userRepository))
	mux.Handle("GET /dashboard/files/{uuid}", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewShowHandler(dashboardGetFileUseCase, authorization), j, userRepository))

	// self files
	mux.Handle("DELETE /api/dashboard/my/files/{uuid}", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewDeleteUserHandler(dashboardDeleteUserFileUseCase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/my/files", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewIndexUserHandler(dashboardGetUserFilesUseCase, authorization), j, userRepository))

	// elements
	mux.Handle("POST /api/dashboard/elements", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewCreateHandler(dashboardCreateElementUsecase, authorization), j, userRepository))
	mux.Handle("DELETE /api/dashboard/elements/{uuid}", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewDeleteHandler(dashboardDeleteElementUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/elements", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewIndexHandler(dashboardGetElementsUsecase, authorization), j, userRepository))
	mux.Handle("GET /api/dashboard/elements/{uuid}", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewShowHandler(dashboardGetElementUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/elements", middleware.NewAuthoriseMiddleware(dashboardElementAPI.NewUpdateHandler(dashboardUpdateElementUsecase, authorization), j, userRepository))

	// config
	mux.Handle("GET /api/dashboard/config", middleware.NewAuthoriseMiddleware(dashboardConfigAPI.NewShowHandler(dashboardGetConfigUsecase, authorization), j, userRepository))
	mux.Handle("PUT /api/dashboard/config", middleware.NewAuthoriseMiddleware(dashboardConfigAPI.NewUpdateHandler(dashboardUpdateConfigUsecase, authorization), j, userRepository))

	return middleware.NewCORSMiddleware(middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute)), publishSubscriber.Wait
}
