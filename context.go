package breeze

import (
	"github.com/goccy/go-json"
	"github.com/panjf2000/gnet/v2"
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
		Status:  200,
		Headers: map[string]string{"Content-Type": "text/plain"},
		Body:    []byte(s),
	}
}

func (ctx *Context) JSON(data any) {
	d, err := json.Marshal(data)
	if err != nil {
		ctx.Res = &HTTPResponse{
			Status:  400,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"message":"error parsing json"}`),
		}
		return
	}
	ctx.Res = &HTTPResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    d,
	}
}

func (ctx *Context) Status(code int) {
	if ctx.Res == nil {
		ctx.Res = &HTTPResponse{Headers: make(map[string]string)}
	}
	ctx.Res.Status = code
}

func (ctx *Context) Param(key string) string {
	if ctx.params == nil {
		return ""
	}
	return ctx.params[key]
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

	// Stop if we've run all middlewares (including the handler)
	if ctx.index >= len(ctx.middlewares) {
		return
	}

	// Execute the current middleware or handler
	fn := ctx.middlewares[ctx.index]
	if fn != nil {
		fn(ctx)
	}
}

func (ctx *Context) Abort() {
	ctx.index = len(ctx.middlewares)
}
