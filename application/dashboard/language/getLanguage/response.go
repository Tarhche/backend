package getlanguage

import "github.com/khanzadimahdi/testproject/domain/language"

type Response struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func NewResponse(l language.Language) *Response {
	return &Response{
		Code: l.Code,
		Name: l.Name,
	}
}
