package getnotes

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 20

type UseCase struct {
	noteRepository     note.Repository
	userRepository     user.Repository
	languageRepository language.Repository
}

func NewUseCase(
	noteRepository note.Repository,
	userRepository user.Repository,
	languageRepository language.Repository,
) *UseCase {
	return &UseCase{
		noteRepository:     noteRepository,
		userRepository:     userRepository,
		languageRepository: languageRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	totalNotes, err := uc.noteRepository.CountByCorrelation(ctx, request.AuthorUUID)
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalNotes / limit

	if (totalPages * limit) != totalNotes {
		totalPages++
	}

	correlationUUIDs, err := uc.noteRepository.GetCorrelationUUIDs(ctx, request.AuthorUUID, offset, limit)
	if err != nil {
		return nil, err
	}

	if len(correlationUUIDs) == 0 {
		return NewResponse(correlationUUIDs, nil, nil, nil, totalPages, currentPage), nil
	}

	notes, err := uc.noteRepository.GetByCorrelationUUIDs(ctx, correlationUUIDs, "")
	if err != nil {
		return nil, err
	}

	userUUIDs := make([]string, len(notes))
	languageCodes := make([]string, len(notes))
	for i := range notes {
		userUUIDs[i] = notes[i].AuthorUUID
		languageCodes[i] = notes[i].LanguageCode
	}

	authors, err := uc.userRepository.GetByUUIDs(ctx, userUUIDs)
	if err != nil {
		return nil, err
	}

	languages, err := uc.languageRepository.GetByCodes(ctx, languageCodes)
	if err != nil {
		return nil, err
	}

	return NewResponse(correlationUUIDs, notes, authors, languages, totalPages, currentPage), nil
}
