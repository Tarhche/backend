package auth

// BannedCode marks a 403 as "this account is banned" rather than "you lack the
// permission for this". Both are 403s, and a client has to tell them apart: a
// permission denial is a dead end for one action, a ban ends the session.
const BannedCode = "user_banned"

// BannedTranslationKey names the message shown to a banned user.
const BannedTranslationKey = "user_is_banned"

// BannedResponse is the body of every "you are banned" 403 — the refused login,
// the refused token refresh, and every authenticated request turned away by the
// Authenticate middleware. Message is already translated, since only the server
// knows which language the user reads.
type BannedResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewBannedResponse(message string) *BannedResponse {
	return &BannedResponse{
		Code:    BannedCode,
		Message: message,
	}
}
