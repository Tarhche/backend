package component

const ComponentTypeCards = "cards"

type Cards struct {
	Title      string
	IsCarousel bool
	ItemsList  []Item
}

func (c Cards) Items() []Item {
	return c.ItemsList
}

func (c Cards) Type() string {
	return ComponentTypeCards
}
