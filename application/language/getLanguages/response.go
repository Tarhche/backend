package getlanguages

import "github.com/khanzadimahdi/testproject/domain/language"

type Response struct {
	Items           []languageResponse `json:"items"`
	DefaultLanguage languageResponse   `json:"default_language"`
}

type languageResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func NewResponse(l []language.Language, defaultLanguage language.Language) *Response {
	items := make([]languageResponse, len(l))

	for i := range l {
		items[i].Code = l[i].Code
		items[i].Name = l[i].Name
	}

	return &Response{
		Items: items,
		DefaultLanguage: languageResponse{
			Code: defaultLanguage.Code,
			Name: defaultLanguage.Name,
		},
	}
}
