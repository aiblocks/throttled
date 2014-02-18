package throttled

import (
	"net/http"
	"sync"
)

var (
	// DefaultDroppedHandler handles the dropped requests that were denied access because
	// of a throttler. By default, returns a 429 status code with a
	// generic message.
	DefaultDroppedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit exceeded", 429)
	})

	// OnError is the function to call when an error occurs on a throttled handler.
	// By default, returns a 500 status code with a generic message.
	OnError = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
)

// The Limiter interface defines the methods required to control access to a
// throttled handler.
type Limiter interface {
	Start()
	Request(http.ResponseWriter, *http.Request) (<-chan bool, error)
}

// Custom creates a Throttler using the provided Limiter implementation.
func Custom(l Limiter) *Throttler {
	return &Throttler{
		limiter: l,
	}
}

// A Throttler controls access to HTTP handlers using a Limiter.
type Throttler struct {
	// DroppedHandler is called if the request is disallowed. If it is nil,
	// the DefaultDroppedHandler variable is used.
	DroppedHandler http.Handler

	limiter Limiter
	// The mutex protects the started flag
	mu      sync.Mutex
	started bool
}

// Throttle wraps a HTTP handler so that its access is controlled by
// the Throttler. It returns the Handler with the throttling logic.
func (t *Throttler) Throttle(h http.Handler) http.Handler {
	droph := t.start()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ch, err := t.limiter.Request(w, r)
		if err != nil {
			OnError(w, r, err)
			return
		}
		ok := <-ch
		if ok {
			h.ServeHTTP(w, r)
		} else {
			droph.ServeHTTP(w, r)
		}
	})
}

// start starts the throttling and returns the effective dropped handler to
// use for requests that were denied access.
func (t *Throttler) start() http.Handler {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Get the effective dropped handler
	drop := t.DroppedHandler
	if drop == nil {
		drop = DefaultDroppedHandler
	}
	if !t.started {
		t.limiter.Start()
		t.started = true
	}
	return drop
}