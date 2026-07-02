package repository

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
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

func (r *ArticlesRepository) GetCorrelationUUIDs(ctx context.Context, offset uint, limit uint) ([]string, error) {
	// track the newest article (max UUID, which is time-ordered) per correlation
	// so groups can be ordered deterministically.
	maxUUID := make(map[string]string)
	r.datastore.Range(func(_, value any) bool {
		v := value.(article.Article)
		if len(v.CorrelationUUID) == 0 {
			return true
		}
		if cur, ok := maxUUID[v.CorrelationUUID]; !ok || strings.Compare(v.UUID, cur) > 0 {
			maxUUID[v.CorrelationUUID] = v.UUID
		}
		return true
	})

	correlationUUIDs := make([]string, 0, len(maxUUID))
	for correlationUUID := range maxUUID {
		correlationUUIDs = append(correlationUUIDs, correlationUUID)
	}

	// newest group first
	slices.SortFunc(correlationUUIDs, func(a, b string) int {
		return strings.Compare(maxUUID[b], maxUUID[a])
	})

	if offset >= uint(len(correlationUUIDs)) {
		return []string{}, nil
	}

	end := offset + limit
	if end > uint(len(correlationUUIDs)) {
		end = uint(len(correlationUUIDs))
	}

	return correlationUUIDs[offset:end], nil
}

func (r *ArticlesRepository) GetAllPublished(ctx context.Context, languageCode string, offset uint, limit uint) ([]article.Article, error) {
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

func (r *ArticlesRepository) GetByCorrelationUUIDs(ctx context.Context, correlationUUIDs []string, languageCode string) ([]article.Article, error) {
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

func (r *ArticlesRepository) GetPublishedLanguageCodes(ctx context.Context, correlationUUID string) ([]string, error) {
	if len(correlationUUID) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]struct{})
	codes := make([]string, 0, 2)

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
		codes = append(codes, v.LanguageCode)

		return true
	})

	return codes, nil
}

func (r *ArticlesRepository) CorrelationExist(ctx context.Context, correlationUUID string) (bool, error) {
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

func (r *ArticlesRepository) GetMostViewed(ctx context.Context, languageCode string, limit uint) ([]article.Article, error) {
	return nil, nil
}

func (r *ArticlesRepository) CountPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string) (uint, error) {
	return 0, nil
}

func (r *ArticlesRepository) GetPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string, offset uint, limit uint) ([]article.Article, error) {
	return nil, nil
}

func (r *ArticlesRepository) CountPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string) (uint, error) {
	return 0, nil
}

func (r *ArticlesRepository) GetPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string, offset uint, limit uint) ([]article.Article, error) {
	return nil, nil
}

func (r *ArticlesRepository) GetByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) (article.Article, error) {
	var (
		found article.Article
		ok    bool
	)

	r.datastore.Range(func(_, value any) bool {
		item := value.(article.Article)
		if item.CorrelationUUID != correlationUUID {
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

func (r *ArticlesRepository) GetOnePublished(ctx context.Context, correlationUUID string, languageCode string) (article.Article, error) {
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

func (r *ArticlesRepository) CountByCorrelation(ctx context.Context) (uint, error) {
	seen := make(map[string]struct{})

	r.datastore.Range(func(_, value any) bool {
		v := value.(article.Article)
		if len(v.CorrelationUUID) == 0 {
			return true
		}
		seen[v.CorrelationUUID] = struct{}{}

		return true
	})

	return uint(len(seen)), nil
}

func (r *ArticlesRepository) CountPublished(ctx context.Context, languageCode string) (uint, error) {
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

func (r *ArticlesRepository) Save(ctx context.Context, a *article.Article) (string, error) {
	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		a.UUID = UUID.String()
	}

	r.datastore.Store(a.UUID, *a)

	return a.UUID, nil
}

func (r *ArticlesRepository) DeleteByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) error {
	r.datastore.Range(func(key, value any) bool {
		item := value.(article.Article)
		if item.CorrelationUUID != correlationUUID {
			return true
		}
		if len(languageCode) > 0 && item.LanguageCode != languageCode {
			return true
		}

		r.datastore.Delete(key)

		return true
	})

	return nil
}

func (r *ArticlesRepository) IncreaseView(ctx context.Context, uuid string, inc uint) error {

	return nil
}
