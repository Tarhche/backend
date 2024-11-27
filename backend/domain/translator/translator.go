package translator

type Translator interface {
	Translate(key string, options ...func(*Params)) string
}

type Params struct {
	Attributes map[string]string
	Locale     string
}

func WithAttribute(key, value string) func(*Params) {
	return func(p *Params) {
		p.Attributes[key] = value
	}
}

func WithLocale(locale string) func(*Params) {
	return func(p *Params) {
		p.Locale = locale
	}
}
