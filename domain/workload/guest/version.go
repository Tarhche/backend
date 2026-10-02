package guest

import "errors"

// ErrUnknownVersion is an agent that does not answer in ProtocolVersion: one
// that names another version in VersionHeader, or names none at all, as the
// agents from before the header was added do not.
//
// A machine outlives the vmhost that booted it, so the vmhost that adopts it
// may be newer than the agent inside it. That vmhost cannot drive the machine:
// the routes and shapes it speaks may not be the ones the agent understands,
// and misreading an answer is worse than refusing it. A machine whose agent is
// refused for its version is dead, and its restart policy boots it again with
// the agent this vmhost brought along.
var ErrUnknownVersion = errors.New("the agent does not speak the version of the guest protocol this vmhost speaks")
