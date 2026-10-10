// Package core holds the providers every command is built on: its
// configuration, its telemetry, its profiler, and the container itself.
//
// They are kept apart from package providers, which is the blog's composition
// root and so imports nearly the whole module, so that a command that wires
// none of the other services, the vmhost's, depends on this package alone.
package core
