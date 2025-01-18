package home

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
)

type Response struct {
	All      []articleResponse `json:"all"`
	Popular  []articleResponse `json:"popular"`
	Elements []elementResponse `json:"elements"`
}

type articleResponse struct {
	UUID        string   `json:"uuid"`
	Cover       string   `json:"cover"`
	Title       string   `json:"title"`
	Author      author   `json:"author"`
	PublishedAt string   `json:"published_at"`
	Excerpt     string   `json:"excerpt"`
	Tags        []string `json:"tags"`
}

type author struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type elementResponse struct {
	Type string `json:"type"`
	Body any    `json:"body"`
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
	itemComponentResponse
}

func NewResponse(all, popular []article.Article, e []element.Element, elementsContent []article.Article) *Response {
	elements := make([]elementResponse, len(e))
	for i := range e {
		c := toComponentResponse(e[i], elementsContent)
		if c == nil {
			continue
		}

		elements[i] = elementResponse{
			Type: e[i].Type,
			Body: c,
		}
	}

	return &Response{
		All:      toArticleResponse(all),
		Popular:  toArticleResponse(popular),
		Elements: elements,
	}
}

func toComponentResponse(e element.Element, elementsContent []article.Article) any {
	var c any

	if e.Type == "jumbotron" {
		c = toJumbotronResponse(e.Body.(component.Jumbotron), elementsContent)
	}

	if e.Type == "featured" {
		c = toFeaturedResponse(e.Body.(component.Featured), elementsContent)
	}

	if e.Type == "item" {
		c = toItemResponse(e.Body.(component.Item), elementsContent)
	}

	return c
}

func toJumbotronResponse(c component.Jumbotron, elementsContent []article.Article) jumbotronComponentResponse {
	return jumbotronComponentResponse{
		itemComponentResponse: toItemResponse(c.Item, elementsContent),
	}
}

func toFeaturedResponse(c component.Featured, elementsContent []article.Article) featuredComponentResponse {
	aside := make([]itemComponentResponse, len(c.Aside))

	for i := range c.Aside {
		aside[i] = toItemResponse(c.Aside[i], elementsContent)
	}

	return featuredComponentResponse{
		Main:  toItemResponse(c.Main, elementsContent),
		Aside: aside,
	}
}

func toItemResponse(c component.Item, elementsContent []article.Article) itemComponentResponse {
	var body any
	for i := range elementsContent {
		if elementsContent[i].UUID == c.UUID {
			body = toArticleResponse([]article.Article{elementsContent[i]})[0]
			break
		}
	}

	return itemComponentResponse{
		Type: c.Type,
		Body: body,
	}
}

func toArticleResponse(a []article.Article) []articleResponse {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].Tags = a[i].Tags
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

		items[i].Author.Name = a[i].Author.Name
		items[i].Author.Avatar = a[i].Author.Avatar
	}

	return items
}
