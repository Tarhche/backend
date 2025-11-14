package component

import (
	"github.com/stretchr/testify/mock"
)

const ComponentTypeMock = "mock"

type MockComponent struct {
	mock.Mock
}

func (c *MockComponent) Items() []Item {
	args := c.Mock.Called()

	if i, ok := args.Get(0).([]Item); ok {
		return i
	}

	return nil
}

func (c *MockComponent) Type() string {
	c.Mock.Called()

	return ComponentTypeMock
}
