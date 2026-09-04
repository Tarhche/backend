package providers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/application/access"
	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	getArticle "github.com/khanzadimahdi/testproject/application/article/getArticle"
	getArticles "github.com/khanzadimahdi/testproject/application/article/getArticles"
	"github.com/khanzadimahdi/testproject/application/article/getArticlesByAuthor"
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
	"github.com/khanzadimahdi/testproject/application/contact/createMessage"
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
	dashboardDeleteContactMessage "github.com/khanzadimahdi/testproject/application/dashboard/contact/deleteMessage"
	dashboardGetContactMessage "github.com/khanzadimahdi/testproject/application/dashboard/contact/getMessage"
	dashboardGetContactMessages "github.com/khanzadimahdi/testproject/application/dashboard/contact/getMessages"
	dashboardMarkContactMessageAsRead "github.com/khanzadimahdi/testproject/application/dashboard/contact/markAsRead"
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
	dashboardCreateLanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/createLanguage"
	dashboardDeleteLanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/deleteLanguage"
	dashboardGetLanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/getLanguage"
	dashboardGetLanguages "github.com/khanzadimahdi/testproject/application/dashboard/language/getLanguages"
	dashboardUpdateLanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/updateLanguage"
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
	runnerAccess "github.com/khanzadimahdi/testproject/application/dashboard/runner/access"
	dashboardAttachContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/attachContainer"
	dashboardDeleteContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/deleteContainer"
	dashboardFollowContainerLogs "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/followContainerLogs"
	dashboardGetContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/getContainer"
	dashboardGetContainerLogs "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/getContainerLogs"
	dashboardGetContainers "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/getContainers"
	dashboardKillContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/killContainer"
	dashboardRestartContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/restartContainer"
	dashboardRunContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/runContainer"
	dashboardStopContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/stopContainer"
	dashboardWatchContainers "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/watchContainers"
	runnerOwners "github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	dashboardDeleteStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/deleteStack"
	dashboardGetStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/getStack"
	dashboardGetStacks "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/getStacks"
	dashboardKillStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/killStack"
	dashboardRestartStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/restartStack"
	dashboardRunStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/runStack"
	dashboardStopStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/stopStack"
	dashboardWatchStacks "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/watchStacks"
	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	getuser "github.com/khanzadimahdi/testproject/application/dashboard/user/getUser"
	getusers "github.com/khanzadimahdi/testproject/application/dashboard/user/getUsers"
	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
	"github.com/khanzadimahdi/testproject/application/dashboard/user/userchangepassword"
	"github.com/khanzadimahdi/testproject/application/element"
	getFile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/application/home"
	getLanguages "github.com/khanzadimahdi/testproject/application/language/getLanguages"
	languageresolver "github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/application/localize"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/permission"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/cache"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	infraHealth "github.com/khanzadimahdi/testproject/infrastructure/health"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	"github.com/khanzadimahdi/testproject/infrastructure/matcher"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/core/pubsub"
	"github.com/khanzadimahdi/testproject/infrastructure/messaging/nats/jetstream/produceConsumer"
	articlesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/articles"
	bookmarksrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/bookmarks"
	commentsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/comments"
	configrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/config"
	contactsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/contacts"
	elementsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/elements"
	filesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/files"
	languagesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/languages"
	permissionsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/permissions"
	rolesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/roles"
	userrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/users"
	runnerClient "github.com/khanzadimahdi/testproject/infrastructure/runner/manager/client"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	infraWebsocket "github.com/khanzadimahdi/testproject/infrastructure/websocket"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
	articleAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/article"
	authAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/auth"
	authorArticleAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/author/article"
	bookmarkAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/bookmark"
	commentAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/comment"
	contactAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/contact"
	dashboardArticleAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/article"
	dashboardBookmarkAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/bookmark"
	dashboardCommentAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/comment"
	dashboardConfigAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/config"
	dashboardContactAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/contact"
	dashboardElementAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/element"
	dashboardFileAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/file"
	dashboardLanguageAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/language"
	dashboardPermissionAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/permission"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/profile"
	dashboardRoleAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/role"
	dashboardRunnerContainerAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/runner/container"
	dashboardRunnerStackAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/runner/stack"
	dashboardUserAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/user"
	fileAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/file"
	hashtagAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/hashtag"
	homeapi "github.com/khanzadimahdi/testproject/presentation/http/blog/api/home"
	languageAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/language"
	websocketAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/websocket"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/openapi"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	blogConsumerID = "blog"

	BlogSubscribers              = "blog:subscribers"
	BlogHTTPCacheBucketName      = "blog_http_cache"
	BlogWebSocketCacheBucketName = "blog_ws_cache"
)

// blogProvider builds the blog service's messaging singletons, HTTP handler and
// message subscribers. It must be registered after its dependencies.
type blogProvider struct {
	terminate func()
}

var _ provider.Provider = &blogProvider{}

func NewBlogProvider() *blogProvider {
	return &blogProvider{}
}

func (p *blogProvider) Register(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *blogProvider) Boot(ctx context.Context, c provider.Container) error {
	var translator translatorContract.Translator
	if err := c.Resolve(&translator); err != nil {
		return err
	}

	var natsConnection *nats.Conn
	if err := c.Resolve(&natsConnection); err != nil {
		return err
	}

	var logger *slog.Logger
	if err := c.Resolve(&logger, provider.WithParams("blog")); err != nil {
		return err
	}

	pc, err := produceConsumer.NewProduceConsumer(natsConnection, blogConsumerID, logger, produceConsumer.WithAckWait(120*time.Second))
	if err != nil {
		return err
	}

	ps := pubsub.NewPublishSubscriber(natsConnection, logger)

	runCodeCache, err := cache.NewNatsCache(
		natsConnection,
		BlogWebSocketCacheBucketName,
		cache.WithTTL(1*time.Hour),
		cache.WithLimitMarkerTTL(1*time.Second),
		cache.WithCompression(true),
	)
	if err != nil {
		return err
	}

	// the gateway carries requests to the broker and replies back; the websocket
	// is one transport it is reachable on. A second protocol is a second
	// transport over the same gateway, not a second gateway.
	messageGateway, err := gateway.New(
		pc,
		ps,
		translator,
		"replies",
		logger,
	)
	if err != nil {
		return err
	}

	cachedGateway := gateway.NewCacheDecorator(messageGateway, runCodeCache, logger, runCode.RunCodeRequest)

	websocketTransport, err := infraWebsocket.NewHandler(messageGateway, logger)
	if err != nil {
		return err
	}

	c.Bind(func() domain.Producer { return pc }, provider.Singleton())
	c.Bind(func() domain.Consumer { return pc }, provider.Singleton())
	c.Bind(func() domain.ProduceConsumer { return pc }, provider.Singleton())
	c.Bind(func() domain.PublishSubscriber { return ps }, provider.Singleton())
	c.Bind(func() *gateway.Gateway { return messageGateway }, provider.Singleton())
	c.Bind(func() *gateway.CacheDecorator { return cachedGateway }, provider.Singleton())
	c.Bind(func() *infraWebsocket.Handler { return websocketTransport }, provider.Singleton())

	p.terminate = func() {
		defer pc.Wait()
		defer ps.Wait()
		defer messageGateway.Close()
	}

	return c.Bind(blog, provider.Singleton())
}

func (p *blogProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}

func blog(
	database *mongo.Database,
	jwt *jwt.JWT,
	hasher password.Hasher,
	asyncProduceConsumer domain.ProduceConsumer,
	translator translatorContract.Translator,
	validator domain.Validator,
	fileStorage file.Storage,
	authorizer domain.Authorizer,
	mailer domain.Mailer,
	renderer domain.Renderer,
	cachedGateway *gateway.CacheDecorator,
	websocketTransport *infraWebsocket.Handler,
	iocContainer provider.Container,
) (http.Handler, error) {
	var logger *slog.Logger
	if err := iocContainer.Resolve(&logger, provider.WithParams("blog")); err != nil {
		return nil, err
	}

	var mailFromAddress string
	if err := iocContainer.Resolve(&mailFromAddress, provider.ResolveName(MailFromAddress)); err != nil {
		return nil, err
	}

	var blogConfigs *configs.Blog
	if err := iocContainer.Resolve(&blogConfigs); err != nil {
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

	articlesRepository := articlesrepository.NewRepository(database)
	commentsRepository := commentsrepository.NewRepository(database)
	contactsRepository := contactsrepository.NewRepository(database)
	filesRepository := filesrepository.NewRepository(database)
	elementsRepository := elementsrepository.NewRepository(database)
	userRepository := userrepository.NewRepository(database)
	permissionRepository := permissionsrepository.NewRepository()
	rolesRepository := rolesrepository.NewRepository(database)
	bookmarkRepository := bookmarksrepository.NewRepository(database)
	configRepository := configrepository.NewRepository(database)
	languageRepository := languagesrepository.NewRepository(database)
	languageResolver := languageresolver.New(languageRepository, configRepository)

	authTokenGenerator := auth.NewTokenGenerator(jwt, rolesRepository)
	elementRetriever := element.NewRetriever(articlesRepository, elementsRepository, userRepository, matcher.New())

	// Every route resolves its language through the Localize middleware. localized
	// injects the resolved language into the request context; scoped additionally
	// builds the route's use case per request from the request-scoped container,
	// so tr/va yield language-aware translation and validation.
	localizer := localize.New(languageResolver)
	localized := func(next http.Handler) http.Handler {
		return middleware.NewLocalizeMiddleware(next, localizer, provider.Default)
	}
	scoped := func(build func(c provider.Container) http.Handler) http.Handler {
		return localized(middleware.NewScopedHandler(build))
	}
	tr := func(c provider.Container) translatorContract.Translator {
		var t translatorContract.Translator
		if err := c.Resolve(&t); err != nil {
			panic(err)
		}
		return t
	}
	va := func(c provider.Container) domain.Validator {
		var v domain.Validator
		if err := c.Resolve(&v); err != nil {
			panic(err)
		}
		return v
	}

	// ---- public ----
	homeUseCase := home.NewUseCase(articlesRepository, userRepository, elementRetriever, languageResolver)

	getArticlesUsecase := getArticles.NewUseCase(articlesRepository, userRepository, languageRepository, languageResolver, elementRetriever)
	getLanguagesUseCase := getLanguages.NewUseCase(languageRepository, languageResolver)
	getFileUseCase := getFile.NewUseCase(filesRepository, fileStorage)

	if err := cachedGateway.Consume(
		context.Background(),
		runCode.RunCodeRequest,
		runCode.NewRunCodeHandler(validator, asyncProduceConsumer, cachedGateway, logger),
	); err != nil {
		return nil, err
	}

	// ---- dashboard: the runner ----
	//
	// the dashboard does not schedule containers itself. It establishes who is
	// asking and whether they may, then passes the request to the runner, so
	// one service owns a container's lifecycle.
	runner, err := runnerClient.New(blogConfigs.RunnerManagerURL)
	if err != nil {
		return nil, err
	}

	authenticator := auth.NewAuthenticator(jwt, userRepository)
	ingressDomain := blogConfigs.RunnerIngressDomain

	// what somebody may do to one particular thing: the permission over all of
	// them, or the one over their own and it being theirs.
	guard := access.NewGuard(authorizer)

	// the runner keeps the id of whoever asked for a container; this is what
	// puts a name to it when the dashboard shows one.
	ownerDirectory := runnerOwners.NewDirectory(userRepository)
	runnerGuard := runnerAccess.NewGuard(runner, guard)

	dashboardGetContainersUseCase := dashboardGetContainers.NewUseCase(runner, ownerDirectory, ingressDomain)
	dashboardGetContainerUseCase := dashboardGetContainer.NewUseCase(runner, ownerDirectory, guard, ingressDomain)
	dashboardRunContainerUseCase := dashboardRunContainer.NewUseCase(runner, validator, ownerDirectory, ingressDomain)
	dashboardStopContainerUseCase := dashboardStopContainer.NewUseCase(runner, runnerGuard)
	dashboardKillContainerUseCase := dashboardKillContainer.NewUseCase(runner, runnerGuard)
	dashboardRestartContainerUseCase := dashboardRestartContainer.NewUseCase(runner, runnerGuard)
	dashboardDeleteContainerUseCase := dashboardDeleteContainer.NewUseCase(runner, runnerGuard)
	dashboardGetContainerLogsUseCase := dashboardGetContainerLogs.NewUseCase(runner, runnerGuard)

	dashboardGetStacksUseCase := dashboardGetStacks.NewUseCase(runner, ownerDirectory, ingressDomain)
	dashboardGetStackUseCase := dashboardGetStack.NewUseCase(runner, ownerDirectory, guard, ingressDomain)
	dashboardRunStackUseCase := dashboardRunStack.NewUseCase(runner, validator, ownerDirectory, ingressDomain)
	dashboardStopStackUseCase := dashboardStopStack.NewUseCase(runner, runnerGuard)
	dashboardKillStackUseCase := dashboardKillStack.NewUseCase(runner, runnerGuard)
	dashboardRestartStackUseCase := dashboardRestartStack.NewUseCase(runner, runnerGuard)
	dashboardDeleteStackUseCase := dashboardDeleteStack.NewUseCase(runner, runnerGuard)

	// a terminal and a live log are streams rather than answers, so they travel
	// over the websocket the gateway already serves: one request opens the
	// stream and its reply arrives chunk by chunk until it ends.
	var messageGateway *gateway.Gateway
	if err := iocContainer.Resolve(&messageGateway); err != nil {
		return nil, err
	}

	streams := gateway.NewStreams()
	if err := messageGateway.WatchStreamCancellations(context.Background(), streams); err != nil {
		return nil, err
	}

	dashboardAttachContainerUseCase := dashboardAttachContainer.NewUseCase(runner, authenticator, authorizer, runnerGuard, validator, cachedGateway, streams, logger)
	dashboardFollowContainerLogsUseCase := dashboardFollowContainerLogs.NewUseCase(runner, authenticator, authorizer, runnerGuard, validator, cachedGateway, streams, logger)
	dashboardWatchContainersUseCase := dashboardWatchContainers.NewUseCase(runner, authenticator, authorizer, validator, cachedGateway, streams, ownerDirectory, ingressDomain, logger)
	dashboardWatchStacksUseCase := dashboardWatchStacks.NewUseCase(runner, authenticator, authorizer, validator, cachedGateway, streams, ownerDirectory, ingressDomain, logger)

	for subject, handler := range map[string]domain.MessageHandler{
		dashboardAttachContainer.AttachName:     dashboardAttachContainerUseCase,
		dashboardAttachContainer.InputName:      dashboardAttachContainerUseCase.InputHandler(),
		dashboardFollowContainerLogs.FollowName: dashboardFollowContainerLogsUseCase,
		dashboardWatchContainers.WatchName:      dashboardWatchContainersUseCase,
		dashboardWatchStacks.WatchName:          dashboardWatchStacksUseCase,
	} {
		if err := cachedGateway.Consume(context.Background(), subject, handler); err != nil {
			return nil, err
		}
	}

	// ---- dashboard ----
	getProfileUseCase := getprofile.NewUseCase(userRepository)
	dashboardProfileGetRolesUseCase := getRoles.NewUseCase(rolesRepository)

	dashboardDeleteArticleUsecase := dashboardDeleteArticle.NewUseCase(articlesRepository)
	dashboardGetArticleUsecase := dashboardGetArticle.NewUseCase(articlesRepository, userRepository)
	dashboardGetArticlesUsecase := dashboardGetArticles.NewUseCase(articlesRepository, userRepository, languageRepository)

	dashboardDeleteCommentUsecase := dashboardDeleteComment.NewUseCase(commentsRepository)
	dashboardGetCommentUsecase := dashboardGetComment.NewUseCase(commentsRepository, userRepository)
	dashboardGetCommentsUsecase := dashboardGetComments.NewUseCase(commentsRepository, userRepository)

	dashboardDeleteContactMessageUsecase := dashboardDeleteContactMessage.NewUseCase(contactsRepository)
	dashboardGetContactMessageUsecase := dashboardGetContactMessage.NewUseCase(contactsRepository)
	dashboardGetContactMessagesUsecase := dashboardGetContactMessages.NewUseCase(contactsRepository)
	dashboardMarkContactMessageAsReadUsecase := dashboardMarkContactMessageAsRead.NewUseCase(contactsRepository)

	dashboardDeleteUserCommentUsecase := dashboardDeleteUserComment.NewUseCase(commentsRepository)
	dashboardGetUserCommentUsecase := dashboardGetUserComment.NewUseCase(commentsRepository, userRepository)
	dashboardGetUserCommentsUsecase := dashboardGetUserComments.NewUseCase(commentsRepository, userRepository)

	dashboardDeleteUserUsecase := deleteuser.NewUseCase(userRepository)
	dashboardGetUserUsecase := getuser.NewUseCase(userRepository)
	dashboardGetUsersUsecase := getusers.NewUseCase(userRepository)

	dashboardGetPermissionsUseCase := dashboardGetPermissions.NewUseCase(permissionRepository)

	dashboardDeleteRoleUsecase := dashboardDeleteRole.NewUseCase(rolesRepository)
	dashboardGetRoleUsecase := dashboardGetRole.NewUseCase(rolesRepository)
	dashboardGetRolesUsecase := dashboardGetRoles.NewUseCase(rolesRepository)

	dashboardDeleteLanguageUsecase := dashboardDeleteLanguage.NewUseCase(languageRepository)
	dashboardGetLanguageUsecase := dashboardGetLanguage.NewUseCase(languageRepository)
	dashboardGetLanguagesUsecase := dashboardGetLanguages.NewUseCase(languageRepository)

	dashboardGetFilesUseCase := dashboardGetFiles.NewUseCase(filesRepository)
	dashboardGetFileUseCase := dashboardGetFile.NewUseCase(filesRepository, fileStorage)
	dashboardDeleteFileUseCase := dashboardDeleteFile.NewUseCase(filesRepository, fileStorage)

	dashboardGetUserFilesUseCase := dashboardGetUserFiles.NewUseCase(filesRepository)
	dashboardDeleteUserFileUseCase := dashboardDeleteUserFile.NewUseCase(filesRepository, fileStorage)

	dashboardDeleteElementUsecase := dashboardDeleteElement.NewUseCase(elementsRepository)
	dashboardGetElementUsecase := dashboardGetElement.NewUseCase(elementsRepository)
	dashboardGetElementsUsecase := dashboardGetElements.NewUseCase(elementsRepository)

	dashboardGetConfigUsecase := dashboardGetConfig.NewUseCase(configRepository)

	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "database", Pinger: infraHealth.NewMongodbPinger(database)},
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

	mux := http.NewServeMux()

	// ---- health ----
	// the container healthcheck probes this
	mux.Handle("GET /health", healthAPI.NewHealthHandler(checkHealthUseCase))

	// ---- openapi ----
	mux.Handle("/openapi/", openapi.NewOpenAPIHandler())

	// ---- public HTTP API ----

	// websocket
	mux.Handle("GET /api/ws", websocketAPI.NewWebsocketHandler(websocketTransport))

	// home
	mux.Handle("GET /api/home", middleware.NewCacheMiddleware(localized(homeapi.NewHomeHandler(homeUseCase)), httpCache))

	// auth
	mux.Handle("POST /api/auth/login", scoped(func(c provider.Container) http.Handler {
		return authAPI.NewLoginHandler(login.NewUseCase(userRepository, authTokenGenerator, hasher, tr(c), va(c)))
	}))
	mux.Handle("POST /api/auth/token/refresh", scoped(func(c provider.Container) http.Handler {
		return authAPI.NewRefreshHandler(refresh.NewUseCase(userRepository, jwt, authTokenGenerator, tr(c), va(c)))
	}))
	mux.Handle("POST /api/auth/password/forget", scoped(func(c provider.Container) http.Handler {
		return authAPI.NewForgetPasswordHandler(forgetpassword.NewUseCase(userRepository, asyncProduceConsumer, tr(c), va(c)))
	}))
	mux.Handle("POST /api/auth/password/reset", scoped(func(c provider.Container) http.Handler {
		return authAPI.NewResetPasswordHandler(resetpassword.NewUseCase(userRepository, hasher, jwt, tr(c), va(c)))
	}))
	mux.Handle("POST /api/auth/register", scoped(func(c provider.Container) http.Handler {
		return authAPI.NewRegisterHandler(register.NewUseCase(userRepository, asyncProduceConsumer, tr(c), va(c)))
	}))
	mux.Handle("POST /api/auth/verify", scoped(func(c provider.Container) http.Handler {
		return authAPI.NewVerifyHandler(verify.NewUseCase(userRepository, rolesRepository, configRepository, languageResolver, hasher, jwt, tr(c), va(c)))
	}))

	// articles
	mux.Handle("GET /api/articles", middleware.NewCacheMiddleware(localized(articleAPI.NewIndexHandler(getArticlesUsecase)), httpCache))
	mux.Handle("GET /api/articles/{uuid}", middleware.NewCacheMiddleware(scoped(func(c provider.Container) http.Handler {
		return articleAPI.NewShowHandler(getArticle.NewUseCase(articlesRepository, userRepository, languageRepository, languageResolver, elementRetriever, va(c)))
	}), httpCache))

	// comments
	mux.Handle("POST /api/comments", middleware.NewAuthenticateMiddleware(scoped(func(c provider.Container) http.Handler {
		return commentAPI.NewCreateHandler(createComment.NewUseCase(commentsRepository, va(c)))
	}), jwt, userRepository))
	mux.Handle("GET /api/comments", scoped(func(c provider.Container) http.Handler {
		return commentAPI.NewIndexHandler(getComments.NewUseCase(commentsRepository, userRepository, va(c)))
	}))

	// contact us
	mux.Handle("POST /api/contact-us", scoped(func(c provider.Container) http.Handler {
		return contactAPI.NewCreateHandler(createMessage.NewUseCase(contactsRepository, va(c)))
	}))

	// bookmark
	mux.Handle("POST /api/bookmarks/exists", middleware.NewAuthenticateMiddleware(scoped(func(c provider.Container) http.Handler {
		return bookmarkAPI.NewExistsHandler(bookmarkExists.NewUseCase(bookmarkRepository, va(c)))
	}), jwt, userRepository))
	mux.Handle("PUT /api/bookmarks", middleware.NewAuthenticateMiddleware(scoped(func(c provider.Container) http.Handler {
		return bookmarkAPI.NewUpdateHandler(updateBookmark.NewUseCase(bookmarkRepository, va(c)))
	}), jwt, userRepository))

	// languages
	mux.Handle("GET /api/languages", middleware.NewCacheMiddleware(languageAPI.NewIndexHandler(getLanguagesUseCase), httpCache))

	// hashtags
	mux.Handle("GET /api/hashtags/{hashtag}", middleware.NewCacheMiddleware(scoped(func(c provider.Container) http.Handler {
		return hashtagAPI.NewShowHandler(getArticlesByHashtag.NewUseCase(articlesRepository, userRepository, languageRepository, languageResolver, elementRetriever, va(c)))
	}), httpCache))

	// authors
	mux.Handle("GET /api/authors/{identity}/articles", middleware.NewCacheMiddleware(scoped(func(c provider.Container) http.Handler {
		return authorArticleAPI.NewIndexHandler(getArticlesByAuthor.NewUseCase(articlesRepository, userRepository, languageRepository, languageResolver, elementRetriever, va(c)))
	}), httpCache))

	// files
	mux.Handle("GET /files/{uuid}", middleware.NewCacheMiddleware(fileAPI.NewShowHandler(getFileUseCase), httpCache))

	// ---- dashboard HTTP API ----

	// profile
	mux.Handle("GET /api/dashboard/profile", middleware.NewAuthenticateMiddleware(profile.NewGetProfileHandler(getProfileUseCase), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/profile", middleware.NewAuthenticateMiddleware(scoped(func(c provider.Container) http.Handler {
		return profile.NewUpdateProfileHandler(updateprofile.NewUseCase(userRepository, languageResolver, va(c), tr(c)))
	}), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/password", middleware.NewAuthenticateMiddleware(scoped(func(c provider.Container) http.Handler {
		return profile.NewChangePasswordHandler(changepassword.NewUseCase(userRepository, hasher, va(c), tr(c)))
	}), jwt, userRepository))
	mux.Handle("GET /api/dashboard/profile/roles", middleware.NewAuthenticateMiddleware(profile.NewGetRolesHandler(dashboardProfileGetRolesUseCase), jwt, userRepository))

	// user
	mux.Handle("POST /api/dashboard/users", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardUserAPI.NewCreateHandler(createuser.NewUseCase(userRepository, languageResolver, hasher, va(c), tr(c)))
	}), authorizer, permission.UsersCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/users/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewDeleteHandler(dashboardDeleteUserUsecase), authorizer, permission.UsersDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/users", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewIndexHandler(dashboardGetUsersUsecase), authorizer, permission.UsersIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/users/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardUserAPI.NewShowHandler(dashboardGetUserUsecase), authorizer, permission.UsersShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/users", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardUserAPI.NewUpdateHandler(updateuser.NewUseCase(userRepository, languageResolver, va(c), tr(c)))
	}), authorizer, permission.UsersUpdate), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/users/password", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardUserAPI.NewChangePasswordHandler(userchangepassword.NewUseCase(userRepository, hasher, va(c)))
	}), authorizer, permission.UsersPasswordUpdate), jwt, userRepository))

	// permissions
	mux.Handle("GET /api/dashboard/permissions", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardPermissionAPI.NewIndexHandler(dashboardGetPermissionsUseCase), authorizer, permission.PermissionsIndex), jwt, userRepository))

	// roles
	mux.Handle("POST /api/dashboard/roles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardRoleAPI.NewCreateHandler(dashboardCreateRole.NewUseCase(rolesRepository, permissionRepository, va(c), tr(c)))
	}), authorizer, permission.RolesCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/roles/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewDeleteHandler(dashboardDeleteRoleUsecase), authorizer, permission.RolesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/roles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewIndexHandler(dashboardGetRolesUsecase), authorizer, permission.RolesIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/roles/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRoleAPI.NewShowHandler(dashboardGetRoleUsecase), authorizer, permission.RolesShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/roles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardRoleAPI.NewUpdateHandler(dashboardUpdateRole.NewUseCase(rolesRepository, permissionRepository, va(c), tr(c)))
	}), authorizer, permission.RolesUpdate), jwt, userRepository))

	// languages
	mux.Handle("POST /api/dashboard/languages", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardLanguageAPI.NewCreateHandler(dashboardCreateLanguage.NewUseCase(languageRepository, va(c), tr(c)))
	}), authorizer, permission.LanguagesCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/languages/{code}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardLanguageAPI.NewDeleteHandler(dashboardDeleteLanguageUsecase), authorizer, permission.LanguagesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/languages", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardLanguageAPI.NewIndexHandler(dashboardGetLanguagesUsecase), authorizer, permission.LanguagesIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/languages/{code}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardLanguageAPI.NewShowHandler(dashboardGetLanguageUsecase), authorizer, permission.LanguagesShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/languages", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardLanguageAPI.NewUpdateHandler(dashboardUpdateLanguage.NewUseCase(languageRepository, va(c)))
	}), authorizer, permission.LanguagesUpdate), jwt, userRepository))

	// articles
	mux.Handle("POST /api/dashboard/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardArticleAPI.NewCreateHandler(dashboardCreateArticle.NewUseCase(articlesRepository, languageRepository, va(c), tr(c)))
	}), authorizer, permission.ArticlesCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/articles/{correlationUUID}/{language_code}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewDeleteHandler(dashboardDeleteArticleUsecase), authorizer, permission.ArticlesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewIndexHandler(dashboardGetArticlesUsecase), authorizer, permission.ArticlesIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/articles/{correlationUUID}/{language_code}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewShowHandler(dashboardGetArticleUsecase), authorizer, permission.ArticlesShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardArticleAPI.NewUpdateHandler(dashboardUpdateArticle.NewUseCase(articlesRepository, languageRepository, va(c), tr(c)))
	}), authorizer, permission.ArticlesUpdate), jwt, userRepository))

	// comments
	mux.Handle("POST /api/dashboard/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardCommentAPI.NewCreateHandler(dashboardCreateComment.NewUseCase(commentsRepository, va(c)))
	}), authorizer, permission.CommentsCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewDeleteHandler(dashboardDeleteCommentUsecase), authorizer, permission.CommentsDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewIndexHandler(dashboardGetCommentsUsecase), authorizer, permission.CommentsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewShowHandler(dashboardGetCommentUsecase), authorizer, permission.CommentsShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardCommentAPI.NewUpdateHandler(dashboardUpdateComment.NewUseCase(commentsRepository, va(c)))
	}), authorizer, permission.CommentsUpdate), jwt, userRepository))

	// self comments
	mux.Handle("DELETE /api/dashboard/my/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewDeleteUserCommentHandler(dashboardDeleteUserCommentUsecase), authorizer, permission.SelfCommentsDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewIndexUserCommentsHandler(dashboardGetUserCommentsUsecase), authorizer, permission.SelfCommentsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/comments/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardCommentAPI.NewShowUserCommentHandler(dashboardGetUserCommentUsecase), authorizer, permission.SelfCommentsShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/my/comments", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardCommentAPI.NewUpdateUserCommentHandler(dashboardUpdateUserComment.NewUseCase(commentsRepository, va(c)))
	}), authorizer, permission.SelfCommentsUpdate), jwt, userRepository))

	// self bookmarks
	mux.Handle("DELETE /api/dashboard/my/bookmarks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardBookmarkAPI.NewDeleteUserBookmarkHandler(dashboardDeleteUserBookmark.NewUseCase(bookmarkRepository, va(c)))
	}), authorizer, permission.SelfBookmarksDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/bookmarks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardBookmarkAPI.NewIndexUserBookmarksHandler(dashboardGetUserBookmarks.NewUseCase(bookmarkRepository, va(c)))
	}), authorizer, permission.SelfBookmarksIndex), jwt, userRepository))

	// files
	mux.Handle("POST /api/dashboard/files", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardFileAPI.NewUploadHandler(dashboardUploadFile.NewUseCase(filesRepository, fileStorage, va(c)))
	}), authorizer, permission.FilesCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/files/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewDeleteHandler(dashboardDeleteFileUseCase), authorizer, permission.FilesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/files", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewIndexHandler(dashboardGetFilesUseCase), authorizer, permission.FilesIndex), jwt, userRepository))
	mux.Handle("GET /dashboard/files/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewShowHandler(dashboardGetFileUseCase), authorizer, permission.FilesShow), jwt, userRepository))

	// self files
	mux.Handle("DELETE /api/dashboard/my/files/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewDeleteUserHandler(dashboardDeleteUserFileUseCase), authorizer, permission.SelfFilesDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/files", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardFileAPI.NewIndexUserHandler(dashboardGetUserFilesUseCase), authorizer, permission.SelfFilesIndex), jwt, userRepository))

	// elements
	mux.Handle("POST /api/dashboard/elements", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardElementAPI.NewCreateHandler(dashboardCreateElement.NewUseCase(elementsRepository, va(c)))
	}), authorizer, permission.ElementsCreate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/elements/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewDeleteHandler(dashboardDeleteElementUsecase), authorizer, permission.ElementsDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/elements", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewIndexHandler(dashboardGetElementsUsecase), authorizer, permission.ElementsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/elements/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardElementAPI.NewShowHandler(dashboardGetElementUsecase), authorizer, permission.ElementsShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/elements", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardElementAPI.NewUpdateHandler(dashboardUpdateElement.NewUseCase(elementsRepository, va(c)))
	}), authorizer, permission.ElementsUpdate), jwt, userRepository))

	// contact us
	mux.Handle("DELETE /api/dashboard/contact-us/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardContactAPI.NewDeleteHandler(dashboardDeleteContactMessageUsecase), authorizer, permission.ContactUsDelete), jwt, userRepository))
	mux.Handle("GET /api/dashboard/contact-us", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardContactAPI.NewIndexHandler(dashboardGetContactMessagesUsecase), authorizer, permission.ContactUsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/contact-us/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardContactAPI.NewShowHandler(dashboardGetContactMessageUsecase), authorizer, permission.ContactUsShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/contact-us/{uuid}/read", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardContactAPI.NewMarkAsReadHandler(dashboardMarkContactMessageAsReadUsecase), authorizer, permission.ContactUsMarkAsRead), jwt, userRepository))

	// runner containers
	mux.Handle("GET /api/dashboard/runner/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewIndexHandler(dashboardGetContainersUseCase), authorizer, permission.RunnerContainersIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/runner/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewIndexMineHandler(dashboardGetContainersUseCase), authorizer, permission.SelfRunnerContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewRunHandler(dashboardRunContainerUseCase), authorizer, permission.RunnerContainersCreate), jwt, userRepository))
	mux.Handle("GET /api/dashboard/runner/containers/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewShowHandler(dashboardGetContainerUseCase), authorizer, permission.RunnerContainersShow, permission.SelfRunnerContainersShow), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/runner/containers/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewDeleteHandler(dashboardDeleteContainerUseCase), authorizer, permission.RunnerContainersDelete, permission.SelfRunnerContainersDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/containers/{uuid}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewStopHandler(dashboardStopContainerUseCase), authorizer, permission.RunnerContainersManage, permission.SelfRunnerContainersManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/containers/{uuid}/kill", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewKillHandler(dashboardKillContainerUseCase), authorizer, permission.RunnerContainersManage, permission.SelfRunnerContainersManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/containers/{uuid}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewRestartHandler(dashboardRestartContainerUseCase), authorizer, permission.RunnerContainersManage, permission.SelfRunnerContainersManage), jwt, userRepository))
	mux.Handle("GET /api/dashboard/runner/containers/{uuid}/logs", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerContainerAPI.NewLogsHandler(dashboardGetContainerLogsUseCase), authorizer, permission.RunnerContainersLogs, permission.SelfRunnerContainersLogs), jwt, userRepository))

	// runner stacks
	mux.Handle("GET /api/dashboard/runner/stacks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewIndexHandler(dashboardGetStacksUseCase), authorizer, permission.RunnerStacksIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/runner/stacks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewIndexMineHandler(dashboardGetStacksUseCase), authorizer, permission.SelfRunnerStacksIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/stacks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewRunHandler(dashboardRunStackUseCase), authorizer, permission.RunnerStacksCreate), jwt, userRepository))
	mux.Handle("GET /api/dashboard/runner/stacks/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewShowHandler(dashboardGetStackUseCase), authorizer, permission.RunnerStacksShow, permission.SelfRunnerStacksShow), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/runner/stacks/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewDeleteHandler(dashboardDeleteStackUseCase), authorizer, permission.RunnerStacksDelete, permission.SelfRunnerStacksDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/stacks/{uuid}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewStopHandler(dashboardStopStackUseCase), authorizer, permission.RunnerStacksManage, permission.SelfRunnerStacksManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/stacks/{uuid}/kill", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewKillHandler(dashboardKillStackUseCase), authorizer, permission.RunnerStacksManage, permission.SelfRunnerStacksManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/runner/stacks/{uuid}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardRunnerStackAPI.NewRestartHandler(dashboardRestartStackUseCase), authorizer, permission.RunnerStacksManage, permission.SelfRunnerStacksManage), jwt, userRepository))

	// config
	mux.Handle("GET /api/dashboard/config", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardConfigAPI.NewShowHandler(dashboardGetConfigUsecase), authorizer, permission.ConfigShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/config", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardConfigAPI.NewUpdateHandler(dashboardUpdateConfig.NewUseCase(configRepository, languageRepository, va(c), tr(c)))
	}), authorizer, permission.ConfigUpdate), jwt, userRepository))

	rateLimited, err := middleware.NewRateLimitMiddleware(mux, 600, 1*time.Minute)
	if err != nil {
		return nil, err
	}

	var tracedProfiler *profiler.TracedProfiler
	if err := iocContainer.Resolve(&tracedProfiler); err != nil {
		return nil, err
	}

	handler := middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/blog",
				// inside Telemetry so profile samples link to the request span
				middleware.NewProfilingMiddleware(
					middleware.NewLogMiddleware(
						middleware.NewCORSMiddleware(
							rateLimited,
						),
						logger,
					),
					tracedProfiler,
				),
			),
		),
		logger,
	)

	webURL := blogConfigs.WebURL
	if len(webURL) == 0 {
		return nil, errors.New("the web url is not configured (--web-url, WEB_URL)")
	}

	// subscribers
	subscribers := map[string]domain.MessageHandler{
		forgetpassword.SendForgetPasswordEmailName: forgetpassword.NewSendForgetPasswordEmailHandler(userRepository, authTokenGenerator, mailer, mailFromAddress, webURL, renderer, translator),
		register.SendRegisterationEmailName:        register.NewSendRegisterationEmailHandler(authTokenGenerator, mailer, mailFromAddress, webURL, renderer, translator),
		taskEvents.HeartbeatName:                   heartbeat.NewHeartbeatHandler(cachedGateway, logger),
		taskEvents.TaskFailedName:                  heartbeat.NewTaskFailedHandler(cachedGateway, logger),
	}

	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(BlogSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
