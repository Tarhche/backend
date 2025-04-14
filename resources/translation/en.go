package translation

const EN = "EN"

var english = map[string]string{
	"error_on_processing_the_request":   "error on processing the request",
	"request_already_exists":            "request already exists",
	"required_field":                    "this field is required",
	"invalid_value":                     "the provided value is invalid",
	"invalid_email":                     "should be a valid email address",
	"repassword":                        "password and it's repeat should be the same",
	"greater_than_zero":                 "the provided value should be greater than zero",
	"exceeds_limit":                     "the provided value exceeds the limits",
	"email_already_exists":              "user with given email already exists",
	"username_already_exists":           "user with given username already exists",
	"user_already_exists":               "user already exists",
	"identity_not_exists":               "identity (email/username) not exists",
	"invalid_identity_or_password":      "identity (email/username) or password is wrong",
	"one_or_more_permissions_not_exist": "one or more of permissions not exist",
	"invalid_state_transition":          "invalid state transition",
}
