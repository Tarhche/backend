package elements

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

type ElementBson struct {
	UUID      string    `bson:"_id,omitempty"`
	Body      any       `bson:"body"`
	Venues    []string  `bson:"venues"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
	UpdatedAt time.Time `bson:"updated_at,omitempty"`
}

type ItemBson struct {
	Type        string `bson:"type"`
	ContentUUID string `bson:"content_uuid"`
	ContentType string `bson:"content_type"`
}

type JumbotronBson struct {
	Type string   `bson:"type"`
	Item ItemBson `bson:"item"`
}

type FeaturedBson struct {
	Type  string     `bson:"type"`
	Main  ItemBson   `bson:"main"`
	Aside []ItemBson `bson:"aside"`
}

type CardsBson struct {
	Type       string     `bson:"type"`
	Title      string     `bson:"title"`
	IsCarousel bool       `bson:"is_carousel"`
	Items      []ItemBson `bson:"items"`
}

func (e *ElementBson) UnmarshalBSON(data []byte) error {
	var temporary struct {
		Element   ElementBson `bson:",inline"`
		Component struct {
			Type string `bson:"type"`
		} `bson:"body"`
	}

	if err := bson.Unmarshal(data, &temporary); err != nil {
		return err
	}

	switch temporary.Component.Type {
	case component.ComponentTypeItem:
		var item struct {
			Body ItemBson `bson:"body"`
		}

		if err := bson.Unmarshal(data, &item); err != nil {
			return err
		}
		temporary.Element.Body = item.Body
	case component.ComponentTypeJumbotron:
		var jumbotron struct {
			Body JumbotronBson `bson:"body"`
		}

		if err := bson.Unmarshal(data, &jumbotron); err != nil {
			return err
		}
		temporary.Element.Body = jumbotron.Body
	case component.ComponentTypeFeatured:
		var featured struct {
			Body FeaturedBson `bson:"body"`
		}
		if err := bson.Unmarshal(data, &featured); err != nil {
			return err
		}
		temporary.Element.Body = featured.Body
	case component.ComponentTypeCards:
		var cards struct {
			Body CardsBson `bson:"body"`
		}
		if err := bson.Unmarshal(data, &cards); err != nil {
			return err
		}
		temporary.Element.Body = cards.Body
	default:
		return element.ErrUnSupportedComponent
	}

	*e = temporary.Element

	return nil
}

// ToBson converts an element to a BSON object.
func elementToBson(e *element.Element) (ElementBson, error) {
	body, err := componentToBson(e.Body)
	if err != nil {
		return ElementBson{}, err
	}

	bson := ElementBson{
		UUID:      e.UUID,
		Body:      body,
		Venues:    e.Venues,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}

	return bson, nil
}

// toElement converts a BSON object to an element.
func bsonToElement(b *ElementBson) (element.Element, error) {
	body, err := bsonToComponent(b.Body)
	if err != nil {
		return element.Element{}, err
	}

	return element.Element{
		UUID:      b.UUID,
		Body:      body,
		Venues:    b.Venues,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}, nil
}

// componentToBson converts a component to a BSON object.
func componentToBson(c element.Component) (any, error) {
	var bson any

	switch c.Type() {
	case component.ComponentTypeItem:
		item := c.(component.Item)
		bson = ItemBson{
			Type:        item.Type(),
			ContentUUID: item.ContentUUID,
			ContentType: item.ContentType,
		}
	case component.ComponentTypeJumbotron:
		jumbotron := c.(component.Jumbotron)

		item, err := componentToBson(jumbotron.Item)
		if err != nil {
			return nil, err
		}

		bson = JumbotronBson{
			Type: jumbotron.Type(),
			Item: item.(ItemBson),
		}
	case component.ComponentTypeFeatured:
		featured := c.(component.Featured)

		main, err := componentToBson(featured.Main)
		if err != nil {
			return nil, err
		}

		asideItems := make([]ItemBson, len(featured.Items()))
		for i := range featured.Items() {
			item, err := componentToBson(featured.Items()[i])
			if err != nil {
				return nil, err
			}
			asideItems[i] = item.(ItemBson)
		}

		bson = FeaturedBson{
			Type:  featured.Type(),
			Main:  main.(ItemBson),
			Aside: asideItems,
		}
	case component.ComponentTypeCards:
		cards := c.(component.Cards)
		items := make([]ItemBson, len(cards.ItemsList))
		for i := range cards.ItemsList {
			item, err := componentToBson(cards.ItemsList[i])
			if err != nil {
				return nil, err
			}
			items[i] = item.(ItemBson)
		}
		bson = CardsBson{
			Type:       cards.Type(),
			Title:      cards.Title,
			IsCarousel: cards.IsCarousel,
			Items:      items,
		}
	default:
		return nil, element.ErrUnSupportedComponent
	}

	return bson, nil
}

// bsonToComponent converts a BSON object to a component.
func bsonToComponent(b any) (element.Component, error) {
	var c element.Component

	switch b.(type) {
	case ItemBson:
		c = component.Item{
			ContentUUID: b.(ItemBson).ContentUUID,
			ContentType: b.(ItemBson).ContentType,
		}
	case JumbotronBson:
		item, err := bsonToComponent(b.(JumbotronBson).Item)
		if err != nil {
			return nil, err
		}

		c = component.Jumbotron{
			Item: item.(component.Item),
		}
	case FeaturedBson:
		main, err := bsonToComponent(b.(FeaturedBson).Main)
		if err != nil {
			return nil, err
		}

		asideItems := make([]component.Item, len(b.(FeaturedBson).Aside))
		for i := range b.(FeaturedBson).Aside {
			asideItem, err := bsonToComponent(b.(FeaturedBson).Aside[i])
			if err != nil {
				return nil, err
			}
			asideItems[i] = asideItem.(component.Item)
		}

		c = component.Featured{
			Main:  main.(component.Item),
			Aside: asideItems,
		}
	case CardsBson:
		items := make([]component.Item, len(b.(CardsBson).Items))
		for i := range b.(CardsBson).Items {
			item, err := bsonToComponent(b.(CardsBson).Items[i])
			if err != nil {
				return nil, err
			}
			items[i] = item.(component.Item)
		}
		c = component.Cards{
			Title:      b.(CardsBson).Title,
			IsCarousel: b.(CardsBson).IsCarousel,
			ItemsList:  items,
		}
	default:
		return nil, element.ErrUnSupportedComponent
	}

	return c, nil
}
