package approveauthorization

// Request is somebody's answer to an application that asked for a session of
// theirs.
type Request struct {
	// RequestToken is the authorization request as the authorization endpoint
	// signed it.
	RequestToken string `json:"request"`

	// Approved is the answer. A request that is not approved is refused, and
	// the application is told so at the address it registered.
	Approved bool `json:"approved"`

	// UserUUID is whose session is being given away. It is who the request's
	// own token is for, filled in by the handler.
	UserUUID string `json:"-"`

	// ImpersonatorUUID says the answer comes from a shadow session. It is
	// filled in by the handler, and it is a refusal: standing in somebody's
	// shoes is not permission to hand their session to an application.
	ImpersonatorUUID string `json:"-"`
}
