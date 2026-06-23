package breeze

import "net/url"

// Method defines the HTTP method type (GET, POST, etc.).
type Method string

const (
	GET    Method = "GET"
	PUT    Method = "PUT"
	PATCH  Method = "PATCH"
	POST   Method = "POST"
	DELETE Method = "DELETE"
	OPTION Method = "OPTION"
)

// HTTPRequest holds parsed HTTP request data.
type HTTPRequest struct {
	Method Method
	Path   string
	Query  url.Values
	Header map[string]string
	Body   []byte
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
