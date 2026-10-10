package providers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danceable/provider"

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
	"github.com/khanzadimahdi/testproject/application/code/answerCodeRun"
	"github.com/khanzadimahdi/testproject/application/code/runCode"
	codeStop "github.com/khanzadimahdi/testproject/application/code/stop"
	"github.com/khanzadimahdi/testproject/application/comment/createComment"
	"github.com/khanzadimahdi/testproject/application/comment/getComments"
	"github.com/khanzadimahdi/testproject/application/contact/createMessage"
	dashboardCreateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/createArticle"
	dashboardDeleteArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
	dashboardDeleteUserArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteUserArticle"
	dashboardGetArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticle"
	dashboardGetArticles "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticles"
	dashboardGetUserArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getUserArticle"
	dashboardGetUserArticles "github.com/khanzadimahdi/testproject/application/dashboard/article/getUserArticles"
	dashboardUpdateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateArticle"
	dashboardUpdateUserArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateUserArticle"
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
	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	getuser "github.com/khanzadimahdi/testproject/application/dashboard/user/getUser"
	getusers "github.com/khanzadimahdi/testproject/application/dashboard/user/getUsers"
	impersonateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/impersonateUser"
	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
	"github.com/khanzadimahdi/testproject/application/dashboard/user/userchangepassword"
	dashboardConnectNetwork "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/connectNetwork"
	dashboardCreateContainer "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/createContainer"
	dashboardDeleteContainer "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/deleteContainer"
	dashboardDisconnectNetwork "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/disconnectNetwork"
	dashboardGetContainer "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainer"
	dashboardGetContainerLogs "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainerLogs"
	dashboardGetContainerStats "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainerStats"
	dashboardGetContainers "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainers"
	dashboardGetVMContainers "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getVMContainers"
	dashboardRestartContainer "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/restartContainer"
	dashboardStartContainer "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/startContainer"
	dashboardStopContainer "github.com/khanzadimahdi/testproject/application/dashboard/workload/container/stopContainer"
	dashboardDeleteImage "github.com/khanzadimahdi/testproject/application/dashboard/workload/image/deleteImage"
	dashboardGetImages "github.com/khanzadimahdi/testproject/application/dashboard/workload/image/getImages"
	dashboardPullImage "github.com/khanzadimahdi/testproject/application/dashboard/workload/image/pullImage"
	dashboardCreateNetwork "github.com/khanzadimahdi/testproject/application/dashboard/workload/network/createNetwork"
	dashboardDeleteNetwork "github.com/khanzadimahdi/testproject/application/dashboard/workload/network/deleteNetwork"
	dashboardGetNetworks "github.com/khanzadimahdi/testproject/application/dashboard/workload/network/getNetworks"
	dashboardWorkloadPresenter "github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	dashboardCreateSnapshot "github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/createSnapshot"
	dashboardDeleteSnapshot "github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/deleteSnapshot"
	dashboardGetSnapshot "github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshot"
	dashboardGetSnapshots "github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshots"
	dashboardRenameSnapshot "github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/renameSnapshot"
	dashboardCreateStack "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	dashboardDeleteStack "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/deleteStack"
	dashboardGetStack "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStack"
	dashboardGetStacks "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStacks"
	dashboardRestartStack "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/restartStack"
	dashboardStartStack "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/startStack"
	dashboardStopStack "github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/stopStack"
	dashboardCreateVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	dashboardDeleteVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/deleteVM"
	dashboardGetVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVM"
	dashboardGetVMLogs "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMLogs"
	dashboardGetVMs "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMs"
	dashboardRestartVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restartVM"
	dashboardRestoreVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restoreVM"
	dashboardStartVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/startVM"
	dashboardStopVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/stopVM"
	dashboardUpdateVM "github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/updateVM"
	dashboardCreateVolume "github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/createVolume"
	dashboardDeleteVolume "github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/deleteVolume"
	dashboardGetVolumes "github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/getVolumes"
	"github.com/khanzadimahdi/testproject/application/element"
	getFile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/application/home"
	getLanguages "github.com/khanzadimahdi/testproject/application/language/getLanguages"
	languageresolver "github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/application/localize"
	"github.com/khanzadimahdi/testproject/application/oauth"
	approveauthorization "github.com/khanzadimahdi/testproject/application/oauth/approveAuthorization"
	"github.com/khanzadimahdi/testproject/application/oauth/authorize"
	describeauthorization "github.com/khanzadimahdi/testproject/application/oauth/describeAuthorization"
	exchangecode "github.com/khanzadimahdi/testproject/application/oauth/exchangeCode"
	refreshsession "github.com/khanzadimahdi/testproject/application/oauth/refreshSession"
	registerclient "github.com/khanzadimahdi/testproject/application/oauth/registerClient"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/permission"
	translatorContract "github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
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
	oauthclientsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/oauth/clients"
	oauthgrantsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/oauth/grants"
	permissionsrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/permissions"
	rolesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/roles"
	userrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/users"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	infraWebsocket "github.com/khanzadimahdi/testproject/infrastructure/websocket"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
	workloadClient "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
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
	dashboardUserAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/user"
	dashboardWorkloadAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
	dashboardContainerAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/container"
	dashboardImageAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/image"
	dashboardNetworkAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/network"
	dashboardSnapshotAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/snapshot"
	dashboardStackAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/stack"
	dashboardVMAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/vm"
	dashboardVolumeAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload/volume"
	fileAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/file"
	hashtagAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/hashtag"
	homeapi "github.com/khanzadimahdi/testproject/presentation/http/blog/api/home"
	languageAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/language"
	oauthAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/api/oauth"
	mcpAPI "github.com/khanzadimahdi/testproject/presentation/http/blog/mcp"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/openapi"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/presentation/http/router"
	websocketAPI "github.com/khanzadimahdi/testproject/presentation/websocket"
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
	scoper    middleware.Scoper
	terminate func()
}

var _ provider.Provider = &blogProvider{}

// NewBlogProvider takes the scoper that opens each request's scope, which must
// be the manager this provider and the scoped providers are registered with.
func NewBlogProvider(scoper middleware.Scoper) *blogProvider {
	return &blogProvider{scoper: scoper}
}

func (p *blogProvider) Register(ctx context.Context, c provider.Container) error {
	return c.Bind(func() middleware.Scoper { return p.scoper }, provider.Singleton())
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

	cachedGateway := gateway.
		NewCacheDecorator(messageGateway, runCodeCache, logger, runCode.RunCodeRequest).
		Keeping(runCode.RunCodeRequest, runCode.Keepable)

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
	scoper middleware.Scoper,
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
		return middleware.NewLocalizeMiddleware(next, localizer, scoper)
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

	// ---- the code runner ----
	//
	// the blog does not schedule tasks itself. It asks the workload for a
	// snippet's task, which the control plane admits and keeps, so one
	// service owns a task's lifecycle; and it answers the reader from what
	// the nodes running it say.
	workload, err := workloadClient.New(blogConfigs.WorkloadControlPlaneURL, blogConfigs.WorkloadVMDockerImage)
	if err != nil {
		return nil, err
	}

	if err := cachedGateway.Consume(
		context.Background(),
		runCode.RunCodeRequest,
		runCode.NewRunCodeHandler(validator, workload, cachedGateway, logger),
	); err != nil {
		return nil, err
	}

	// what the workload answers a task's ports under, so the code runner can
	// turn an exposed port into an address a reader can open.
	ingressDomain := blogConfigs.WorkloadIngressDomain

	if err := cachedGateway.Consume(
		context.Background(),
		codeStop.StopName,
		codeStop.NewUseCase(workload, validator, cachedGateway, logger),
	); err != nil {
		return nil, err
	}

	// ---- dashboard: the workload ----
	//
	// the dashboard runs no VM, container or stack itself. It establishes who
	// is asking and whether they may, then passes the request to the workload,
	// which owns their lives: a route of the my set asks for the caller's own,
	// one of the workload set for anybody's, and a create is always the
	// caller's. What the workload refuses is said in the reader's language,
	// which is why most of these are built per request.
	//
	// the workload keeps the id of whoever owns a VM, a snapshot or a stack;
	// this is what puts a name to it when the dashboard shows one.
	workloadOwners := dashboardWorkloadPresenter.NewDirectory(userRepository)

	dashboardGetVMUseCase := dashboardGetVM.NewUseCase(workload, workloadOwners, ingressDomain)
	dashboardGetSnapshotsUseCase := dashboardGetSnapshots.NewUseCase(workload, workloadOwners)
	dashboardGetSnapshotUseCase := dashboardGetSnapshot.NewUseCase(workload, workloadOwners)
	dashboardGetStacksUseCase := dashboardGetStacks.NewUseCase(workload, workloadOwners)
	dashboardGetStackUseCase := dashboardGetStack.NewUseCase(workload, workloadOwners)

	// ---- dashboard ----
	getProfileUseCase := getprofile.NewUseCase(userRepository)
	dashboardProfileGetRolesUseCase := getRoles.NewUseCase(rolesRepository)

	dashboardDeleteArticleUsecase := dashboardDeleteArticle.NewUseCase(articlesRepository)
	dashboardGetArticleUsecase := dashboardGetArticle.NewUseCase(articlesRepository, userRepository)
	dashboardGetArticlesUsecase := dashboardGetArticles.NewUseCase(articlesRepository, userRepository, languageRepository)
	dashboardGetUserArticlesUsecase := dashboardGetUserArticles.NewUseCase(articlesRepository, userRepository, languageRepository)
	dashboardGetUserArticleUsecase := dashboardGetUserArticle.NewUseCase(articlesRepository, userRepository)
	dashboardDeleteUserArticleUsecase := dashboardDeleteUserArticle.NewUseCase(articlesRepository)

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

	// ---- oauth ----
	//
	// this estate is the authorization server an MCP client is given a session
	// by. What it hands over is an ordinary session of ours, so an application
	// acts with the permissions of whoever approved it and with no others.
	oauthClientsRepository := oauthclientsrepository.NewRepository(database)
	oauthGrantsRepository := oauthgrantsrepository.NewRepository(database)
	if err := oauthGrantsRepository.EnsureIndexes(context.Background()); err != nil {
		return nil, err
	}

	if err := oauthClientsRepository.EnsureIndexes(context.Background()); err != nil {
		return nil, err
	}

	oauthRequests := oauth.NewRequests(jwt)
	oauthClients := oauth.NewClients(oauthClientsRepository, hasher)

	registerClientUseCase := registerclient.NewUseCase(oauthClientsRepository, hasher)
	authorizeUseCase := authorize.NewUseCase(oauthClientsRepository, oauthRequests)
	describeAuthorizationUseCase := describeauthorization.NewUseCase(oauthClientsRepository, oauthRequests)
	approveAuthorizationUseCase := approveauthorization.NewUseCase(oauthClientsRepository, oauthGrantsRepository, oauthRequests, hasher)
	exchangeCodeUseCase := exchangecode.NewUseCase(oauthGrantsRepository, oauthClients, userRepository, hasher, authTokenGenerator)

	checkHealthUseCase := checkhealth.NewUseCase(
		checkhealth.Dependency{Name: "database", Pinger: infraHealth.NewMongodbPinger(database)},
		checkhealth.Dependency{Name: "messaging", Pinger: infraHealth.NewNatsPinger(natsConnection)},
	)

	webURL := blogConfigs.WebURL
	if len(webURL) == 0 {
		return nil, errors.New("the web url is not configured (--web-url, WEB_URL)")
	}

	// where this API answers, said absolutely: an application looking for a
	// session has to be told where to go, and a relative address says nothing
	// to somebody who is not here yet.
	serviceURL := blogConfigs.ServiceURL
	if len(serviceURL) == 0 {
		return nil, errors.New("the service url is not configured (--service-url, SERVICE_URL)")
	}

	// the router keeps what it registers, so the MCP server below can be held
	// against the routes that exist rather than against a copy of them.
	mux := router.New()

	// ---- health ----
	// the task healthcheck probes this
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
		return authAPI.NewRefreshHandler(refresh.NewUseCase(userRepository, jwt, authTokenGenerator, authorizer, tr(c), va(c)))
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
	mux.Handle("POST /api/dashboard/users/{uuid}/impersonate", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardUserAPI.NewImpersonateHandler(impersonateuser.NewUseCase(userRepository, authTokenGenerator, tr(c), va(c)))
	}), authorizer, permission.UsersImpersonate), jwt, userRepository))

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

	// one's own articles
	mux.Handle("GET /api/dashboard/my/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewIndexUserHandler(dashboardGetUserArticlesUsecase), authorizer, permission.SelfArticlesIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/articles/{correlationUUID}/{language_code}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewShowUserHandler(dashboardGetUserArticleUsecase), authorizer, permission.SelfArticlesShow), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/articles/{correlationUUID}/{language_code}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardArticleAPI.NewDeleteUserHandler(dashboardDeleteUserArticleUsecase), authorizer, permission.SelfArticlesDelete), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/my/articles", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardArticleAPI.NewUpdateUserHandler(dashboardUpdateUserArticle.NewUseCase(articlesRepository, languageRepository, va(c), tr(c)))
	}), authorizer, permission.SelfArticlesUpdate), jwt, userRepository))

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

	// config
	mux.Handle("GET /api/dashboard/config", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardConfigAPI.NewShowHandler(dashboardGetConfigUsecase), authorizer, permission.ConfigShow), jwt, userRepository))
	mux.Handle("PUT /api/dashboard/config", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardConfigAPI.NewUpdateHandler(dashboardUpdateConfig.NewUseCase(configRepository, languageRepository, va(c), tr(c)))
	}), authorizer, permission.ConfigUpdate), jwt, userRepository))

	// workload vms
	mux.Handle("GET /api/dashboard/workload/vms", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewIndexHandler(dashboardGetVMs.NewUseCase(workload, va(c), workloadOwners, ingressDomain), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewCreateHandler(dashboardCreateVM.NewUseCase(workload, va(c), tr(c), workloadOwners, ingressDomain))
	}), authorizer, permission.WorkloadVMsCreate), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardVMAPI.NewShowHandler(dashboardGetVMUseCase, dashboardWorkloadAPI.Anybody), authorizer, permission.WorkloadVMsShow), jwt, userRepository))
	mux.Handle("PATCH /api/dashboard/workload/vms/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewUpdateHandler(dashboardUpdateVM.NewUseCase(workload, va(c), tr(c), workloadOwners, ingressDomain), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsUpdate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/vms/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewDeleteHandler(dashboardDeleteVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/start", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewStartHandler(dashboardStartVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewStopHandler(dashboardStopVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewRestartHandler(dashboardRestartVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/restore", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewRestoreHandler(dashboardRestoreVM.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsManage), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/logs", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewLogsHandler(dashboardGetVMLogs.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadVMsLogs), jwt, userRepository))

	// one's own vms
	mux.Handle("GET /api/dashboard/my/workload/vms", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewIndexHandler(dashboardGetVMs.NewUseCase(workload, va(c), workloadOwners, ingressDomain), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardVMAPI.NewShowHandler(dashboardGetVMUseCase, dashboardWorkloadAPI.Caller), authorizer, permission.SelfWorkloadVMsShow), jwt, userRepository))
	mux.Handle("PATCH /api/dashboard/my/workload/vms/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewUpdateHandler(dashboardUpdateVM.NewUseCase(workload, va(c), tr(c), workloadOwners, ingressDomain), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsUpdate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/vms/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewDeleteHandler(dashboardDeleteVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/start", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewStartHandler(dashboardStartVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewStopHandler(dashboardStopVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewRestartHandler(dashboardRestartVM.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/restore", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewRestoreHandler(dashboardRestoreVM.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsManage), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/logs", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVMAPI.NewLogsHandler(dashboardGetVMLogs.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadVMsLogs), jwt, userRepository))

	// workload snapshots
	mux.Handle("GET /api/dashboard/workload/snapshots", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardSnapshotAPI.NewIndexHandler(dashboardGetSnapshotsUseCase, dashboardWorkloadAPI.Anybody), authorizer, permission.WorkloadSnapshotsIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/snapshots", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardSnapshotAPI.NewCreateHandler(dashboardCreateSnapshot.NewUseCase(workload, va(c), tr(c), workloadOwners))
	}), authorizer, permission.WorkloadSnapshotsCreate), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/snapshots/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardSnapshotAPI.NewShowHandler(dashboardGetSnapshotUseCase, dashboardWorkloadAPI.Anybody), authorizer, permission.WorkloadSnapshotsShow), jwt, userRepository))
	mux.Handle("PATCH /api/dashboard/workload/snapshots/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardSnapshotAPI.NewUpdateHandler(dashboardRenameSnapshot.NewUseCase(workload, va(c), tr(c), workloadOwners), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadSnapshotsUpdate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/snapshots/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardSnapshotAPI.NewDeleteHandler(dashboardDeleteSnapshot.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadSnapshotsDelete), jwt, userRepository))

	// one's own snapshots
	mux.Handle("GET /api/dashboard/my/workload/snapshots", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardSnapshotAPI.NewIndexHandler(dashboardGetSnapshotsUseCase, dashboardWorkloadAPI.Caller), authorizer, permission.SelfWorkloadSnapshotsIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/snapshots/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardSnapshotAPI.NewShowHandler(dashboardGetSnapshotUseCase, dashboardWorkloadAPI.Caller), authorizer, permission.SelfWorkloadSnapshotsShow), jwt, userRepository))
	mux.Handle("PATCH /api/dashboard/my/workload/snapshots/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardSnapshotAPI.NewUpdateHandler(dashboardRenameSnapshot.NewUseCase(workload, va(c), tr(c), workloadOwners), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadSnapshotsUpdate), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/snapshots/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardSnapshotAPI.NewDeleteHandler(dashboardDeleteSnapshot.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadSnapshotsDelete), jwt, userRepository))

	// workload containers in docker vms
	mux.Handle("GET /api/dashboard/workload/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewIndexHandler(dashboardGetContainers.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewCreateHandler(dashboardCreateContainer.NewUseCase(workload, va(c), tr(c)))
	}), authorizer, permission.WorkloadContainersCreate), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewVMIndexHandler(dashboardGetVMContainers.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/containers/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewShowHandler(dashboardGetContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersShow), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/vms/{uuid}/containers/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewDeleteHandler(dashboardDeleteContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/containers/{id}/start", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewStartHandler(dashboardStartContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/containers/{id}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewStopHandler(dashboardStopContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/containers/{id}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewRestartHandler(dashboardRestartContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/containers/{id}/logs", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewLogsHandler(dashboardGetContainerLogs.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersLogs), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/containers/{id}/stats", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewStatsHandler(dashboardGetContainerStats.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersShow), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/containers/{id}/networks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewConnectHandler(dashboardConnectNetwork.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/vms/{uuid}/containers/{id}/networks/{network}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewDisconnectHandler(dashboardDisconnectNetwork.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))

	// one's own containers in docker vms
	mux.Handle("GET /api/dashboard/my/workload/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewIndexHandler(dashboardGetContainers.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/containers", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewVMIndexHandler(dashboardGetVMContainers.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/containers/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewShowHandler(dashboardGetContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersShow), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/vms/{uuid}/containers/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewDeleteHandler(dashboardDeleteContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/containers/{id}/start", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewStartHandler(dashboardStartContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/containers/{id}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewStopHandler(dashboardStopContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/containers/{id}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewRestartHandler(dashboardRestartContainer.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/containers/{id}/logs", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewLogsHandler(dashboardGetContainerLogs.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersLogs), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/containers/{id}/stats", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewStatsHandler(dashboardGetContainerStats.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersShow), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/containers/{id}/networks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewConnectHandler(dashboardConnectNetwork.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/vms/{uuid}/containers/{id}/networks/{network}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardContainerAPI.NewDisconnectHandler(dashboardDisconnectNetwork.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))

	// workload images in docker vms
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/images", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardImageAPI.NewIndexHandler(dashboardGetImages.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/images", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardImageAPI.NewPullHandler(dashboardPullImage.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/vms/{uuid}/images/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardImageAPI.NewDeleteHandler(dashboardDeleteImage.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersDelete), jwt, userRepository))

	// one's own images in docker vms
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/images", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardImageAPI.NewIndexHandler(dashboardGetImages.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/images", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardImageAPI.NewPullHandler(dashboardPullImage.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/vms/{uuid}/images/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardImageAPI.NewDeleteHandler(dashboardDeleteImage.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersDelete), jwt, userRepository))

	// workload networks in docker vms
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/networks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardNetworkAPI.NewIndexHandler(dashboardGetNetworks.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/networks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardNetworkAPI.NewCreateHandler(dashboardCreateNetwork.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/vms/{uuid}/networks/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardNetworkAPI.NewDeleteHandler(dashboardDeleteNetwork.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersDelete), jwt, userRepository))

	// one's own networks in docker vms
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/networks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardNetworkAPI.NewIndexHandler(dashboardGetNetworks.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/networks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardNetworkAPI.NewCreateHandler(dashboardCreateNetwork.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/vms/{uuid}/networks/{id}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardNetworkAPI.NewDeleteHandler(dashboardDeleteNetwork.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersDelete), jwt, userRepository))

	// workload volumes in docker vms
	mux.Handle("GET /api/dashboard/workload/vms/{uuid}/volumes", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVolumeAPI.NewIndexHandler(dashboardGetVolumes.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/vms/{uuid}/volumes", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVolumeAPI.NewCreateHandler(dashboardCreateVolume.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/vms/{uuid}/volumes/{name}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVolumeAPI.NewDeleteHandler(dashboardDeleteVolume.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadContainersDelete), jwt, userRepository))

	// one's own volumes in docker vms
	mux.Handle("GET /api/dashboard/my/workload/vms/{uuid}/volumes", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVolumeAPI.NewIndexHandler(dashboardGetVolumes.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/vms/{uuid}/volumes", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVolumeAPI.NewCreateHandler(dashboardCreateVolume.NewUseCase(workload, va(c), tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersManage), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/vms/{uuid}/volumes/{name}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardVolumeAPI.NewDeleteHandler(dashboardDeleteVolume.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadContainersDelete), jwt, userRepository))

	// workload stacks
	mux.Handle("GET /api/dashboard/workload/stacks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardStackAPI.NewIndexHandler(dashboardGetStacksUseCase, dashboardWorkloadAPI.Anybody), authorizer, permission.WorkloadStacksIndex), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/stacks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewCreateHandler(dashboardCreateStack.NewUseCase(workload, va(c), tr(c), workloadOwners))
	}), authorizer, permission.WorkloadStacksCreate), jwt, userRepository))
	mux.Handle("GET /api/dashboard/workload/stacks/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardStackAPI.NewShowHandler(dashboardGetStackUseCase, dashboardWorkloadAPI.Anybody), authorizer, permission.WorkloadStacksShow), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/workload/stacks/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewDeleteHandler(dashboardDeleteStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadStacksDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/stacks/{uuid}/start", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewStartHandler(dashboardStartStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadStacksManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/stacks/{uuid}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewStopHandler(dashboardStopStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadStacksManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/workload/stacks/{uuid}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewRestartHandler(dashboardRestartStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Anybody)
	}), authorizer, permission.WorkloadStacksManage), jwt, userRepository))

	// one's own stacks
	mux.Handle("GET /api/dashboard/my/workload/stacks", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardStackAPI.NewIndexHandler(dashboardGetStacksUseCase, dashboardWorkloadAPI.Caller), authorizer, permission.SelfWorkloadStacksIndex), jwt, userRepository))
	mux.Handle("GET /api/dashboard/my/workload/stacks/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(dashboardStackAPI.NewShowHandler(dashboardGetStackUseCase, dashboardWorkloadAPI.Caller), authorizer, permission.SelfWorkloadStacksShow), jwt, userRepository))
	mux.Handle("DELETE /api/dashboard/my/workload/stacks/{uuid}", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewDeleteHandler(dashboardDeleteStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadStacksDelete), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/stacks/{uuid}/start", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewStartHandler(dashboardStartStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadStacksManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/stacks/{uuid}/stop", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewStopHandler(dashboardStopStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadStacksManage), jwt, userRepository))
	mux.Handle("POST /api/dashboard/my/workload/stacks/{uuid}/restart", middleware.NewAuthenticateMiddleware(middleware.NewAuthorizeMiddleware(scoped(func(c provider.Container) http.Handler {
		return dashboardStackAPI.NewRestartHandler(dashboardRestartStack.NewUseCase(workload, tr(c)), dashboardWorkloadAPI.Caller)
	}), authorizer, permission.SelfWorkloadStacksManage), jwt, userRepository))

	// ---- oauth ----
	//
	// how an application is given a session of somebody's: it registers, is
	// put to them, and collects what they gave it. Only the answer itself is
	// behind authentication -- the rest is how a client that has never been
	// here finds its way.
	mux.Handle("GET /.well-known/oauth-protected-resource", oauthAPI.NewProtectedResourceHandler(serviceURL))
	mux.Handle("GET /.well-known/oauth-protected-resource/mcp", oauthAPI.NewProtectedResourceHandler(serviceURL))
	mux.Handle("GET /.well-known/oauth-authorization-server", oauthAPI.NewAuthorizationServerHandler(serviceURL))
	// anybody may register, which is what lets an MCP client introduce itself
	// without being issued credentials first. One caller registering twenty
	// applications in an hour is already generous; the rest is somebody
	// filling the collection, and a registration nobody approves is thrown
	// away a day later anyway.
	registerClient, err := middleware.NewRateLimitMiddleware(
		oauthAPI.NewRegisterHandler(registerClientUseCase),
		20,
		1*time.Hour,
	)
	if err != nil {
		return nil, err
	}

	mux.Handle("POST /oauth/register", registerClient)
	mux.Handle("GET /oauth/authorize", oauthAPI.NewAuthorizeHandler(authorizeUseCase, strings.TrimSuffix(webURL, "/")+oauthAPI.ConsentPath))
	mux.Handle("POST /oauth/token", scoped(func(c provider.Container) http.Handler {
		return oauthAPI.NewTokenHandler(
			exchangeCodeUseCase,
			refreshsession.NewUseCase(oauthClients, refresh.NewUseCase(userRepository, jwt, authTokenGenerator, authorizer, tr(c), va(c))),
		)
	}))
	mux.Handle("GET /api/oauth/authorization", oauthAPI.NewDescribeAuthorizationHandler(describeAuthorizationUseCase))
	mux.Handle("POST /api/oauth/authorization", middleware.NewAuthenticateMiddleware(oauthAPI.NewApproveAuthorizationHandler(approveAuthorizationUseCase), jwt, userRepository))

	// ---- mcp ----
	//
	// the same API, as tools. It is built last because it is held against the
	// routes above: it reads what each of them asks of whoever calls it, and
	// refuses to be built at all if one of them has no tool.
	authenticator := auth.NewAuthenticator(jwt, userRepository)

	mcpHandler, err := mcpAPI.NewHandler(mux, authenticator, oauthAPI.ProtectedResourceMetadataURL(serviceURL), logger)
	if err != nil {
		return nil, err
	}
	mux.Handle(mcpAPI.Path, mcpHandler)

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

	// subscribers
	subscribers := map[string]domain.MessageHandler{
		forgetpassword.SendForgetPasswordEmailName: forgetpassword.NewSendForgetPasswordEmailHandler(userRepository, authTokenGenerator, mailer, mailFromAddress, webURL, renderer, translator),
		register.SendRegisterationEmailName:        register.NewSendRegisterationEmailHandler(authTokenGenerator, mailer, mailFromAddress, webURL, renderer, translator),
		kind.HeartbeatName(taskKind.Name):          answerCodeRun.NewHeartbeatHandler(cachedGateway, ingressDomain, logger),
		kind.ResourceActedOnName:                   answerCodeRun.NewResourceActedOnHandler(cachedGateway, logger),
	}

	if err := iocContainer.Bind(func() map[string]domain.MessageHandler {
		return subscribers
	}, provider.Singleton(), provider.WithName(BlogSubscribers)); err != nil {
		return nil, err
	}

	return handler, nil
}
