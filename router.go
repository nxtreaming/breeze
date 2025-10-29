package breeze

import "strings"

type HandlerFunc func(*Context)

type route struct {
	method      Method
	pattern     string
	segments    []string
	handler     HandlerFunc
	middlewares []HandlerFunc
}

type Router struct {
	routes      []*route
	middlewares []HandlerFunc
}

func NewRouter() *Router {
	return &Router{}
}

func (r *Router) Use(mw ...HandlerFunc) {
	r.middlewares = append(r.middlewares, mw...)
}

func (r *Router) Handle(method Method, pattern string, handler HandlerFunc) {
	trimmed := strings.Trim(pattern, "/")
	var segments []string
	if trimmed == "" {
		segments = []string{} // root route
	} else {
		segments = strings.Split(trimmed, "/")
	}

	r.routes = append(r.routes, &route{
		method:   method,
		pattern:  pattern,
		segments: segments,
		handler:  handler,
	})
}

func (r *Router) Find(req *HTTPRequest) (HandlerFunc, []HandlerFunc, map[string]string) {
	path := strings.Trim(req.Path, "/")
	reqSegments := []string{}
	if path != "" {
		reqSegments = strings.Split(path, "/")
	}

	for _, rt := range r.routes {
		if rt.method != req.Method {
			continue
		}
		if len(rt.segments) != len(reqSegments) {
			continue
		}

		params := map[string]string{}
		match := true
		for i := range rt.segments {
			if strings.HasPrefix(rt.segments[i], ":") {
				params[rt.segments[i][1:]] = reqSegments[i]
			} else if rt.segments[i] != reqSegments[i] {
				match = false
				break
			}
		}
		if match {
			return rt.handler, rt.middlewares, params
		}
	}
	return nil, nil, nil
}
