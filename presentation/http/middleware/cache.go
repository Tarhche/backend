package middleware

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

type Cache struct {
	cache domain.Cache
	next  http.Handler
}

var _ http.Handler = &Cache{}

func NewCacheMiddleware(next http.Handler, cache domain.Cache) *Cache {
	return &Cache{
		cache: cache,
		next:  next,
	}
}

func (c *Cache) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if !c.canBeCached(r) {
		c.next.ServeHTTP(rw, r)

		return
	}

	sortURLParams(r.URL)
	cacheKey := generateCacheKey(r)
	cachedResponse := response{Headers: make(http.Header)}

	cachedValue, err := c.cache.Get(cacheKey)
	if err != nil {
		c.next.ServeHTTP(newCacheWriter(&cachedResponse), r)

		_ = c.persistCache(cacheKey, &cachedResponse)

		c.writeResponse(rw, &cachedResponse)

		return
	}

	if err := json.NewDecoder(bytes.NewReader(cachedValue)).Decode(&cachedResponse); err != nil {
		c.next.ServeHTTP(rw, r)
	}

	c.writeResponse(rw, &cachedResponse)
}

func (c *Cache) canBeCached(r *http.Request) bool {
	return r.Method == http.MethodGet
}

func (c *Cache) persistCache(cacheKey string, cachedResponse *response) error {
	var buffer bytes.Buffer

	err := json.NewEncoder(&buffer).Encode(&cachedResponse)
	if err != nil {
		return err
	}

	return c.cache.Set(cacheKey, buffer.Bytes())
}

func (c *Cache) writeResponse(rw http.ResponseWriter, cachedResponse *response) {
	for k, v := range cachedResponse.Headers {
		for i := range v {
			rw.Header().Add(k, v[i])
		}
	}
	rw.WriteHeader(cachedResponse.Status)
	rw.Write(cachedResponse.Body)
}

type response struct {
	Headers http.Header `json:"headers"`
	Body    []byte      `json:"body"`
	Status  int         `json:"status"`
}

type cacheWriter struct {
	response *response
	lock     sync.RWMutex
}

var _ http.ResponseWriter = &cacheWriter{}

func newCacheWriter(cachedResponse *response) *cacheWriter {
	return &cacheWriter{
		response: cachedResponse,
	}
}

func (rw *cacheWriter) Header() http.Header {
	rw.lock.RLock()
	defer rw.lock.RUnlock()

	return rw.response.Headers
}

func (rw *cacheWriter) WriteHeader(statusCode int) {
	rw.lock.Lock()
	defer rw.lock.Unlock()

	rw.response.Status = statusCode
}

func (rw *cacheWriter) Write(b []byte) (int, error) {
	rw.lock.Lock()
	defer rw.lock.Unlock()

	rw.response.Body = append(rw.response.Body, b...)

	return len(b), nil
}

func generateCacheKey(r *http.Request) string {
	hash := fnv.New64a()
	hash.Write([]byte(r.Method))
	hash.Write([]byte(r.URL.String()))

	return strconv.FormatUint(hash.Sum64(), 16)
}

func sortURLParams(URL *url.URL) {
	params := URL.Query()
	for _, param := range params {
		sort.Slice(param, func(i, j int) bool {
			return param[i] < param[j]
		})
	}
	URL.RawQuery = params.Encode()
}
