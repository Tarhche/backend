package providers

import "sort"

type Response struct {
	Items []providerResponse `json:"items"`
}

type providerResponse struct {
	Name string `json:"name"`
}

func NewResponse(names []string) *Response {
	sort.Strings(names)

	items := make([]providerResponse, len(names))
	for i := range names {
		items[i].Name = names[i]
	}

	return &Response{Items: items}
}
