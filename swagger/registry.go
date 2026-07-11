package swagger

import (
	"strings"
	"sync"
	"sync/atomic"
)

// routeEntry is the internal record stored per registered route.
type routeEntry struct {
	method string
	path   string // OpenAPI-style path with {param} placeholders
	doc    RouteDoc
}

var (
	mu         sync.RWMutex
	routes     []routeEntry
	enabled    atomic.Bool
	apiTitle   string
	apiVersion string
	apiDesc    string
)

// Enable activates API documentation collection.
func Enable() {
	enabled.Store(true)
}

// Enabled returns whether Scalar is currently active.
func Enabled() bool {
	return enabled.Load()
}

// Enabled returns whether swagger is currently active.

// SetInfo sets the API title, version, and optional description shown in the UI.
func SetInfo(title, version, description string) {
	mu.Lock()
	defer mu.Unlock()
	apiTitle = title
	apiVersion = version
	apiDesc = description
}

// RegisterRoute records a route's documentation at startup time.
// method should be uppercase ("GET", "POST", …).
// path is the Breeze-style pattern (e.g. "/user/:id").
func RegisterRoute(method, path string, doc RouteDoc) {
	if !enabled.Load() {
		return
	}
	entry := routeEntry{
		method: strings.ToLower(method),
		path:   breezePath(path),
		doc:    doc,
	}
	mu.Lock()
	routes = append(routes, entry)
	mu.Unlock()
}

// allRoutes returns a snapshot of all registered routes.
func allRoutes() []routeEntry {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]routeEntry, len(routes))
	copy(out, routes)
	return out
}

// breezePath converts a Breeze route pattern ("/user/:id") into an OpenAPI
// path template ("/user/{id}").
func breezePath(pattern string) string {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + p[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}
