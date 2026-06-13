package createarticle

import "github.com/khanzadimahdi/testproject/domain"

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	CorrelationUUID string `json:"correlation_uuid,omitempty"`
	LanguageCode    string `json:"language_code,omitempty"`
}
