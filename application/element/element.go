package element

import (
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/domain/user"
)

// ElementRetriever is a service that retrieves elements by venues.
type Retriever struct {
	articleRepository article.Repository
	elementRepository element.Repository
	userRepository    user.Repository
}

// NewRetriever creates a new Retriever.
func NewRetriever(
	articleRepository article.Repository,
	elementRepository element.Repository,
	userRepository user.Repository,
) *Retriever {
	return &Retriever{
		articleRepository: articleRepository,
		elementRepository: elementRepository,
		userRepository:    userRepository,
	}
}

// RetrieveByVenues retrieves elements by venues.
func (r *Retriever) RetrieveByVenues(venues []string, languageCode string) ([]Response, error) {
	elements, err := r.elementRepository.GetByVenues(venues)
	if err != nil {
		return nil, err
	}

	items := make([]component.Item, 0, len(elements))
	for i := range elements {
		items = append(items, elements[i].Body.Items()...)
	}

	uuids := make([]string, len(items))
	for i := range items {
		uuids[i] = items[i].ContentUUID
	}

	articles, err := r.articleRepository.GetByCorrelationUUIDs(uuids, languageCode)
	if err != nil {
		return nil, err
	}

	userUUIDs := make([]string, len(articles))
	for i := range articles {
		userUUIDs[i] = articles[i].AuthorUUID
	}

	users, err := r.userRepository.GetByUUIDs(userUUIDs)
	if err != nil {
		return nil, err
	}

	return NewResponse(elements, articles, users), nil
}
