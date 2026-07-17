package breeze

import "net/url"

// Method defines the HTTP method type.
type Method string

const (
        GET     Method = "GET"
        PUT     Method = "PUT"
        PATCH   Method = "PATCH"
        POST    Method = "POST"
        DELETE  Method = "DELETE"
        OPTIONS Method = "OPTIONS" // FIX: was "OPTION" — RFC 9110 defines "OPTIONS"
)

// HTTPRequest holds a fully parsed HTTP request.
//
// Memory layout — two independent allocations, both Go-managed:
//
//  1. req.owned  — a copy of the raw header bytes (data[:headerEnd]).
//     req.Path and all req.Header keys/values are unsafe string views
//     (b2s slices) into this allocation. The GC keeps owned alive as
//     long as any of those strings are reachable, even after *HTTPRequest
//     itself is collected. Handlers may safely stash header strings in
//     globals or caches without copying them.
//
//  2. OnTraffic's reassembly buf — a Go-owned []byte built by appending
//     gnet's read buffer into an existing slice. req.Body is a zero-copy
//     subslice of this allocation. The GC keeps the backing array alive
//     as long as req.Body is reachable.
//
// Neither allocation is shared with gnet's internal ring buffer or with
// the per-connection leftover slice stored in s.bufs. OnTraffic is free
// to compact or discard s.bufs at any time without affecting an
// in-flight handler's view of req.Path, req.Header, or req.Body.
//
// req.Method is either a package-level constant (no allocation) or a
// freshly copied string for unknown methods — it does not point into owned.
type HTTPRequest struct {
        Method Method
        Path   string
        Query  url.Values
        Header map[string]string
        Body   []byte
        // owned holds the header bytes that req.Path and req.Header strings
        // point into. Unexported so callers cannot mutate it; its presence
        // here ensures the GC can trace the pointer chain from any escaped
        // header string back to this backing array.
        owned []byte
}

// HTTPResponse represents an HTTP response.
type HTTPResponse struct {
        Status  int
        Headers map[string]string
        Body    []byte
        // headersShared is true when Headers points to one of the package-level
        // shared maps (hdrsJSON / hdrsText / hdrsHTML). SetHeader must copy-on-write
        // before mutating. Go does not allow map == map comparisons, so we use this
        // flag as the sentinel instead.
        headersShared bool
}
