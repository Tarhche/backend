package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/gofrs/uuid"
	"github.com/julienschmidt/httprouter"
	getArticle "github.com/khanzadimahdi/testproject/application/article/getArticle"
	getArticles "github.com/khanzadimahdi/testproject/application/article/getArticles"
	"github.com/khanzadimahdi/testproject/application/auth/login"
	dashboardCreateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/createArticle"
	dashboardDeleteArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
	dashboardGetArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticle"
	dashboardGetArticles "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticles"
	dashboardUpdateArticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateArticle"
	dashboardDeleteFile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteFile"
	dashboardGetFile "github.com/khanzadimahdi/testproject/application/dashboard/file/getFile"
	dashboardUploadFile "github.com/khanzadimahdi/testproject/application/dashboard/file/uploadFile"
	getFile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/application/home"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/console"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	articlesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/articles"
	filesrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/files"
	userrepository "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/users"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
	"github.com/khanzadimahdi/testproject/presentation/commands"
	articleAPI "github.com/khanzadimahdi/testproject/presentation/http/api/article"
	"github.com/khanzadimahdi/testproject/presentation/http/api/auth"
	dashboardArticleAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/article"
	dashboardFileAPI "github.com/khanzadimahdi/testproject/presentation/http/api/dashboard/file"
	fileAPI "github.com/khanzadimahdi/testproject/presentation/http/api/file"
	homeapi "github.com/khanzadimahdi/testproject/presentation/http/api/home"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	console := console.NewConsole(path.Base(os.Args[0]), "Application description", os.Stderr)
	console.Register(commands.NewServeCommand(httpHandler()))
	code := console.Run(ctx, os.Args)

	cancel()
	os.Exit(code)
}

func httpHandler() http.Handler {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://test:test@mongodb:27017"))
	if err != nil {
		panic(err)
	}
	database := client.Database("blog")

	fileStorage, err := minio.New(minio.Options{
		Endpoint:   "minio:9000",
		AccessKey:  "A0o5qAgFQ80j8B18ZvD2",
		SecretKey:  "7RM2qqzBvmpQR78euAHt5k0UIOnNb9y5L9DtJaYT",
		UseSSL:     false,
		BucketName: "blog",
	})

	if err != nil {
		panic(err)
	}

	articlesRepository := articlesrepository.NewArticlesRepository(database)
	filesRepository := filesrepository.NewFilesRepository(database)

	for i := 0; i <= 1000; i++ {
		u, _ := uuid.NewV7()

		articlesRepository.Save(&article.Article{
			UUID:  u.String(),
			Cover: fmt.Sprintf("https://picsum.photos/536/354?rand=%d", time.Now().Nanosecond()),
			Title: fmt.Sprintf("post title [%s]", u),
			Body: fmt.Sprintf(`
				Lorem ipsum is placeholder text commonly used in the graphic, print,
				and publishing industries for previewing layouts and visual mockups. [%s]`, u),
		})
	}

	userRepository := userrepository.NewUsersRepository(database)

	u, _ := uuid.NewV7()
	userRepository.Save(&user.User{
		UUID:     u.String(),
		Name:     "Mahdi Khanzadi",
		Username: "mahdi.khanzadi",
		Password: "123",
	})

	privateKeyData, err := ioutil.ReadFile("/app/key.pem")
	if err != nil {
		panic(err)
	}

	publicKeyData, err := ioutil.ReadFile("/app/key.pem.pub")
	if err != nil {
		panic(err)
	}

	privateKey, err := ecdsa.ParsePrivateKey(privateKeyData)
	if err != nil {
		panic(err)
	}

	publicKey, err := ecdsa.ParsePublicKey(publicKeyData)
	if err != nil {
		panic(err)
	}

	j := jwt.NewJWT(privateKey, publicKey)

	homeUseCase := home.NewUseCase(articlesRepository)

	router := httprouter.New()
	log.SetFlags(log.LstdFlags | log.Llongfile)
	loginUseCase := login.NewUseCase(userRepository, j)
	getArticleUsecase := getArticle.NewUseCase(articlesRepository)
	getArticlesUsecase := getArticles.NewUseCase(articlesRepository)
	getFileUseCase := getFile.NewUseCase(filesRepository, fileStorage)

	// home
	router.Handler(http.MethodGet, "/api/home", homeapi.NewHomeHandler(homeUseCase))

	// auth
	router.Handler(http.MethodPost, "/api/auth/login", auth.NewLoginHandler(loginUseCase))

	// articles
	router.Handler(http.MethodGet, "/api/articles", articleAPI.NewIndexHandler(getArticlesUsecase))
	router.Handler(http.MethodGet, "/api/articles/:uuid", articleAPI.NewShowHandler(getArticleUsecase))

	// files
	router.Handler(http.MethodGet, "/api/files/:uuid", fileAPI.NewShowHandler(getFileUseCase))

	// -------------------- dashboard -------------------- //
	dashboardCreateArticleUsecase := dashboardCreateArticle.NewUseCase(articlesRepository)
	dashboardDeleteArticleUsecase := dashboardDeleteArticle.NewUseCase(articlesRepository)
	dashboardGetArticleUsecase := dashboardGetArticle.NewUseCase(articlesRepository)
	dashboardGetArticlesUsecase := dashboardGetArticles.NewUseCase(articlesRepository)
	dashboardUpdateArticleUsecase := dashboardUpdateArticle.NewUseCase(articlesRepository)
	dashboardGetFileUseCase := dashboardGetFile.NewUseCase(filesRepository, fileStorage)
	dashboardUploadFileUseCase := dashboardUploadFile.NewUseCase(filesRepository, fileStorage)
	dashboardDeleteFileUseCase := dashboardDeleteFile.NewUseCase(filesRepository, fileStorage)

	// articles
	router.Handler(http.MethodPost, "/api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewCreateHandler(dashboardCreateArticleUsecase), j, userRepository))
	router.Handler(http.MethodDelete, "/api/dashboard/articles/:uuid", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewDeleteHandler(dashboardDeleteArticleUsecase), j, userRepository))
	router.Handler(http.MethodGet, "/api/dashboard/articles", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewIndexHandler(dashboardGetArticlesUsecase), j, userRepository))
	router.Handler(http.MethodGet, "/api/dashboard/articles/:uuid", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewShowHandler(dashboardGetArticleUsecase), j, userRepository))
	router.Handler(http.MethodPut, "/api/dashboard/articles/:uuid", middleware.NewAuthoriseMiddleware(dashboardArticleAPI.NewUpdateHandler(dashboardUpdateArticleUsecase), j, userRepository))

	// files
	router.Handler(http.MethodPost, "/api/dashboard/files", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewUploadHandler(dashboardUploadFileUseCase), j, userRepository))
	router.Handler(http.MethodDelete, "/api/dashboard/files/:uuid", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewDeleteHandler(dashboardDeleteFileUseCase), j, userRepository))
	router.Handler(http.MethodGet, "/api/dashboard/files/:uuid", middleware.NewAuthoriseMiddleware(dashboardFileAPI.NewShowHandler(dashboardGetFileUseCase), j, userRepository))

	return middleware.NewRateLimitMiddleware(router, 60, 1*time.Minute)
}
