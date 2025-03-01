package translation

const FA = "FA"

var farsi = map[string]string{
	"required_field":                    "این فیلد اجباری است",
	"invalid_value":                     "مقدار ارائه شده نامعتبر است",
	"invalid_email":                     "مقدار ارائه شده باید یک آدرس ایمیل معتبر باشد",
	"repassword":                        "کلمه عبور و تکرار آن باید یکسان باشند",
	"greater_than_zero":                 "مقدار ارائه شده باید بزرگتر از صفر باشد",
	"exceeds_limit":                     "مقدار ارائه شده بیش از حد است",
	"email_already_exists":              "کاربر با ایمیل داده شده از قبل وجود دارد",
	"username_already_exists":           "کاربر با نام کاربری داده شده از قبل وجود دارد",
	"user_already_exists":               "کاربر از قبل وجود دارد",
	"identity_not_exists":               "هویت (ایمیل/نام کاربری) وجود ندارد",
	"invalid_identity_or_password":      "هویت (ایمیل/نام کاربری) یا رمز عبور اشتباه است",
	"one_or_more_permissions_not_exist": "یک یا چند مجوز وجود ندارد",
	"invalid_state_transition":          "تغییر وضعیت غیر ممکن است",
}
