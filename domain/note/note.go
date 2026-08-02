package note

import (
	"context"
	"time"
)

// Note is a short, body-only piece of content published by a user. Like an
// article, a note's identity is its CorrelationUUID and each language version
// is a separate document keyed by (correlationUUID, languageCode).
type Note struct {
	UUID            string
	Body            string
	PublishedAt     time.Time
	AuthorUUID      string
	Tags            []string
	LanguageCode    string
	CorrelationUUID string
}

type Repository interface {
	// An empty authorUUID means "every author"; a non-empty one scopes the
	// query to that author's own notes (the dashboard's "my notes" listing).
	GetCorrelationUUIDs(ctx context.Context, authorUUID string, offset uint, limit uint) ([]string, error)
	CountByCorrelation(ctx context.Context, authorUUID string) (uint, error)

	GetByCorrelationUUIDs(ctx context.Context, correlationUUIDs []string, languageCode string) ([]Note, error)
	GetByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) (Note, error)
	GetOnePublished(ctx context.Context, correlationUUID string, languageCode string) (Note, error)
	GetPublishedLanguageCodes(ctx context.Context, correlationUUID string) ([]string, error)

	// excludedAuthorUUIDs drops notes whose author keeps their notes private, so
	// the count and the page stay consistent with each other.
	CountPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string) (uint, error)
	GetPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string, offset uint, limit uint) ([]Note, error)

	CountPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string) (uint, error)
	GetPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string, offset uint, limit uint) ([]Note, error)

	CorrelationExist(ctx context.Context, correlationUUID string) (bool, error)
	Save(ctx context.Context, n *Note) (string, error)
	DeleteByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) error
}
