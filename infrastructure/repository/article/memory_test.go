package article

import (
	"github.com/Tarhche/backend/domain/article"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

func TestInMemoryRepository(t *testing.T) {
	t.Run("it retrieves articles", func(t *testing.T) {
		articles := []article.Entity{
			article.Entity{},
			article.Entity{},
			article.Entity{},
		}

		repository := NewInMemoryRepository()
		repository.articles = articles

		got, err := repository.Articles()
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(got, articles) {
			t.Errorf("got %#v, want %#v", got, articles)
		}
	})

	t.Run("it creates articles", func(t *testing.T) {
		repository := NewInMemoryRepository()
		goroutinesCount := runtime.NumCPU() * 10
		wg := sync.WaitGroup{}

		wg.Add(goroutinesCount)
		for i := 0; i < goroutinesCount; i++ {
			go func(i int) {
				defer wg.Done()
				err := repository.CreateArticle(&article.Entity{})
				if err != nil {
					t.Fatal(err)
				}
			}(i)
		}
		wg.Wait()

		got := len(repository.articles)
		if got != goroutinesCount {
			t.Errorf("got %d articles, want %d", len(repository.articles), got)
		}
	})

	t.Run("it updates articles", func(t *testing.T) {
		repository := NewInMemoryRepository()
		goroutinesCount := runtime.NumCPU() * 10
		wg := sync.WaitGroup{}

		repository.articles = make([]article.Entity, goroutinesCount)
		for i := range repository.articles {
			repository.articles[i].ID = strconv.Itoa(i)
		}

		const updatedTitle string = "updated-title"
		wg.Add(goroutinesCount)
		for i := 0; i < goroutinesCount; i++ {
			go func(i int) {
				defer wg.Done()
				anArticle := repository.articles[i]
				anArticle.Title = updatedTitle
				err := repository.UpdateArticle(&anArticle)
				if err != nil {
					t.Fatal(err)
				}
			}(i)
		}
		wg.Wait()

		for _, anArticle := range repository.articles {
			if anArticle.Title != updatedTitle {
				t.Errorf("got %s, want %s", anArticle.Title, updatedTitle)
			}
		}

		if err := repository.UpdateArticle(&article.Entity{ID: "not-found-article"}); err == nil {
			t.Error("expects an error")
		}
	})

	t.Run("it retrieves an article", func(t *testing.T) {
		anArticle := article.Entity{ID: "test-id", Title: "test-title"}
		repository := NewInMemoryRepository()
		repository.articles = []article.Entity{anArticle}

		got, err := repository.Article("test-id")
		if err != nil {
			t.Fatal(err)
		}

		if reflect.DeepEqual(got, anArticle) {
			t.Errorf("got %#v, want %#v", got, anArticle)
		}

		got, err = repository.Article("not-found-id")
		if err == nil {
			t.Error("expects an error")
		}

		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("it deletes an article", func(t *testing.T) {
		repository := NewInMemoryRepository()
		goroutinesCount := runtime.NumCPU() * 10
		wg := sync.WaitGroup{}
		wait := make(chan struct{})

		repository.articles = make([]article.Entity, goroutinesCount)
		for i := range repository.articles {
			repository.articles[i].ID = strconv.Itoa(i)
		}

		wg.Add(goroutinesCount)
		for i := 0; i < goroutinesCount; i++ {
			go func(ID string) {
				defer wg.Done()
				<-wait

				if err := repository.DeleteArticle(ID); err != nil {
					t.Fatal(err)
				}
			}(repository.articles[i].ID)
		}
		close(wait)
		wg.Wait()

		got := len(repository.articles)
		if got != 0 {
			t.Errorf("all articles should be deleted, but %d still exists", got)
		}

		if err := repository.DeleteArticle("not-found-id"); err == nil {
			t.Error("expects an error")
		}
	})
}
