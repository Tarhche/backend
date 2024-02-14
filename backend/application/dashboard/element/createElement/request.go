package createelement

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

type validationErrors map[string]string

type Request struct {
	Type   string            `json:"type"`
	Body   element.Component `json:"body"`
	Venues []string          `json:"venues"`
}

func (e *Request) UnmarshalJSON(data []byte) error {
	type Child Request

	var tmp struct {
		Child
		Body json.RawMessage `json:"body"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	switch tmp.Type {
	case "jumbotron":
		j := component.Jumbotron{}
		if err := json.Unmarshal(tmp.Body, &j); err != nil {
			return err
		}
		tmp.Child.Body = j
	case "featured":
		j := component.Featured{}
		if err := json.Unmarshal(tmp.Body, &j); err != nil {
			return err
		}
		tmp.Child.Body = j
	case "item":
		j := component.Item{}
		if err := json.Unmarshal(tmp.Body, &j); err != nil {
			return err
		}
		tmp.Child.Body = j
	default:
		return element.ErrUnSupportedComponent
	}

	*e = Request(tmp.Child)

	return nil
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
