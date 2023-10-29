package repository

import (
	"sync"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject.git/domain/article"
)

func TestNewArticlesRepository(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Error("expects an error")
		}
	}()

	NewArticlesRepository(nil)
}

func TestArticlesRepository_GetAll(t *testing.T) {
	datastore := sync.Map{}
	itemsCount := uint(32)

	for i := uint(0); i < itemsCount; i++ {
		uuid, err := uuid.NewV7()
		if err != nil {
			t.Error("unexpected error")
		}

		datastore.Store(uuid.String(), article.Article{
			UUID: uuid.String(),
		})
	}

	repository := NewArticlesRepository(&datastore)

	offset := uint(5)
	articles, err := repository.GetAll(5, itemsCount)
	if err != nil {
		t.Errorf("unexpected error %q", err)
	}

	if uint(len(articles)) != (itemsCount - offset) {
		t.Errorf("unexpected number of articles %d", len(articles))
	}
}

func TestArticlesRepository_GetOne(t *testing.T) {
	datastore := sync.Map{}
	itemsCount := 20

	for i := 0; i < itemsCount; i++ {
		u, err := uuid.NewV7()
		if err != nil {
			t.Error("unexpected error")
		}

		datastore.Store(u.String(), article.Article{
			UUID: u.String(),
		})
	}

	wantedUUID, err := uuid.NewV7()
	if err != nil {
		t.Error("unexpected error")
	}

	datastore.Store(wantedUUID.String(), article.Article{
		UUID: wantedUUID.String(),
	})

	repository := NewArticlesRepository(&datastore)

	t.Run("finds", func(t *testing.T) {
		article, err := repository.GetOne(wantedUUID.String())
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}

		if article.UUID != wantedUUID.String() {
			t.Errorf("unexpected error %q", err)
		}
	})

	t.Run("not finds", func(t *testing.T) {
		nonExistanceUUID := "some-non-existance-uuid"
		article, err := repository.GetOne(nonExistanceUUID)
		if err == nil {
			t.Errorf("expected an error, but got nothing")
		}

		if len(article.UUID) > 0 {
			t.Error("unexpected article")
		}
	})
}

func TestArticlesRepository_Count(t *testing.T) {
	datastore := sync.Map{}
	itemsCount := 200

	for i := 0; i < itemsCount; i++ {
		uuid, err := uuid.NewV7()
		if err != nil {
			t.Error("unexpected error")
		}

		datastore.Store(uuid.String(), article.Article{
			UUID: uuid.String(),
		})
	}

	repository := NewArticlesRepository(&datastore)

	count, err := repository.Count()
	if err != nil {
		t.Error("unexpected error")
	}

	if count != 200 {
		t.Errorf("unexpected articles count %d", count)
	}
}
