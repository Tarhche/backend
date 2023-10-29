package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"

	getarticle "github.com/khanzadimahdi/testproject.git/application/article/getArticle"
	getarticles "github.com/khanzadimahdi/testproject.git/application/article/getArticles"
	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
	repository "github.com/khanzadimahdi/testproject.git/infrastructure/repository/memory"
	"github.com/khanzadimahdi/testproject.git/presentation/commands"
	"github.com/khanzadimahdi/testproject.git/presentation/http/api/v1/articles"
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
	var datastore sync.Map

	articlesRepository := repository.NewArticlesRepository(&datastore)

	handler := articles.NewArticlesMux(
		getarticle.NewUseCase(articlesRepository),
		getarticles.NewUseCase(articlesRepository),
	)

	return handler
}
