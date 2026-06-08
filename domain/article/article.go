package article

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/language"
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
	GetAll(offset uint, limit uint) ([]Article, error)
	GetAllPublished(languageCode string, offset uint, limit uint) ([]Article, error)
	GetOne(UUID string) (Article, error)
	GetOnePublished(correlationUUID string, languageCode string) (Article, error)
	GetByCorrelationUUIDs(correlationUUIDs []string, languageCode string) ([]Article, error)
	GetPublishedLanguages(correlationUUID string) ([]language.Language, error)
	GetMostViewed(languageCode string, limit uint) ([]Article, error)
	CountPublishedByHashtags(hashtags []string, languageCode string) (uint, error)
	GetPublishedByHashtags(hashtags []string, languageCode string, offset uint, limit uint) ([]Article, error)
	CountPublishedByAuthor(authorUUID string, languageCode string) (uint, error)
	GetPublishedByAuthor(authorUUID string, languageCode string, offset uint, limit uint) ([]Article, error)
	Count() (uint, error)
	CountPublished(languageCode string) (uint, error)
	CorrelationExist(correlationUUID string) (bool, error)
	Save(*Article) (string, error)
	Delete(UUID string) error
	IncreaseView(uuid string, inc uint) error
}
