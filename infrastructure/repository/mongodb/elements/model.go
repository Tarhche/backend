package elements

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

type ElementBson struct {
	UUID      string            `bson:"_id,omitempty"`
	Type      string            `bson:"type,omitempty"`
	Body      element.Component `bson:"body"`
	Venues    []string          `bson:"venues"`
	CreatedAt time.Time         `bson:"created_at,omitempty"`
	UpdatedAt time.Time         `bson:"updated_at,omitempty"`
}

func (e *ElementBson) UnmarshalBSON(data []byte) error {
	type Child ElementBson

	var tmp struct {
		Child `bson:",inline"`
		Body  bson.Raw `bson:"body"`
	}

	if err := bson.Unmarshal(data, &tmp); err != nil {
		return err
	}

	switch tmp.Type {
	case "jumbotron":
		j := component.Jumbotron{}

		if err := bson.Unmarshal(tmp.Body, &j); err != nil {
			return err
		}
		tmp.Child.Body = j
	case "featured":
		j := component.Featured{}
		if err := bson.Unmarshal(tmp.Body, &j); err != nil {
			return err
		}
		tmp.Child.Body = j
	case "item":
		j := component.Item{}
		if err := bson.Unmarshal(tmp.Body, &j); err != nil {
			return err
		}
		tmp.Child.Body = j
	default:
		return element.ErrUnSupportedComponent
	}

	*e = ElementBson(tmp.Child)

	return nil
}
