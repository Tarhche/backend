package article

import (
	"context"
	"time"
)

type Article struct {
	UUID            string
	Cover           string
	Video           string
	Title           string
	Excerpt         string
	Body            string
	PublishedAt     time.Time
	AuthorUUID      string
	Tags            []string
	ViewCount       uint
	LanguageCode    string
	CorrelationUUID string
}

type Repository interface {
	GetCorrelationUUIDs(ctx context.Context, offset uint, limit uint) ([]string, error)
	GetAllPublished(ctx context.Context, languageCode string, offset uint, limit uint) ([]Article, error)
	GetByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) (Article, error)
	GetOnePublished(ctx context.Context, correlationUUID string, languageCode string) (Article, error)
	GetByCorrelationUUIDs(ctx context.Context, correlationUUIDs []string, languageCode string) ([]Article, error)
	GetPublishedLanguageCodes(ctx context.Context, correlationUUID string) ([]string, error)
	GetMostViewed(ctx context.Context, languageCode string, limit uint) ([]Article, error)
	CountPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string) (uint, error)
	GetPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string, offset uint, limit uint) ([]Article, error)
	CountPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string) (uint, error)
	GetPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string, offset uint, limit uint) ([]Article, error)
	CountByCorrelation(ctx context.Context) (uint, error)
	CountPublished(ctx context.Context, languageCode string) (uint, error)
	CorrelationExist(ctx context.Context, correlationUUID string) (bool, error)
	Save(ctx context.Context, a *Article) (string, error)
	DeleteByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) error
	IncreaseView(ctx context.Context, uuid string, inc uint) error
}
