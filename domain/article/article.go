package article

import (
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
	GetCorrelationUUIDs(offset uint, limit uint) ([]string, error)
	GetAllPublished(languageCode string, offset uint, limit uint) ([]Article, error)
	GetByCorrelationUUIDAndLanguage(correlationUUID string, languageCode string) (Article, error)
	GetOnePublished(correlationUUID string, languageCode string) (Article, error)
	GetByCorrelationUUIDs(correlationUUIDs []string, languageCode string) ([]Article, error)
	GetPublishedLanguageCodes(correlationUUID string) ([]string, error)
	GetMostViewed(languageCode string, limit uint) ([]Article, error)
	CountPublishedByHashtags(hashtags []string, languageCode string) (uint, error)
	GetPublishedByHashtags(hashtags []string, languageCode string, offset uint, limit uint) ([]Article, error)
	CountPublishedByAuthor(authorUUID string, languageCode string) (uint, error)
	GetPublishedByAuthor(authorUUID string, languageCode string, offset uint, limit uint) ([]Article, error)
	CountByCorrelation() (uint, error)
	CountPublished(languageCode string) (uint, error)
	CorrelationExist(correlationUUID string) (bool, error)
	Save(*Article) (string, error)
	DeleteByCorrelationUUIDAndLanguage(correlationUUID string, languageCode string) error
	IncreaseView(uuid string, inc uint) error
}
