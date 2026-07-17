package breeze

import "sync"

// pool.go — sync.Pool definitions for per-request objects.
//
// Pooling strategy (Phase 1.3.4):
//
//   - *Context: pooled. Acquired in OnTraffic, released in the exec closure's
//     deferred cleanup after the response is written. Reset() clears all
//     fields including the lazy store map (required by
//     TestContextStoreNotRetainedAcrossRequests).
//
//   - *HTTPResponse: pooled. Acquired lazily by ensureResponse() (called
//     from WriteString/JSON/HTML/Status/SetHeader). Released in the exec
//     closure's deferred cleanup via releaseContext, which checks if
//     ctx.Res is non-nil and returns it to the pool.
//
//   - *HTTPRequest: NOT pooled in this step. The `owned` slice holds header
//     bytes that string views (req.Path, req.Header keys/values) point into
//     via b2s. Reusing `owned` across requests would break the documented
//     safety contract (types.go:25-26: "Handlers may safely stash header
//     strings in globals or caches without copying them"). HTTPRequest
//     pooling requires either (a) weakening that contract or (b) copying
//     header strings out of `owned` — both are deferred to a future step.
//
// Lifecycle safety:
//
//   - The exec closure in OnTraffic captures ctx and c. After exec returns
//     (normally or via panic recovery), the deferred release runs:
//       1. recover() handles any panic, builds a 500 response, calls
//          AsyncWrite with the wire bytes.
//       2. The release defer (registered FIRST, runs LAST) calls
//          releaseContext(ctx), which resets ctx.Res, ctx.Req, and all
//          other fields, then returns ctx to the pool.
//   - AsyncWrite copies the buffer into gnet's internal out-ring before
//     returning, so the wire bytes are safe to release after the call.
//   - sync.Pool is safe for concurrent use; the worker goroutine can
//     release while the event loop acquires on a different request.

var contextPool = sync.Pool{
        New: func() any { return &Context{} },
}

var responsePool = sync.Pool{
        New: func() any { return &HTTPResponse{} },
}

// paramsPool stores route parameter maps (map[string]string) for reuse
// across requests. The profiling report identified Router.Find's params
// map allocation as 34% of JSON-path bytes — pooling eliminates it.
//
// Lifecycle safety (analyzed before implementation):
//
//   - findRoute acquires a map from the pool when a parametric route
//     matches. The map is pre-sized by the pool's New func (capacity 4,
//     enough for typical 1-2 param routes without growth).
//   - OnTraffic sets ctx.params = params.
//   - Handlers read via ctx.Param(key) / ctx.GetParam(key) — both return
//     string values (copies), so no aliasing.
//   - ctx.GetParams() returns a COPY of the map — safe to stash.
//   - releaseContext clears all keys (delete loop) and returns the map
//     to the pool.
//
// SetParams(p) takes ownership of p. Callers must NOT pass a pooled map
// (which they cannot obtain — the pool is unexported). All current callers
// pass freshly-created maps, so this is safe.
//
// SetParam(key, value) when ctx.params == nil creates a new map via
// make(map[string]string) — NOT from the pool. This is correct because
// SetParam is a user-initiated write (not a route match), and the map
// will be returned to the pool by releaseContext. This means a request
// that calls SetParam but doesn't match a parametric route will allocate
// one map (same as before) — no regression.
var paramsPool = sync.Pool{
        New: func() any { return make(map[string]string, 4) },
}

// acquireContext returns a *Context from the pool. The caller MUST call
// releaseContext when done (typically in a deferred cleanup). Fields are
// zero-valued on first use and cleared by Reset on subsequent uses.
func acquireContext() *Context {
        return contextPool.Get().(*Context)
}

// releaseContext resets all fields on ctx and returns it to the pool.
// If ctx.Res is non-nil, it is also reset and returned to the response pool.
// If ctx.params is non-nil, it is cleared and returned to the params pool.
// If ctx.Req is non-nil, it is left for GC (HTTPRequest is not pooled).
//
// Safe to call on a nil ctx (no-op).
func releaseContext(ctx *Context) {
        if ctx == nil {
                return
        }
        // Release the response to its pool if one was built.
        if ctx.Res != nil {
                releaseResponse(ctx.Res)
                ctx.Res = nil
        }
        // Release the params map to its pool if one was set (route match or
        // user SetParam/SetParams). Clear all keys first so the next request
        // starts with an empty map.
        if ctx.params != nil {
                releaseParams(ctx.params)
                ctx.params = nil
        }
        // Clear all fields. Order doesn't matter, but be thorough — any
        // field left populated would leak data into the next request that
        // acquires this Context from the pool.
        ctx.Conn = nil
        ctx.Req = nil
        ctx.middlewares = nil
        ctx.index = -1
        ctx.store = nil // MUST be nil — TestContextStoreNotRetainedAcrossRequests
        contextPool.Put(ctx)
}

// acquireParams returns a map[string]string from the params pool. The map
// is pre-cleared (empty) — the caller populates it with route parameters.
func acquireParams() map[string]string {
        return paramsPool.Get().(map[string]string)
}

// releaseParams clears all keys from m and returns it to the params pool.
// The map MUST not be referenced by the caller after this call — it will
// be reused by a future request.
func releaseParams(m map[string]string) {
        for k := range m {
                delete(m, k)
        }
        paramsPool.Put(m)
}

// acquireResponse returns a *HTTPResponse from the pool. The caller MUST
// ensure the response is eventually returned via releaseResponse (either
// directly or via releaseContext).
func acquireResponse() *HTTPResponse {
        return responsePool.Get().(*HTTPResponse)
}

// releaseResponse resets all fields on r and returns it to the pool.
//
// The Headers map is set to nil (not cleared in-place) so the GC can
// collect it. Pooling the map separately is a future optimization.
// The shared maps (hdrsJSON/hdrsText/hdrsHTML) are package-level vars
// and are not affected by nil-ing r.Headers.
func releaseResponse(r *HTTPResponse) {
        r.Status = 0
        r.Headers = nil
        r.headersShared = false
        r.Body = nil
        responsePool.Put(r)
}

// ensureResponse returns a *HTTPResponse for ctx, acquiring one from the
// pool if ctx.Res is nil. If ctx.Res already exists (e.g., Status was
// called first), it is reused — no allocation.
//
// All body methods (WriteString/JSON/HTML) and SetHeader call this to
// ensure responses come from the pool rather than via &HTTPResponse{...}
// literals.
func (ctx *Context) ensureResponse() *HTTPResponse {
        if ctx.Res == nil {
                ctx.Res = acquireResponse()
        }
        return ctx.Res
}
