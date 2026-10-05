// Package core holds the providers every command is built on: its
// configuration, its telemetry, its profiler, and the container itself.
//
// They are kept apart from package providers, which is the blog's composition
// root and so imports nearly the whole module, because what a command imports
// is what it is built from. The vmhost's image is rebuilt and redeployed, and
// every VM on a node stopped with it, only when something its command depends
// on changes (scripts/vmhost-fingerprint.sh), so the vmhost depends on this
// package and on nothing that wires the other services.
package core
