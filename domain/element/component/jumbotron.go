package component

const ComponentTypeJumbotron = "jumbotron"

type Jumbotron struct {
	Item Item
}

func (c Jumbotron) Items() []Item {
	return []Item{c.Item}
}

func (c Jumbotron) Type() string {
	return ComponentTypeJumbotron
}
