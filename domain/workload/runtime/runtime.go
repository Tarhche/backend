// Package runtime is what the workload knows about how a task is run: the
// name of the class a task asks for, what a class can do, and what a node
// holds for it.
//
// It is all the control plane, the ingress and the dashboard ever learn about
// runtimes. Which technology stands behind a class — a container on a daemon
// under sysbox-runc, a Firecracker microVM — is the business of the node that
// offers it, so a third runtime is a new driver on the nodes and a new name
// everywhere else, rather than a change to anything that asks for a task.
package runtime

import (
	"fmt"
	"regexp"
	"strings"
)

// Class is the name a task asks to be run with, through compose's own
// runtime: key. It is resolved once, when the task is created, and stored on
// it, so changing the platform's default later never moves a task that is
// already there.
type Class string

const (
	// Sysbox is what ran every task before there were classes: a container on
	// the shared docker daemon, which runs under sysbox-runc. A task, a stack
	// or an execution that names no class is one of these, so nothing stored
	// before classes existed has to change to keep meaning what it meant.
	Sysbox Class = "sysbox"

	// Firecracker is a microVM of the task's own, with a kernel of its own,
	// which vmhost runs on the node's host.
	Firecracker Class = "firecracker"
)

// classPattern is what a class can be called. It travels in execution IDs,
// labels, configuration and URLs, so it is kept to what is safe in all of
// them, and short.
var classPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// IsValid reports whether c can name a class at all. Whether a class may be
// asked for is configuration (WORKLOAD_RUNTIMES), not syntax.
func (c Class) IsValid() bool {
	return classPattern.MatchString(string(c))
}

// OrSysbox is what a class read back from storage or from a message means.
//
// One written before there were classes names none, and every one of those
// ran under sysbox. A new task that names none is not this: it gets the
// platform's default, which is the control plane's to decide
// (WORKLOAD_DEFAULT_RUNTIME).
func (c Class) OrSysbox() Class {
	if len(c) == 0 {
		return Sysbox
	}

	return c
}

func (c Class) String() string {
	return string(c)
}

// ParseClasses reads a comma-separated list of classes, the way
// WORKLOAD_RUNTIMES gives them. Spaces around a class are not part of it, an
// empty item is not a class, and a class named twice is named once.
func ParseClasses(value string) ([]Class, error) {
	classes := make([]Class, 0, 2)
	seen := make(map[Class]bool)

	for item := range strings.SplitSeq(value, ",") {
		class := Class(strings.TrimSpace(item))
		if len(class) == 0 || seen[class] {
			continue
		}

		if !class.IsValid() {
			return nil, fmt.Errorf("%q cannot name a runtime class", class)
		}

		seen[class] = true
		classes = append(classes, class)
	}

	return classes, nil
}
