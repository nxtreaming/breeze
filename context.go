package breeze

import (
	"github.com/goccy/go-json"
	"github.com/panjf2000/gnet/v2"
)

// Pre-built common response headers. These are reused across all responses
// so we never allocate a new map for the standard content types.
// They are marked read-only by convention — never write to them directly.
// SetHeader uses the headersShared flag for copy-on-write protection.
var (
	hdrsJSON = map[string]string{"Content-Type": "application/json"}
	hdrsText = map[string]string{"Content-Type": "text/plain"}
	hdrsHTML = map[string]string{"Content-Type": "text/html; charset=utf-8"}
)

type Context struct {
	Conn        gnet.Conn
	Req         *HTTPRequest
	Res         *HTTPResponse
	params      map[string]string
	middlewares []HandlerFunc
	index       int

	// store is a lazy-initialized typed key-value store for middleware that
	// need to attach structured data (e.g. JWT claims, user objects) to the
	// request context. It is nil until the first Set call, so requests that
	// don't use it pay zero allocation cost.
	//
	// FIX: Added so middleware like JWT can store claims as a typed value
	// instead of a fmt.Sprintf("%v") string that downstream handlers cannot
	// parse back. This follows the same pattern used by Gin, Echo, and Fiber.
	store map[string]any
}

// statusOrDefault returns the status code already set on ctx.Res, if any,
// so that calling Status() before a body method (WriteString/JSON/HTML)
// is not silently discarded. Falls back to def when no status was set yet.
func (ctx *Context) statusOrDefault(def int) int {
	if ctx.Res != nil && ctx.Res.Status != 0 {
		return ctx.Res.Status
	}
	return def
}

func (ctx *Context) WriteString(s string) {
	ctx.Res = &HTTPResponse{
		Status:        ctx.statusOrDefault(200),
		Headers:       hdrsText,
		Body:          []byte(s),
		headersShared: true,
	}
}

func (ctx *Context) JSON(data any) {
	d, err := json.Marshal(data)
	if err != nil {
		ctx.Res = &HTTPResponse{
			Status:        ctx.statusOrDefault(400),
			Headers:       hdrsJSON,
			Body:          []byte(`{"message":"error parsing json"}`),
			headersShared: true,
		}
		return
	}
	ctx.Res = &HTTPResponse{
		Status:        ctx.statusOrDefault(200),
		Headers:       hdrsJSON,
		Body:          d,
		headersShared: true,
	}
}

func (ctx *Context) HTML(data []byte) {
	ctx.Res = &HTTPResponse{
		Status:        ctx.statusOrDefault(200),
		Headers:       hdrsHTML,
		Body:          data,
		headersShared: true,
	}
}

// Status sets (or overrides) the response status code.
//
// Order-independent: Status may be called before or after the body
// methods (JSON/WriteString/HTML). Those methods replace ctx.Res but
// preserve any status code already set via Status, so both of these
// work identically:
//
//	ctx.Status(401); ctx.WriteString("nope")
//	ctx.WriteString("nope"); ctx.Status(401)
//
// For bodyless responses (204, 304) call this alone.
func (ctx *Context) Status(code int) {
	if ctx.Res == nil {
		ctx.Res = &HTTPResponse{
			Status:  code,
			Headers: make(map[string]string),
		}
		return
	}
	ctx.Res.Status = code
}

// SetHeader adds or replaces a single response header.
//
// When the response was built via JSON/WriteString/HTML, its Headers field
// points to a shared package-level map. SetHeader detects this via the
// headersShared flag and performs a copy-on-write before mutating, so the
// shared maps are never clobbered. Subsequent SetHeader calls on the same
// response are direct writes into the private copy.
func (ctx *Context) SetHeader(key, value string) {
	if ctx.Res == nil {
		ctx.Res = &HTTPResponse{
			Status:  200,
			Headers: make(map[string]string, 4),
		}
	}
	// Copy-on-write: upgrade shared map to a private one.
	if ctx.Res.headersShared {
		orig := ctx.Res.Headers
		priv := make(map[string]string, len(orig)+4)
		for k, v := range orig {
			priv[k] = v
		}
		ctx.Res.Headers = priv
		ctx.Res.headersShared = false
	}
	if ctx.Res.Headers == nil {
		ctx.Res.Headers = make(map[string]string, 4)
	}
	ctx.Res.Headers[key] = value
}

// --- Typed store (Set/Get) ---
//
// FIX: These methods provide a typed key-value store so middleware can attach
// structured data (JWT claims, user objects, trace IDs, etc.) to the context
// without serializing to string. The store is lazy-initialized — requests that
// never call Set pay zero allocation cost.
//
// Usage:
//
//	// In middleware:
//	ctx.Set("user", claims)
//
//	// In handler:
//	claims, ok := ctx.Get("user").(jwt.MapClaims)

// Set stores a typed value under key. The store is allocated on first call.
func (ctx *Context) Set(key string, val any) {
	if ctx.store == nil {
		ctx.store = make(map[string]any, 4)
	}
	ctx.store[key] = val
}

// Get retrieves a typed value. Returns (nil, false) if key is absent.
func (ctx *Context) Get(key string) (any, bool) {
	if ctx.store == nil {
		return nil, false
	}
	v, ok := ctx.store[key]
	return v, ok
}

// MustGet retrieves a typed value, panicking if key is absent.
// Use only when you are certain the key was set (e.g. after JWT middleware).
func (ctx *Context) MustGet(key string) any {
	v, ok := ctx.Get(key)
	if !ok {
		panic("breeze: context key not found: " + key)
	}
	return v
}

// --- Params helpers ---

func (ctx *Context) Param(key string) string {
	if ctx.params == nil {
		return ""
	}
	return ctx.params[key]
}

func (ctx *Context) GetParam(key string) string {
	if ctx.params == nil {
		return ""
	}
	return ctx.params[key]
}

func (ctx *Context) SetParam(key, value string) {
	if ctx.params == nil {
		ctx.params = make(map[string]string)
	}
	ctx.params[key] = value
}

func (ctx *Context) SetParams(p map[string]string) {
	if p == nil {
		ctx.params = make(map[string]string)
	} else {
		ctx.params = p
	}
}

func (ctx *Context) GetParams() map[string]string {
	if ctx.params == nil {
		return map[string]string{}
	}
	cpy := make(map[string]string, len(ctx.params))
	for k, v := range ctx.params {
		cpy[k] = v
	}
	return cpy
}

func (ctx *Context) Query(key string) string {
	if ctx.Req == nil || ctx.Req.Query == nil {
		return ""
	}
	return ctx.Req.Query.Get(key)
}

// --- Middleware chain control ---

func (ctx *Context) Next() {
	ctx.index++
	if ctx.index >= len(ctx.middlewares) {
		return
	}
	fn := ctx.middlewares[ctx.index]
	if fn != nil {
		fn(ctx)
	}
}

func (ctx *Context) Abort() {
	ctx.index = len(ctx.middlewares)
}
