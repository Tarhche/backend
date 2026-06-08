package repository

import (
	"slices"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
)

type ArticlesRepository struct {
	datastore *sync.Map
}

var _ article.Repository = &ArticlesRepository{}

func NewArticlesRepository(datastore *sync.Map) *ArticlesRepository {
	if datastore == nil {
		panic("datastore should not be nil")
	}

	return &ArticlesRepository{
		datastore: datastore,
	}
}

func (r *ArticlesRepository) GetAll(offset uint, limit uint) ([]article.Article, error) {
	var (
		a []article.Article
		i uint
		j uint
	)

	r.datastore.Range(func(key, value any) bool {
		if i < offset {
			i++
			return true
		}

		a = append(a, value.(article.Article))
		j++

		return j < limit
	})

	return a, nil
}

func (r *ArticlesRepository) GetAllPublished(languageCode string, offset uint, limit uint) ([]article.Article, error) {
	var (
		a []article.Article
		i uint
		j uint
	)

	r.datastore.Range(func(key, value any) bool {
		if i < offset {
			i++
			return true
		}

		article := value.(article.Article)
		if !article.PublishedAt.Before(time.Now()) {
			return true
		}
		if len(languageCode) > 0 && article.LanguageCode != languageCode {
			return true
		}

		a = append(a, article)
		j++

		return j < limit
	})

	return a, nil
}

func (r *ArticlesRepository) GetByCorrelationUUIDs(correlationUUIDs []string, languageCode string) ([]article.Article, error) {
	a := make([]article.Article, 0, len(correlationUUIDs))

	r.datastore.Range(func(key, value any) bool {
		v := value.(article.Article)
		if !slices.Contains(correlationUUIDs, v.CorrelationUUID) {
			return true
		}
		if len(languageCode) > 0 && v.LanguageCode != languageCode {
			return true
		}

		a = append(a, v)

		return true
	})

	return a, nil
}

func (r *ArticlesRepository) GetPublishedLanguages(correlationUUID string) ([]language.Language, error) {
	if len(correlationUUID) == 0 {
		return []language.Language{}, nil
	}

	seen := make(map[string]struct{})
	languages := make([]language.Language, 0, 2)

	r.datastore.Range(func(_, value any) bool {
		v := value.(article.Article)
		if v.CorrelationUUID != correlationUUID {
			return true
		}
		if v.PublishedAt.After(time.Now()) {
			return true
		}
		if _, ok := seen[v.LanguageCode]; ok {
			return true
		}

		seen[v.LanguageCode] = struct{}{}
		languages = append(languages, language.Language{Code: v.LanguageCode})

		return true
	})

	return languages, nil
}

func (r *ArticlesRepository) CorrelationExist(correlationUUID string) (bool, error) {
	if len(correlationUUID) == 0 {
		return false, nil
	}

	exist := false

	r.datastore.Range(func(_, value any) bool {
		if v := value.(article.Article); v.CorrelationUUID == correlationUUID {
			exist = true
			return false
		}

		return true
	})

	return exist, nil
}

func (r *ArticlesRepository) GetMostViewed(languageCode string, limit uint) ([]article.Article, error) {
	return nil, nil
}

func (r *ArticlesRepository) CountPublishedByHashtags(hashtags []string, languageCode string) (uint, error) {
	return 0, nil
}

func (r *ArticlesRepository) GetPublishedByHashtags(hashtags []string, languageCode string, offset uint, limit uint) ([]article.Article, error) {
	return nil, nil
}

func (r *ArticlesRepository) CountPublishedByAuthor(authorUUID string, languageCode string) (uint, error) {
	return 0, nil
}

func (r *ArticlesRepository) GetPublishedByAuthor(authorUUID string, languageCode string, offset uint, limit uint) ([]article.Article, error) {
	return nil, nil
}

func (r *ArticlesRepository) GetOne(UUID string) (article.Article, error) {
	a, ok := r.datastore.Load(UUID)
	if !ok {
		return article.Article{}, domain.ErrNotExists
	}

	return a.(article.Article), nil
}

func (r *ArticlesRepository) GetOnePublished(correlationUUID string, languageCode string) (article.Article, error) {
	var (
		found article.Article
		ok    bool
	)

	r.datastore.Range(func(_, value any) bool {
		item := value.(article.Article)
		if item.CorrelationUUID != correlationUUID {
			return true
		}
		if item.PublishedAt.After(time.Now()) {
			return true
		}
		if len(languageCode) > 0 && item.LanguageCode != languageCode {
			return true
		}

		found = item
		ok = true

		return false
	})

	if !ok {
		return article.Article{}, domain.ErrNotExists
	}

	return found, nil
}

func (r *ArticlesRepository) Count() (uint, error) {
	var c uint

	r.datastore.Range(func(_, _ any) bool {
		c++

		return true
	})

	return c, nil
}

func (r *ArticlesRepository) CountPublished(languageCode string) (uint, error) {
	var c uint

	r.datastore.Range(func(_, value any) bool {
		article := value.(article.Article)
		if !article.PublishedAt.Before(time.Now()) {
			return true
		}
		if len(languageCode) > 0 && article.LanguageCode != languageCode {
			return true
		}
		c++

		return true
	})

	return c, nil
}

func (r *ArticlesRepository) Save(a *article.Article) (string, error) {
	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		a.UUID = UUID.String()
	}

	if len(a.CorrelationUUID) == 0 {
		a.CorrelationUUID = a.UUID
	}

	r.datastore.Store(a.UUID, *a)

	return a.UUID, nil
}

func (r *ArticlesRepository) Delete(UUID string) error {
	r.datastore.Delete(UUID)

	return nil
}

func (r *ArticlesRepository) IncreaseView(uuid string, inc uint) error {

	return nil
}
