package breeze

import (
        "os"
        "path/filepath"
        "strings"
)

type HandlerFunc func(*Context)

type route struct {
        method       Method
        pattern      string
        segments     []string
        // paramIndex[i] is true when segments[i] is a :param (cached at registration)
        paramIndex   []bool
        handler      HandlerFunc // finalHandler closure (for backward compat with public Find)
        // userHandler is the actual handler passed to Handle, without the
        // finalHandler wrapper. Used by findRoute to build the chain in one alloc.
        userHandler  HandlerFunc
        // routeMWs is the per-route middleware slice (defensive copy made at
        // registration). Used by findRoute to build the full middleware chain
        // in a single allocation in OnTraffic, eliminating the double-build
        // that finalHandler caused.
        routeMWs     []HandlerFunc
        hasWildcard  bool
        wildcardName string
        // paramCount is the number of :param segments (pre-counted to pre-size the map)
        paramCount int
}

type Router struct {
        routes        []*route
        middlewares   []HandlerFunc
        staticDir     string
        autoServeRoot bool
}

func NewRouter() *Router {
        return &Router{
                staticDir:     "./public",
                autoServeRoot: true,
        }
}

// RouteInfo exposes read-only information about a registered route so
// external packages (e.g. the dashboard) can inspect the routing table
// without depending on the unexported `route` struct.
type RouteInfo interface {
        Method() Method
        Pattern() string
        Segments() []string
        HasWildcard() bool
        WildcardName() string
        ParamCount() int
}

func (r *route) Method() Method      { return r.method }
func (r *route) Pattern() string     { return r.pattern }
func (r *route) Segments() []string  { return r.segments }
func (r *route) HasWildcard() bool   { return r.hasWildcard }
func (r *route) WildcardName() string { return r.wildcardName }
func (r *route) ParamCount() int     { return r.paramCount }

// RoutesInfo returns the routing table as a slice of RouteInfo so external
// packages can iterate without depending on the unexportd *route type.
func (r *Router) RoutesInfo() []RouteInfo {
        out := make([]RouteInfo, len(r.routes))
        for i, rt := range r.routes {
                out[i] = rt
        }
        return out
}

func (r *Router) Use(mw ...HandlerFunc) {
        r.middlewares = append(r.middlewares, mw...)
}

func (r *Router) Routes() []*route {
        return r.routes
}

func (r *Router) SetStaticDir(dir string) {
        r.staticDir = dir
}

func (r *Router) Handle(method Method, pattern string, handler HandlerFunc, middlewares ...HandlerFunc) {
        if pattern == "" || pattern[0] != '/' {
                panic("invalid route pattern: must start with '/'")
        }

        trimmed := strings.Trim(pattern, "/")
        var segments []string
        hasWildcard := false
        wildcardName := ""

        if trimmed == "" {
                segments = []string{}
        } else {
                segments = strings.Split(trimmed, "/")
                last := segments[len(segments)-1]

                if strings.HasPrefix(last, "*") {
                        hasWildcard = true
                        if len(last) > 1 {
                                wildcardName = last[1:]
                        } else {
                                wildcardName = "wildcard"
                        }
                        segments = segments[:len(segments)-1]
                }
        }

        // Pre-compute which segments are params and how many there are.
        paramIdx := make([]bool, len(segments))
        paramCount := 0
        for i, s := range segments {
                if len(s) > 0 && s[0] == ':' {
                        paramIdx[i] = true
                        paramCount++
                }
        }

        // Capture per-route middlewares for the final handler closure.
        // We copy the slice so the caller can't mutate it after registration.
        var routeMWs []HandlerFunc
        if len(middlewares) > 0 {
                routeMWs = make([]HandlerFunc, len(middlewares))
                copy(routeMWs, middlewares)
        }

        // finalHandler is kept for backward compatibility with the public
        // Find method. OnTraffic uses findRoute (which returns the actual
        // handler + routeMWs) to build the chain in one allocation, avoiding
        // the double-build that finalHandler causes.
        finalHandler := func(ctx *Context) {
                ctx.middlewares = append(routeMWs, handler)
                ctx.index = -1
                ctx.Next()
        }

        r.routes = append(r.routes, &route{
                method:       method,
                pattern:      pattern,
                segments:     segments,
                paramIndex:   paramIdx,
                handler:      finalHandler,
                userHandler:  handler,
                routeMWs:     routeMWs,
                hasWildcard:  hasWildcard,
                wildcardName: wildcardName,
                paramCount:   paramCount,
        })
}

// Find matches the incoming request to a registered route.
//
// Performance decisions:
//   - Path splitting uses a manual scanner instead of strings.Split so we can
//     bail out early (wrong segment count) with zero allocations on a miss.
//   - The params map is only allocated when the route actually has :param
//     segments, and is pre-sized to paramCount.
//   - paramIndex[] is a pre-computed bool slice so we avoid strings.HasPrefix
//     inside the hot matching loop.
func (r *Router) Find(req *HTTPRequest) (HandlerFunc, []HandlerFunc, map[string]string) {
        // Split the request path into segments without allocating a []string.
        // We store them in a small stack-allocated array for the common case.
        var segBuf [16]string
        reqSegments := segBuf[:0]

        path := req.Path
        // Trim leading slash
        if len(path) > 0 && path[0] == '/' {
                path = path[1:]
        }
        // Trim trailing slash
        if len(path) > 0 && path[len(path)-1] == '/' {
                path = path[:len(path)-1]
        }

        if path != "" {
                start := 0
                for i := 0; i <= len(path); i++ {
                        if i == len(path) || path[i] == '/' {
                                seg := path[start:i]
                                if len(reqSegments) < len(segBuf) {
                                        reqSegments = reqSegments[:len(reqSegments)+1]
                                        reqSegments[len(reqSegments)-1] = seg
                                } else {
                                        // Overflow: fall back to heap allocation (paths with >16 segments)
                                        reqSegments = append(reqSegments, seg)
                                }
                                start = i + 1
                        }
                }
        }

        nReq := len(reqSegments)

        for _, rt := range r.routes {
                if rt.method != req.Method {
                        continue
                }

                if rt.hasWildcard {
                        if nReq < len(rt.segments) {
                                continue
                        }
                        match := true
                        for i, rseg := range rt.segments {
                                if rt.paramIndex[i] {
                                        // param: always matches, captured below
                                } else if rseg != reqSegments[i] {
                                        match = false
                                        break
                                }
                        }
                        if !match {
                                continue
                        }
                        var params map[string]string
                        if rt.paramCount > 0 || rt.wildcardName != "" {
                                params = acquireParams()
                                for i, rseg := range rt.segments {
                                        if rt.paramIndex[i] {
                                                params[rseg[1:]] = reqSegments[i]
                                        }
                                }
                                params[rt.wildcardName] = strings.Join(reqSegments[len(rt.segments):], "/")
                        }
                        return rt.handler, r.middlewares, params
                }

                // Normal route: segment count must match exactly.
                if len(rt.segments) != nReq {
                        continue
                }

                match := true
                for i, rseg := range rt.segments {
                        if !rt.paramIndex[i] && rseg != reqSegments[i] {
                                match = false
                                break
                        }
                }
                if !match {
                        continue
                }

                // Only allocate the params map when there are actual params.
                var params map[string]string
                if rt.paramCount > 0 {
                        params = acquireParams()
                        for i, rseg := range rt.segments {
                                if rt.paramIndex[i] {
                                        params[rseg[1:]] = reqSegments[i]
                                }
                        }
                }

                return rt.handler, r.middlewares, params
        }

        // Auto serve index.html for "/"
        if r.autoServeRoot && nReq == 0 && req.Method == GET {
                indexPath := filepath.Join(r.staticDir, "index.html")
                if _, err := os.Stat(indexPath); err == nil {
                        return func(ctx *Context) {
                                data, err := os.ReadFile(indexPath)
                                if err != nil {
                                        ctx.WriteString("Error reading index.html")
                                        return
                                }
                                ctx.HTML(data)
                        }, r.middlewares, nil
                }
        }

        return nil, nil, nil
}

// findRoute is the internal version of Find used by OnTraffic. It returns
// the actual user handler (not the finalHandler closure) and the route-local
// middlewares separately, so OnTraffic can build the full middleware chain
// in a single allocation:
//
//      chain = [global_mw..., route_mw..., handler]
//
// This eliminates the double-build that the finalHandler closure caused
// (outer append in OnTraffic + inner append in finalHandler).
//
// The global middlewares (r.middlewares) are NOT returned here — OnTraffic
// accesses them via r.middlewares directly. This avoids an extra return
// value and keeps the signature minimal.
func (r *Router) findRoute(req *HTTPRequest) (handler HandlerFunc, routeMWs []HandlerFunc, params map[string]string) {
        // Split the request path into segments without allocating a []string.
        // We store them in a small stack-allocated array for the common case.
        var segBuf [16]string
        reqSegments := segBuf[:0]

        path := req.Path
        if len(path) > 0 && path[0] == '/' {
                path = path[1:]
        }
        if len(path) > 0 && path[len(path)-1] == '/' {
                path = path[:len(path)-1]
        }

        if path != "" {
                start := 0
                for i := 0; i <= len(path); i++ {
                        if i == len(path) || path[i] == '/' {
                                seg := path[start:i]
                                if len(reqSegments) < len(segBuf) {
                                        reqSegments = reqSegments[:len(reqSegments)+1]
                                        reqSegments[len(reqSegments)-1] = seg
                                } else {
                                        reqSegments = append(reqSegments, seg)
                                }
                                start = i + 1
                        }
                }
        }

        nReq := len(reqSegments)

        for _, rt := range r.routes {
                if rt.method != req.Method {
                        continue
                }

                if rt.hasWildcard {
                        if nReq < len(rt.segments) {
                                continue
                        }
                        match := true
                        for i, rseg := range rt.segments {
                                if rt.paramIndex[i] {
                                        // param: always matches, captured below
                                } else if rseg != reqSegments[i] {
                                        match = false
                                        break
                                }
                        }
                        if !match {
                                continue
                        }
                        if rt.paramCount > 0 || rt.wildcardName != "" {
                                params = acquireParams()
                                for i, rseg := range rt.segments {
                                        if rt.paramIndex[i] {
                                                params[rseg[1:]] = reqSegments[i]
                                        }
                                }
                                params[rt.wildcardName] = strings.Join(reqSegments[len(rt.segments):], "/")
                        }
                        return rt.userHandler, rt.routeMWs, params
                }

                // Normal route: segment count must match exactly.
                if len(rt.segments) != nReq {
                        continue
                }

                match := true
                for i, rseg := range rt.segments {
                        if !rt.paramIndex[i] && rseg != reqSegments[i] {
                                match = false
                                break
                        }
                }
                if !match {
                        continue
                }

                var params map[string]string
                if rt.paramCount > 0 {
                        params = acquireParams()
                        for i, rseg := range rt.segments {
                                if rt.paramIndex[i] {
                                        params[rseg[1:]] = reqSegments[i]
                                }
                        }
                }

                return rt.userHandler, rt.routeMWs, params
        }

        // Auto serve index.html for "/"
        if r.autoServeRoot && nReq == 0 && req.Method == GET {
                indexPath := filepath.Join(r.staticDir, "index.html")
                if _, err := os.Stat(indexPath); err == nil {
                        h := func(ctx *Context) {
                                data, err := os.ReadFile(indexPath)
                                if err != nil {
                                        ctx.WriteString("Error reading index.html")
                                        return
                                }
                                ctx.HTML(data)
                        }
                        return h, nil, nil
                }
        }

        return nil, nil, nil
}

// Middlewares returns the router's global middleware slice. This is used
// by OnTraffic to build the full middleware chain in a single allocation.
// The returned slice is NOT a copy — callers must not mutate it.
func (r *Router) Middlewares() []HandlerFunc {
        return r.middlewares
}
