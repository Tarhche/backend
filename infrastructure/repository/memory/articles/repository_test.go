package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
)

func TestNewArticlesRepository(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Error("expects an error")
		}
	}()

	NewArticlesRepository(nil)
}

func seed(datastore *sync.Map, articles ...article.Article) {
	for _, a := range articles {
		datastore.Store(a.UUID, a)
	}
}

func TestArticlesRepository_GetCorrelationUUIDs(t *testing.T) {
	datastore := sync.Map{}
	seed(&datastore,
		article.Article{UUID: "uuid-13", CorrelationUUID: "correlation-1", LanguageCode: "EN"},
		article.Article{UUID: "uuid-12", CorrelationUUID: "correlation-1", LanguageCode: "FA"},
		article.Article{UUID: "uuid-22", CorrelationUUID: "correlation-2", LanguageCode: "EN"},
		article.Article{UUID: "uuid-21", CorrelationUUID: "correlation-2", LanguageCode: "DE"},
		article.Article{UUID: "uuid-31", CorrelationUUID: "correlation-3", LanguageCode: "FA"},
		// an article without a correlation uuid must never appear in the result
		article.Article{UUID: "uuid-00", CorrelationUUID: "", LanguageCode: "EN"},
	)

	repository := NewArticlesRepository(&datastore)

	t.Run("returns distinct correlation uuids ordered by newest article, skipping empty", func(t *testing.T) {
		// max uuid per group: correlation-3 (uuid-31), correlation-2 (uuid-22), correlation-1 (uuid-13)
		correlationUUIDs, err := repository.GetCorrelationUUIDs(context.Background(), 0, 20)
		assert.NoError(t, err)
		assert.Equal(t, []string{"correlation-3", "correlation-2", "correlation-1"}, correlationUUIDs)
		assert.NotContains(t, correlationUUIDs, "")
	})

	t.Run("paginates over groups", func(t *testing.T) {
		correlationUUIDs, err := repository.GetCorrelationUUIDs(context.Background(), 1, 1)
		assert.NoError(t, err)
		assert.Equal(t, []string{"correlation-2"}, correlationUUIDs)
	})

	t.Run("offset beyond the number of groups returns nothing", func(t *testing.T) {
		correlationUUIDs, err := repository.GetCorrelationUUIDs(context.Background(), 10, 20)
		assert.NoError(t, err)
		assert.Len(t, correlationUUIDs, 0)
	})
}

func TestArticlesRepository_GetByCorrelationUUIDAndLanguage(t *testing.T) {
	datastore := sync.Map{}
	seed(&datastore,
		article.Article{UUID: "uuid-en", CorrelationUUID: "correlation-1", LanguageCode: "EN"},
		article.Article{UUID: "uuid-fa", CorrelationUUID: "correlation-1", LanguageCode: "FA"},
	)

	repository := NewArticlesRepository(&datastore)

	t.Run("finds", func(t *testing.T) {
		a, err := repository.GetByCorrelationUUIDAndLanguage(context.Background(), "correlation-1", "FA")
		assert.NoError(t, err)
		assert.Equal(t, "uuid-fa", a.UUID)
	})

	t.Run("not finds for an unknown language", func(t *testing.T) {
		a, err := repository.GetByCorrelationUUIDAndLanguage(context.Background(), "correlation-1", "DE")
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, a.UUID)
	})

	t.Run("not finds for an unknown correlation", func(t *testing.T) {
		a, err := repository.GetByCorrelationUUIDAndLanguage(context.Background(), "correlation-unknown", "EN")
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, a.UUID)
	})
}

func TestArticlesRepository_CountByCorrelation(t *testing.T) {
	datastore := sync.Map{}
	seed(&datastore,
		article.Article{UUID: "uuid-1", CorrelationUUID: "correlation-1", LanguageCode: "EN"},
		article.Article{UUID: "uuid-2", CorrelationUUID: "correlation-1", LanguageCode: "FA"},
		article.Article{UUID: "uuid-3", CorrelationUUID: "correlation-2", LanguageCode: "EN"},
		article.Article{UUID: "uuid-4", CorrelationUUID: "correlation-3", LanguageCode: "EN"},
		// an article without a correlation uuid must not be counted
		article.Article{UUID: "uuid-5", CorrelationUUID: "", LanguageCode: "EN"},
	)

	repository := NewArticlesRepository(&datastore)

	count, err := repository.CountByCorrelation(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, uint(3), count)
}

func TestArticlesRepository_DeleteByCorrelationUUIDAndLanguage(t *testing.T) {
	datastore := sync.Map{}
	seed(&datastore,
		article.Article{UUID: "uuid-en", CorrelationUUID: "correlation-1", LanguageCode: "EN"},
		article.Article{UUID: "uuid-fa", CorrelationUUID: "correlation-1", LanguageCode: "FA"},
	)

	repository := NewArticlesRepository(&datastore)

	err := repository.DeleteByCorrelationUUIDAndLanguage(context.Background(), "correlation-1", "EN")
	assert.NoError(t, err)

	_, err = repository.GetByCorrelationUUIDAndLanguage(context.Background(), "correlation-1", "EN")
	assert.ErrorIs(t, err, domain.ErrNotExists)

	// the other language in the same correlation group is left untouched
	a, err := repository.GetByCorrelationUUIDAndLanguage(context.Background(), "correlation-1", "FA")
	assert.NoError(t, err)
	assert.Equal(t, "uuid-fa", a.UUID)
}
