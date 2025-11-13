package element

import (
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

// ElementRetriever is a service that retrieves elements by venues.
type Retriever struct {
	articleRepository article.Repository
	elementRepository element.Repository
}

// NewRetriever creates a new Retriever.
func NewRetriever(
	articleRepository article.Repository,
	elementRepository element.Repository,
) *Retriever {
	return &Retriever{
		articleRepository: articleRepository,
		elementRepository: elementRepository,
	}
}

// RetrieveByVenues retrieves elements by venues.
func (r *Retriever) RetrieveByVenues(venues []string) ([]Response, error) {
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

	articles, err := r.articleRepository.GetByUUIDs(uuids)
	if err != nil {
		return nil, err
	}

	return NewResponse(elements, articles), nil
}
