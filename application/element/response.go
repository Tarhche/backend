package element

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	Type string `json:"type"`
	Body any    `json:"body"`
}

type authorResponse struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

type articleResponse struct {
	UUID        string         `json:"uuid"`
	Cover       string         `json:"cover"`
	Title       string         `json:"title"`
	Author      authorResponse `json:"author"`
	PublishedAt string         `json:"published_at"`
	Excerpt     string         `json:"excerpt"`
	Tags        []string       `json:"tags"`
}

type itemComponentResponse struct {
	Type string `json:"type"`
	Body any    `json:"body"`
}

type featuredComponentResponse struct {
	Main  itemComponentResponse   `json:"main"`
	Aside []itemComponentResponse `json:"aside"`
}

type jumbotronComponentResponse struct {
	Item itemComponentResponse `json:"item"`
}

type cardsComponentResponse struct {
	Title      string                  `json:"title"`
	IsCarousel bool                    `json:"is_carousel"`
	Items      []itemComponentResponse `json:"items"`
}

func NewResponse(elements []element.Element, elementsContent []article.Article, authors []user.User) []Response {
	authorByUUID := make(map[string]user.User, len(authors))
	for i := range authors {
		authorByUUID[authors[i].UUID] = authors[i]
	}

	response := make([]Response, len(elements))
	for i := range elements {
		response[i].Type = elements[i].Body.Type()
		response[i].Body = toComponentResponse(elements[i].Body, elementsContent, authorByUUID)
	}

	return response
}

func toComponentResponse(ec element.Component, elementsContent []article.Article, authors map[string]user.User) any {
	var c any

	if ec.Type() == component.ComponentTypeJumbotron {
		c = toJumbotronResponse(ec.(component.Jumbotron), elementsContent, authors)
	}

	if ec.Type() == component.ComponentTypeFeatured {
		c = toFeaturedResponse(ec.(component.Featured), elementsContent, authors)
	}

	if ec.Type() == component.ComponentTypeItem {
		c = toItemResponse(ec.(component.Item), elementsContent, authors)
	}

	if ec.Type() == component.ComponentTypeCards {
		c = toCardsResponse(ec.(component.Cards), elementsContent, authors)
	}

	return c
}

func toJumbotronResponse(c component.Jumbotron, elementsContent []article.Article, authors map[string]user.User) jumbotronComponentResponse {
	return jumbotronComponentResponse{
		Item: toItemResponse(c.Item, elementsContent, authors),
	}
}

func toFeaturedResponse(c component.Featured, elementsContent []article.Article, authors map[string]user.User) featuredComponentResponse {
	aside := make([]itemComponentResponse, len(c.Aside))

	for i := range c.Aside {
		aside[i] = toItemResponse(c.Aside[i], elementsContent, authors)
	}

	return featuredComponentResponse{
		Main:  toItemResponse(c.Main, elementsContent, authors),
		Aside: aside,
	}
}

func toCardsResponse(c component.Cards, elementsContent []article.Article, authors map[string]user.User) cardsComponentResponse {
	items := make([]itemComponentResponse, len(c.ItemsList))
	for i := range c.ItemsList {
		items[i] = toItemResponse(c.ItemsList[i], elementsContent, authors)
	}

	return cardsComponentResponse{
		Title:      c.Title,
		IsCarousel: c.IsCarousel,
		Items:      items,
	}
}

func toItemResponse(c component.Item, elementsContent []article.Article, authors map[string]user.User) itemComponentResponse {
	var body any
	for i := range elementsContent {
		if elementsContent[i].UUID == c.ContentUUID {
			body = toArticleResponse([]article.Article{elementsContent[i]}, authors)[0]
			break
		}
	}

	return itemComponentResponse{
		Type: c.Type(),
		Body: body,
	}
}

func toArticleResponse(a []article.Article, authors map[string]user.User) []articleResponse {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].Tags = a[i].Tags
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

		if u, ok := authors[a[i].AuthorUUID]; ok {
			items[i].Author.UUID = u.UUID
			items[i].Author.Name = u.Name
			items[i].Author.Avatar = u.Avatar
			items[i].Author.Username = u.Username
		}
	}

	return items
}
