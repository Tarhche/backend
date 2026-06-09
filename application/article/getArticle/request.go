package getarticle

import (
	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	CorrelationUUID string
	LanguageCode    string
}

// Ensure Request implements domain.Validatable
var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.CorrelationUUID) == 0 {
		validationErrors["correlation_uuid"] = "required_field"
	} else {
		if _, err := uuid.FromString(r.CorrelationUUID); err != nil {
			validationErrors["correlation_uuid"] = "invalid_value"
		}
	}

	return validationErrors
}
