package translation

const FA = "fa"

var farsi = map[string]string{
	"error_on_processing_the_request":   "خطا در پردازش درخواست رخ داده است",
	"request_already_exists":            "درخواست قبلا ارسال شده است",
	"too_many_requests":                 "تعداد درخواست‌های در انتظار پاسخ بیش از حد مجاز است",
	"stream_is_not_open":                "جریان داده باز نیست",
	"ports_require_network":             "کانتینر بدون شبکه نمی‌تواند پورتی منتشر کند",
	"invalid_network_policy":            "دسترسی شبکه باید یکی از این موارد باشد: none، isolated، public",
	"container_is_not_running":          "کانتینر در حال اجرا نیست",
	"too_many_services":                 "تعداد سرویس‌های استک بیش از حد مجاز است",
	"required_field":                    "این فیلد اجباری است",
	"invalid_value":                     "مقدار ارائه شده نامعتبر است",
	"invalid_email":                     "مقدار ارائه شده باید یک آدرس ایمیل معتبر باشد",
	"invalid_phone_number":              "شماره تماس باید فقط شامل ارقام و حداقل ۴ رقم باشد",
	"email_or_phone_required":           "وارد کردن ایمیل یا شماره تماس الزامی است",
	"repassword":                        "کلمه عبور و تکرار آن باید یکسان باشند",
	"greater_than_zero":                 "مقدار ارائه شده باید بزرگتر از صفر باشد",
	"exceeds_limit":                     "مقدار ارائه شده بیش از حد است",
	"email_already_exists":              "کاربر با ایمیل داده شده از قبل وجود دارد",
	"username_already_exists":           "کاربر با نام کاربری داده شده از قبل وجود دارد",
	"user_already_exists":               "کاربر از قبل وجود دارد",
	"identity_not_exists":               "هویت (ایمیل/نام کاربری) وجود ندارد",
	"invalid_identity_or_password":      "هویت (ایمیل/نام کاربری) یا رمز عبور اشتباه است",
	"user_is_banned":                    "حساب کاربری شما مسدود شده است، می‌توانید از طریق فرم تماس با ما در ارتباط باشید",
	"one_or_more_permissions_not_exist": "یک یا چند مجوز وجود ندارد",
	"invalid_state_transition":          "تغییر وضعیت غیر ممکن است",
	"registration_email_subject":        "ثبت نام",
	"reset_password_email_subject":      "بازیابی کلمه عبور",
}
