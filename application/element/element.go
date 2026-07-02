package element

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/domain/user"
)

// VenueMatcher reports whether a concrete path matches a (possibly wildcard) venue pattern.
type VenueMatcher interface {
	Match(pattern, path string) bool
}

// ElementRetriever is a service that retrieves elements by venues.
type Retriever struct {
	articleRepository article.Repository
	elementRepository element.Repository
	userRepository    user.Repository
	venueMatcher      VenueMatcher
}

// NewRetriever creates a new Retriever.
func NewRetriever(
	articleRepository article.Repository,
	elementRepository element.Repository,
	userRepository user.Repository,
	venueMatcher VenueMatcher,
) *Retriever {
	return &Retriever{
		articleRepository: articleRepository,
		elementRepository: elementRepository,
		userRepository:    userRepository,
		venueMatcher:      venueMatcher,
	}
}

// RetrieveByVenues retrieves elements whose (possibly wildcard) venue patterns match any
// of the given concrete venues.
func (r *Retriever) RetrieveByVenues(ctx context.Context, venues []string, languageCode string) ([]Response, error) {
	elements, err := r.matchingElements(ctx, venues)
	if err != nil {
		return nil, err
	}

	if len(elements) == 0 {
		return []Response{}, nil
	}

	items := make([]component.Item, 0, len(elements))
	for i := range elements {
		items = append(items, elements[i].Body.Items()...)
	}

	uuids := make([]string, len(items))
	for i := range items {
		uuids[i] = items[i].ContentUUID
	}

	articles, err := r.articleRepository.GetByCorrelationUUIDs(ctx, uuids, languageCode)
	if err != nil {
		return nil, err
	}

	userUUIDs := make([]string, len(articles))
	for i := range articles {
		userUUIDs[i] = articles[i].AuthorUUID
	}

	users, err := r.userRepository.GetByUUIDs(ctx, userUUIDs)
	if err != nil {
		return nil, err
	}

	return NewResponse(elements, articles, users), nil
}

// matchingElements loads all elements and keeps those whose venue patterns match any of the
// given concrete venues.
func (r *Retriever) matchingElements(ctx context.Context, venues []string) ([]element.Element, error) {
	count, err := r.elementRepository.Count(ctx)
	if err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, nil
	}

	all, err := r.elementRepository.GetAll(ctx, 0, count)
	if err != nil {
		return nil, err
	}

	matched := make([]element.Element, 0, len(all))
	for i := range all {
		if r.matchesAnyVenue(all[i].Venues, venues) {
			matched = append(matched, all[i])
		}
	}

	return matched, nil
}

// matchesAnyVenue reports whether any stored venue pattern matches any requested venue.
func (r *Retriever) matchesAnyVenue(patterns []string, venues []string) bool {
	for _, pattern := range patterns {
		for _, venue := range venues {
			if r.venueMatcher.Match(pattern, venue) {
				return true
			}
		}
	}

	return false
}
