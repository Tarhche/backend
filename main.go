package main

import (
	"context"
	"fmt"
	"log"
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
	uploadfile "github.com/khanzadimahdi/testproject.git/application/file/uploadFile"
	"github.com/khanzadimahdi/testproject.git/domain/article"
	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
	articlesrepository "github.com/khanzadimahdi/testproject.git/infrastructure/repository/mongodb/articles"
	"github.com/khanzadimahdi/testproject.git/presentation/commands"
	articleapi "github.com/khanzadimahdi/testproject.git/presentation/http/api/article"
	fileapi "github.com/khanzadimahdi/testproject.git/presentation/http/api/file"
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

	articlesRepository := articlesrepository.NewArticlesRepository(database)

	for i := 0; i <= 1000; i++ {
		u, _ := uuid.NewV7()

		err := articlesRepository.Save(&article.Article{
			UUID:  u.String(),
			Cover: fmt.Sprintf("https://picsum.photos/536/354?rand=%d", time.Now().Nanosecond()),
			Title: fmt.Sprintf("post title [%s]", u),
			Body: fmt.Sprintf(`
				Lorem ipsum is placeholder text commonly used in the graphic, print,
				and publishing industries for previewing layouts and visual mockups. [%s]`, u),
		})

		log.Println(err)
	}

	createArticleUsecase := createarticle.NewUseCase(articlesRepository)
	deleteArticleUsecase := deletearticle.NewUseCase(articlesRepository)
	getArticleUsecase := getarticle.NewUseCase(articlesRepository)
	getArticlesUsecase := getarticles.NewUseCase(articlesRepository)
	updateArticleUsecase := updatearticle.NewUseCase(articlesRepository)
	uploadFileUseCase := uploadfile.NewUseCase(articlesRepository)

	router := httprouter.New()

	// articles
	router.Handler(http.MethodPost, "/api/articles", articleapi.NewCreateHandler(createArticleUsecase))
	router.Handler(http.MethodDelete, "/api/articles/:uuid", articleapi.NewDeleteHandler(deleteArticleUsecase))
	router.Handler(http.MethodGet, "/api/articles", articleapi.NewIndexHandler(getArticlesUsecase))
	router.Handler(http.MethodGet, "/api/articles/:uuid", articleapi.NewShowHandler(getArticleUsecase))
	router.Handler(http.MethodPut, "/api/articles", articleapi.NewUpdateHandler(updateArticleUsecase))

	// files
	router.Handler(http.MethodPost, "/api/files", fileapi.NewUploadHandler(uploadFileUseCase))
	// router.Handler(http.MethodDelete, "/api/files/:uuid", fileapi.NewDeleteHandler(deleteArticleUsecase))
	// router.Handler(http.MethodGet, "/api/files/:uuid", fileapi.NewShowHandler(getArticleUsecase))

	return router
}
