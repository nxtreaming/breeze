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
}

func (ctx *Context) WriteString(s string) {
	ctx.Res = &HTTPResponse{
		Status:        200,
		Headers:       hdrsText,
		Body:          []byte(s),
		headersShared: true,
	}
}

func (ctx *Context) JSON(data any) {
	d, err := json.Marshal(data)
	if err != nil {
		ctx.Res = &HTTPResponse{
			Status:        400,
			Headers:       hdrsJSON,
			Body:          []byte(`{"message":"error parsing json"}`),
			headersShared: true,
		}
		return
	}
	ctx.Res = &HTTPResponse{
		Status:        200,
		Headers:       hdrsJSON,
		Body:          d,
		headersShared: true,
	}
}

func (ctx *Context) HTML(data []byte) {
	ctx.Res = &HTTPResponse{
		Status:        200,
		Headers:       hdrsHTML,
		Body:          data,
		headersShared: true,
	}
}

// Status sets (or overrides) the response status code.
// For bodyless responses (204, 304) call this alone.
// For responses with a body, call the body method first (JSON/WriteString/HTML),
// then call Status — those methods replace ctx.Res entirely.
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
