package createelement

import (
	"encoding/json"
	"strconv"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

type Request struct {
	Body   domain.Validatable `json:"-"`
	Venues []string           `json:"-"`
}

var _ domain.Validatable = &Request{}
var _ json.Unmarshaler = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if errs := r.Body.Validate(); len(errs) > 0 {
		for errKey, errValue := range errs {
			validationErrors["body."+errKey] = errValue
		}
	}

	return validationErrors
}

type itemComponentRequest struct {
	Type        string `json:"type"`
	ContentUUID string `json:"content_uuid"`
	ContentType string `json:"content_type"`
}

var _ domain.Validatable = &itemComponentRequest{}

func (r *itemComponentRequest) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Type) == 0 {
		validationErrors["type"] = "required_field"
	}

	if r.Type != component.ComponentTypeItem {
		validationErrors["type"] = "invalid_value"
	}

	if len(r.ContentUUID) == 0 {
		validationErrors["content_uuid"] = "required_field"
	}

	if len(r.ContentType) == 0 {
		validationErrors["content_type"] = "required_field"
	}

	return validationErrors
}

type featuredComponentRequest struct {
	Type  string                 `json:"type"`
	Main  itemComponentRequest   `json:"main"`
	Aside []itemComponentRequest `json:"aside"`
}

var _ domain.Validatable = &featuredComponentRequest{}

func (r *featuredComponentRequest) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Type) == 0 {
		validationErrors["type"] = "required_field"
	}

	if r.Type != component.ComponentTypeFeatured {
		validationErrors["type"] = "invalid_value"
	}

	if errs := r.Main.Validate(); len(errs) > 0 {
		for errKey, errValue := range errs {
			validationErrors["main."+errKey] = errValue
		}
	}

	for i, aside := range r.Aside {
		if errs := aside.Validate(); len(errs) > 0 {
			for errKey, errValue := range errs {
				validationErrors["aside."+strconv.Itoa(i)+"."+errKey] = errValue
			}
		}
	}

	return validationErrors
}

type jumbotronComponentRequest struct {
	Type string               `json:"type"`
	Item itemComponentRequest `json:"item"`
}

var _ domain.Validatable = &jumbotronComponentRequest{}

func (r *jumbotronComponentRequest) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Type) == 0 {
		validationErrors["type"] = "required_field"
	}

	if r.Type != component.ComponentTypeJumbotron {
		validationErrors["type"] = "invalid_value"
	}

	if errs := r.Item.Validate(); len(errs) > 0 {
		for errKey, errValue := range errs {
			validationErrors["item."+errKey] = errValue
		}
	}

	return validationErrors
}

func (e *Request) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Venues    []string `json:"venues"`
		Component struct {
			Type string `json:"type"`
		} `json:"body"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	switch tmp.Component.Type {
	case component.ComponentTypeItem:
		var component struct {
			Body itemComponentRequest `json:"body"`
		}

		if err := json.Unmarshal(data, &component); err != nil {
			return err
		}
		e.Body = &component.Body
	case component.ComponentTypeJumbotron:
		var component struct {
			Body jumbotronComponentRequest `json:"body"`
		}

		if err := json.Unmarshal(data, &component); err != nil {
			return err
		}
		e.Body = &component.Body
	case component.ComponentTypeFeatured:
		var component struct {
			Body featuredComponentRequest `json:"body"`
		}
		if err := json.Unmarshal(data, &component); err != nil {
			return err
		}
		e.Body = &component.Body
	default:
		return element.ErrUnSupportedComponent
	}

	e.Venues = tmp.Venues

	return nil
}

func (r *Request) ToElement() *element.Element {
	e := &element.Element{
		Venues: r.Venues,
	}

	switch r.Body.(type) {
	case *itemComponentRequest:
		e.Body = component.Item{
			ContentUUID: r.Body.(*itemComponentRequest).ContentUUID,
			ContentType: r.Body.(*itemComponentRequest).ContentType,
		}
	case *jumbotronComponentRequest:
		e.Body = component.Jumbotron{
			Item: component.Item{
				ContentUUID: r.Body.(*jumbotronComponentRequest).Item.ContentUUID,
				ContentType: r.Body.(*jumbotronComponentRequest).Item.ContentType,
			},
		}
	case *featuredComponentRequest:
		main := component.Item{
			ContentUUID: r.Body.(*featuredComponentRequest).Main.ContentUUID,
			ContentType: r.Body.(*featuredComponentRequest).Main.ContentType,
		}

		aside := make([]component.Item, len(r.Body.(*featuredComponentRequest).Aside))
		for i := range r.Body.(*featuredComponentRequest).Aside {
			aside[i] = component.Item{
				ContentUUID: r.Body.(*featuredComponentRequest).Aside[i].ContentUUID,
				ContentType: r.Body.(*featuredComponentRequest).Aside[i].ContentType,
			}
		}

		e.Body = component.Featured{
			Main:  main,
			Aside: aside,
		}
	}

	return e
}
