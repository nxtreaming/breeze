package breeze

import (
	"strconv"
)

// statusTexts maps common status codes without allocating a map per call.
var statusTexts = [600]string{}

func init() {
	statusTexts[200] = "OK"
	statusTexts[201] = "Created"
	statusTexts[204] = "No Content"
	statusTexts[301] = "Moved Permanently"
	statusTexts[302] = "Found"
	statusTexts[304] = "Not Modified"
	statusTexts[400] = "Bad Request"
	statusTexts[401] = "Unauthorized"
	statusTexts[403] = "Forbidden"
	statusTexts[404] = "Not Found"
	statusTexts[405] = "Method Not Allowed"
	statusTexts[408] = "Request Timeout"
	statusTexts[409] = "Conflict"
	statusTexts[422] = "Unprocessable Entity"
	statusTexts[429] = "Too Many Requests"
	statusTexts[500] = "Internal Server Error"
	statusTexts[502] = "Bad Gateway"
	statusTexts[503] = "Service Unavailable"
}

// Bytes serializes the HTTPResponse to raw HTTP/1.1 bytes.
//
// Performance decisions:
//   - No fmt.Sprintf: we use strconv.AppendInt for the status code and
//     content-length, which writes directly into the buffer.
//   - We pre-size the buffer to avoid growth reallocations for typical responses.
//   - The status text lookup is a direct array index — O(1), no map hash.
func (r *HTTPResponse) Bytes() []byte {
	statusText := ""
	if r.Status > 0 && r.Status < len(statusTexts) {
		statusText = statusTexts[r.Status]
	}
	if statusText == "" {
		statusText = "OK"
	}

	// Estimate capacity: status line (~20) + headers (~40 each) + body.
	est := 32 + len(statusText) + len(r.Headers)*48 + len(r.Body)
	buf := make([]byte, 0, est)

	buf = append(buf, "HTTP/1.1 "...)
	buf = strconv.AppendInt(buf, int64(r.Status), 10)
	buf = append(buf, ' ')
	buf = append(buf, statusText...)
	buf = append(buf, "\r\n"...)

	for k, v := range r.Headers {
		buf = append(buf, k...)
		buf = append(buf, ": "...)
		buf = append(buf, v...)
		buf = append(buf, "\r\n"...)
	}

	buf = append(buf, "Content-Length: "...)
	buf = strconv.AppendInt(buf, int64(len(r.Body)), 10)
	buf = append(buf, "\r\n\r\n"...)
	buf = append(buf, r.Body...)

	return buf
}
