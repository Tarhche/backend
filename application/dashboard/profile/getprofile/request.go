package getprofile

type Request struct {
	// UserUUID is whose profile this is. It is not asked for: it is who the
	// request is from.
	UserUUID string

	// ImpersonatorUUID is who is seeing the dashboard as them, when somebody is.
	// It is not asked for either: it is what the request's own token says.
	ImpersonatorUUID string
}
