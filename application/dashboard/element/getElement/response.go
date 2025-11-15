package getelement

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

type Response struct {
	UUID      string   `json:"uuid"`
	Body      any      `json:"body"`
	Venues    []string `json:"venues"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type itemComponentResponse struct {
	Type        string `json:"type"`
	ContentUUID string `json:"content_uuid"`
	ContentType string `json:"content_type"`
}

type featuredComponentResponse struct {
	Type  string                  `json:"type"`
	Main  itemComponentResponse   `json:"main"`
	Aside []itemComponentResponse `json:"aside"`
}

type jumbotronComponentResponse struct {
	Type string                `json:"type"`
	Item itemComponentResponse `json:"item"`
}

type cardsComponentResponse struct {
	Type       string                  `json:"type"`
	Title      string                  `json:"title"`
	IsCarousel bool                    `json:"is_carousel"`
	Items      []itemComponentResponse `json:"items"`
}

func toComponentResponse(c element.Component) any {
	switch c.Type() {
	case component.ComponentTypeItem:
		return itemComponentResponse{
			Type:        c.Type(),
			ContentUUID: c.(component.Item).ContentUUID,
			ContentType: c.(component.Item).ContentType,
		}
	case component.ComponentTypeFeatured:
		featured := c.(component.Featured)

		aside := make([]itemComponentResponse, len(featured.Aside))
		for i := range featured.Aside {
			aside[i] = toComponentResponse(featured.Aside[i]).(itemComponentResponse)
		}

		return featuredComponentResponse{
			Type:  c.Type(),
			Main:  toComponentResponse(featured.Main).(itemComponentResponse),
			Aside: aside,
		}
	case component.ComponentTypeJumbotron:
		jumbotron := c.(component.Jumbotron)

		return jumbotronComponentResponse{
			Type: c.Type(),
			Item: toComponentResponse(jumbotron.Item).(itemComponentResponse),
		}
	case component.ComponentTypeCards:
		cards := c.(component.Cards)
		items := make([]itemComponentResponse, len(cards.ItemsList))
		for i := range cards.ItemsList {
			items[i] = toComponentResponse(cards.ItemsList[i]).(itemComponentResponse)
		}

		return cardsComponentResponse{
			Type:       c.Type(),
			Title:      cards.Title,
			IsCarousel: cards.IsCarousel,
			Items:      items,
		}
	}

	return nil
}

func NewResponse(e element.Element) *Response {
	return &Response{
		UUID:      e.UUID,
		Body:      toComponentResponse(e.Body),
		Venues:    e.Venues,
		CreatedAt: e.CreatedAt.Format(time.RFC3339),
		UpdatedAt: e.UpdatedAt.Format(time.RFC3339),
	}
}
