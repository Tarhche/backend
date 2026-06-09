package getlanguages

import "github.com/khanzadimahdi/testproject/domain/language"

type languageResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type Response struct {
	Items      []languageResponse `json:"items"`
	Pagination pagination         `json:"pagination"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(l []language.Language, totalPages, currentPage uint) *Response {
	items := make([]languageResponse, len(l))

	for i := range l {
		items[i].Code = l[i].Code
		items[i].Name = l[i].Name
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
