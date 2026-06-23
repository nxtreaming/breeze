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
	handler      HandlerFunc
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
				params = make(map[string]string, rt.paramCount+1)
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
			params = make(map[string]string, rt.paramCount)
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
