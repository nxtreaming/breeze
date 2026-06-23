package breeze

import (
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unsafe"
)

// crlfcrlf is the header terminator we scan for once per request.
var crlfcrlf = []byte("\r\n\r\n")

// ParseHTTPRequest parses raw bytes into an HTTPRequest.
//
// Performance decisions:
//   - b2s converts []byte → string with zero allocation (safe here because
//     the string is only used within this function or stored in a map whose
//     backing bytes outlive the function).
//   - We scan the request-line manually instead of bytes.Split to avoid
//     allocating a [][]byte for the header lines.
//   - Header keys are lowercased in-place on a stack buffer up to 64 bytes.
func ParseHTTPRequest(data []byte) (*HTTPRequest, int, error) {
	headerEnd := bytes.Index(data, crlfcrlf)
	if headerEnd < 0 {
		return nil, 0, nil // incomplete
	}

	header := data[:headerEnd]

	// ── Parse request line ─────────────────────────────────────────────────
	lineEnd := bytes.IndexByte(header, '\r')
	if lineEnd < 0 {
		lineEnd = len(header)
	}
	requestLine := header[:lineEnd]

	s1 := bytes.IndexByte(requestLine, ' ')
	if s1 < 0 {
		return nil, 0, fmt.Errorf("malformed request line")
	}
	s2 := bytes.IndexByte(requestLine[s1+1:], ' ')
	if s2 < 0 {
		s2 = len(requestLine) - s1 - 1
	}

	methodBytes := requestLine[:s1]
	rawPath := requestLine[s1+1 : s1+1+s2]

	// Parse path + query without net/url overhead for the common case
	// (no fragment, no userinfo, just path?query).
	path, query := splitPathQuery(rawPath)

	req := &HTTPRequest{
		Method: Method(b2s(methodBytes)),
		Path:   b2s(path),
		Header: make(map[string]string, 8), // pre-size for typical header count
	}

	// Parse query string only when present
	if len(query) > 0 {
		q, err := url.ParseQuery(b2s(query))
		if err == nil {
			req.Query = q
		}
	}

	// ── Parse headers (scan without bytes.Split) ───────────────────────────
	pos := lineEnd + 2 // skip past request line's \r\n
	for pos < len(header) {
		lineEnd := bytes.IndexByte(header[pos:], '\r')
		if lineEnd < 0 {
			lineEnd = len(header) - pos
		}
		line := header[pos : pos+lineEnd]
		pos += lineEnd + 2

		if len(line) == 0 {
			continue
		}
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}

		key := toLowerASCII(line[:colon])
		val := strings.TrimSpace(b2s(line[colon+1:]))
		req.Header[key] = val
	}

	// ── Body ───────────────────────────────────────────────────────────────
	clStr := req.Header["content-length"]
	if clStr != "" {
		cl, err := strconv.Atoi(clStr)
		if err != nil || cl < 0 {
			return nil, 0, fmt.Errorf("invalid content-length")
		}
		total := headerEnd + 4 + cl
		if len(data) < total {
			return nil, 0, nil // wait for more data
		}
		req.Body = data[headerEnd+4 : total]
		return req, total, nil
	}

	return req, headerEnd + 4, nil
}

// splitPathQuery splits rawPath at the first '?' without allocating.
func splitPathQuery(raw []byte) (path, query []byte) {
	i := bytes.IndexByte(raw, '?')
	if i < 0 {
		return raw, nil
	}
	return raw[:i], raw[i+1:]
}

// toLowerASCII converts a byte slice to a lowercase ASCII string without
// allocating in the common case where the key is already lowercase.
func toLowerASCII(b []byte) string {
	needsLower := false
	for _, c := range b {
		if c >= 'A' && c <= 'Z' {
			needsLower = true
			break
		}
	}
	if !needsLower {
		return b2s(b)
	}
	buf := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			buf[i] = c + 32
		} else {
			buf[i] = c
		}
	}
	return string(buf)
}

// b2s converts a byte slice to a string without allocation.
// SAFE only when the string will not outlive the byte slice it points into,
// OR when the bytes are stored in a long-lived map (the map value copies the
// string header, but the backing bytes come from the request buffer which is
// kept alive in s.Bufs[fd] for the duration of the request).
func b2s(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
