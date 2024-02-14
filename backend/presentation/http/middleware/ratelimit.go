package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
)

type RateLimit struct {
	limiter http.Handler
}

var _ http.Handler = &RateLimit{}

func NewRateLimitMiddleware(next http.Handler, tokens uint64, interval time.Duration) *RateLimit {
	store, err := memorystore.New(&memorystore.Config{
		Tokens:   tokens,
		Interval: interval,
	})

	if err != nil {
		log.Fatal(err)
	}

	// Create the HTTP middleware from the store, keying by IP address.
	middleware, err := httplimit.NewMiddleware(store, httplimit.IPKeyFunc())
	if err != nil {
		log.Fatal(err)
	}

	return &RateLimit{limiter: middleware.Handle(next)}
}

func (a *RateLimit) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	a.limiter.ServeHTTP(rw, r)
}
