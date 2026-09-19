package approveauthorization

// Response is where the browser goes next, which is the application's own
// address either way: it is told what it was given, or why it was not.
type Response struct {
	RedirectTo string `json:"redirect_to"`
}
