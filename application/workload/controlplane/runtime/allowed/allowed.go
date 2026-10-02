// Package allowed is which runtime classes the workload lets a task be run
// with, and which one a task that names none is given.
//
// Both are the platform's to say (WORKLOAD_RUNTIMES, WORKLOAD_DEFAULT_RUNTIME)
// rather than the task's: a class is turned on for everybody by being allowed,
// and turned off again by being taken out, which turns new tasks of it away
// while the ones already running carry on. The class a task resolves to is
// stored on it when it is created, so changing the default later never moves a
// task that is already there.
package allowed

import (
	"errors"
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// Classes is what the platform allows. Its zero value is the platform as it
// always was: sysbox, and nothing else.
type Classes struct {
	classes      []runtime.Class
	defaultClass runtime.Class
}

// New is the classes a task may ask for, in the order the platform names them,
// and the one a task naming none is given, which has to be among them: a
// default nobody may ask for would turn away every task that asks for nothing.
func New(classes []runtime.Class, defaultClass runtime.Class) (Classes, error) {
	if len(classes) == 0 {
		return Classes{}, errors.New("no runtime class is allowed, so no task could be run")
	}

	for _, class := range classes {
		if !class.IsValid() {
			return Classes{}, fmt.Errorf("%q cannot name a runtime class", class)
		}
	}

	if !slices.Contains(classes, defaultClass) {
		return Classes{}, fmt.Errorf("the default runtime class %q is not one a task may ask for (%v)", defaultClass, classes)
	}

	return Classes{classes: slices.Clone(classes), defaultClass: defaultClass}, nil
}

// All is every class a task may ask for, in the order the platform names
// them, which is the order the dashboard offers them in.
func (c Classes) All() []runtime.Class {
	if len(c.classes) == 0 {
		return []runtime.Class{runtime.Sysbox}
	}

	return slices.Clone(c.classes)
}

// Default is the class a task that names none is run with.
func (c Classes) Default() runtime.Class {
	if len(c.defaultClass) == 0 {
		return runtime.Sysbox
	}

	return c.defaultClass
}

// Resolve is the class a task asking for this one is run with: what it asked
// for, or the default when it asked for nothing.
func (c Classes) Resolve(asked runtime.Class) runtime.Class {
	if len(asked) == 0 {
		return c.Default()
	}

	return asked
}

// Allows reports whether a task may be run with a class.
func (c Classes) Allows(class runtime.Class) bool {
	return slices.Contains(c.All(), class)
}
