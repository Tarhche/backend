package runtime

import "strings"

// separator is what puts a class in front of what its driver calls a run.
// Docker never puts one in a container ID and vmhost never puts one in a
// VM's, so the first one there is is always this one.
const separator = ":"

// Qualify names a run by its class and by what its driver calls it, so that
// whatever asks about the run later can be sent to the driver holding it
// without anything having to remember which one that is.
//
// A sysbox run keeps the bare name docker gave it: a bare name already
// means sysbox, so what ran before there were classes, and what runs under
// sysbox now, are named exactly as docker names them.
func Qualify(class Class, native string) string {
	if class.OrSysbox() == Sysbox {
		return native
	}

	return string(class) + separator + native
}

// Split takes a run's name apart into its class and what its driver calls it.
// A bare name is sysbox's, which is every name from before there were
// classes.
func Split(id string) (Class, string) {
	class, native, found := strings.Cut(id, separator)
	if !found {
		return Sysbox, id
	}

	return Class(class).OrSysbox(), native
}

// Why a task the workload could not place, or a node could not run, failed.
// They are codes rather than sentences, since the dashboard says them in the
// reader's own language.
const (
	// ReasonNoNodeOffersRuntime is a task whose class no node offers at all,
	// which waiting will not change.
	ReasonNoNodeOffersRuntime = "no_node_offers_runtime"

	// ReasonRuntimeNotOffered is a node refusing a task of a class it does
	// not offer, which placement should never ask of it, and which another
	// node may still run.
	ReasonRuntimeNotOffered = "runtime_not_offered"
)
