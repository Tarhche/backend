//go:build !linux

// Package agent is the init of every microVM vmhost boots. Machines run
// Linux, so the agent is built for Linux alone; elsewhere it is only here so
// that the module builds, and refuses to run.
package agent

import (
	"context"
	"errors"
	"log/slog"
)

// errNotLinux is the agent being run anywhere but in a Linux machine.
var errNotLinux = errors.New("the agent is a Linux machine's init, and runs nowhere else")

// Agent is a machine's init.
type Agent struct{}

// New builds the init of the machine it runs in.
func New(logger *slog.Logger) *Agent {
	return &Agent{}
}

// Run refuses to run: there is no machine here to be the init of.
func (a *Agent) Run(ctx context.Context) error {
	return errNotLinux
}

// Halt does nothing: there is no machine here to turn off.
func Halt() {}
