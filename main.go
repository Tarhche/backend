package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/gofrs/uuid"
	"github.com/julienschmidt/httprouter"
	createarticle "github.com/khanzadimahdi/testproject.git/application/article/createArticle"
	deletearticle "github.com/khanzadimahdi/testproject.git/application/article/deleteArticle"
	getarticle "github.com/khanzadimahdi/testproject.git/application/article/getArticle"
	getarticles "github.com/khanzadimahdi/testproject.git/application/article/getArticles"
	updatearticle "github.com/khanzadimahdi/testproject.git/application/article/updateArticle"
	"github.com/khanzadimahdi/testproject.git/application/auth/login"
	deletefile "github.com/khanzadimahdi/testproject.git/application/file/deleteFile"
	getfile "github.com/khanzadimahdi/testproject.git/application/file/getFile"
	uploadfile "github.com/khanzadimahdi/testproject.git/application/file/uploadFile"
	"github.com/khanzadimahdi/testproject.git/domain/article"
	"github.com/khanzadimahdi/testproject.git/domain/user"
	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
	"github.com/khanzadimahdi/testproject.git/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject.git/infrastructure/jwt"
	articlesrepository "github.com/khanzadimahdi/testproject.git/infrastructure/repository/mongodb/articles"
	filesrepository "github.com/khanzadimahdi/testproject.git/infrastructure/repository/mongodb/files"
	userrepository "github.com/khanzadimahdi/testproject.git/infrastructure/repository/mongodb/users"
	"github.com/khanzadimahdi/testproject.git/infrastructure/storage/minio"
	"github.com/khanzadimahdi/testproject.git/presentation/commands"
	articleapi "github.com/khanzadimahdi/testproject.git/presentation/http/api/article"
	"github.com/khanzadimahdi/testproject.git/presentation/http/api/auth"
	fileapi "github.com/khanzadimahdi/testproject.git/presentation/http/api/file"
	"github.com/khanzadimahdi/testproject.git/presentation/http/middleware"
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

	loginUseCase := login.NewUseCase(userRepository, j)
	createArticleUsecase := createarticle.NewUseCase(articlesRepository)
	deleteArticleUsecase := deletearticle.NewUseCase(articlesRepository)
	getArticleUsecase := getarticle.NewUseCase(articlesRepository)
	getArticlesUsecase := getarticles.NewUseCase(articlesRepository)
	updateArticleUsecase := updatearticle.NewUseCase(articlesRepository)
	getFileUseCase := getfile.NewUseCase(filesRepository, fileStorage)
	uploadFileUseCase := uploadfile.NewUseCase(filesRepository, fileStorage)
	deleteFileUseCase := deletefile.NewUseCase(filesRepository, fileStorage)

	router := httprouter.New()

	// auth
	router.Handler(http.MethodPost, "/api/auth/login", auth.NewLoginHandler(loginUseCase))

	// articles
	router.Handler(http.MethodPost, "/api/articles", middleware.NewAuthoriseMiddleware(articleapi.NewCreateHandler(createArticleUsecase), j, userRepository))
	router.Handler(http.MethodDelete, "/api/articles/:uuid", middleware.NewAuthoriseMiddleware(articleapi.NewDeleteHandler(deleteArticleUsecase), j, userRepository))
	router.Handler(http.MethodGet, "/api/articles", articleapi.NewIndexHandler(getArticlesUsecase))
	router.Handler(http.MethodGet, "/api/articles/:uuid", articleapi.NewShowHandler(getArticleUsecase))
	router.Handler(http.MethodPut, "/api/articles", middleware.NewAuthoriseMiddleware(articleapi.NewUpdateHandler(updateArticleUsecase), j, userRepository))

	// files
	router.Handler(http.MethodPost, "/api/files", middleware.NewAuthoriseMiddleware(fileapi.NewUploadHandler(uploadFileUseCase), j, userRepository))
	router.Handler(http.MethodDelete, "/api/files/:uuid", middleware.NewAuthoriseMiddleware(fileapi.NewDeleteHandler(deleteFileUseCase), j, userRepository))
	router.Handler(http.MethodGet, "/api/files/:uuid", fileapi.NewShowHandler(getFileUseCase))

	return middleware.NewRateLimitMiddleware(router, 60, 1*time.Minute)
}
