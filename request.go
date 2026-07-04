package breeze

import (
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"unsafe"
)

// crlfcrlf is the header terminator we scan for once per request.
var crlfcrlf = []byte("\r\n\r\n")

// ParseHTTPRequest parses raw bytes into an HTTPRequest.
//
// # Correctness: owned header copy, zero-copy body
//
// String fields (Method, Path, header keys/values) are unsafe views into a
// byte slice. With a worker pool the handler goroutine and the gnet event loop
// run concurrently on the same connection buffer, so those views must NOT point
// into the caller's buffer — a subsequent OnTraffic call can compact or
// reallocate it while the handler is still reading.
//
// Solution: copy only the header block (data[:headerEnd]) into req.owned.
// All string views are b2s slices of req.owned, which is kept alive by the
// GC for exactly as long as *HTTPRequest is reachable.
//
// req.Body is kept as a zero-copy slice of the caller's buffer. This is safe
// because the body occupies data[headerEnd+4 : consumed], and the leftover
// bytes that OnTraffic retains start at data[consumed:]. They never overlap,
// so OnTraffic never mutates or discards the body bytes while they are in use.
//
// # Single-pass header parsing
//
// The previous version did two full scans of the header block: one pre-scan
// (indexHeaderValue) to find Content-Length so it could size the copy, and a
// second scan to build req.Header. This version does one scan that tracks
// Content-Length inline while building the map, then sets req.Body after.
//
// # Other performance decisions
//   - Manual byte scanner replaces bytes.Split → no [][]byte alloc.
//   - splitPathQuery uses bytes.IndexByte → no url.Parse overhead.
//   - toLowerASCII avoids allocation when the key is already lowercase.
//   - url.ParseQuery copies internally — b2s(query) is transient and safe.
//   - internMethod returns a package-level constant for the seven known methods.
func ParseHTTPRequest(data []byte) (*HTTPRequest, int, error) {
	// ── Find header boundary ───────────────────────────────────────────────
	headerEnd := bytes.Index(data, crlfcrlf)
	if headerEnd < 0 {
		return nil, 0, nil // incomplete — wait for more data
	}

	// ── Copy header block only ─────────────────────────────────────────────
	// Body bytes are zero-copy (see doc above); only headers need isolation.
	owned := make([]byte, headerEnd)
	copy(owned, data[:headerEnd])

	// All parsing below operates on owned.
	header := owned

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
	path, query := splitPathQuery(rawPath)

	req := &HTTPRequest{
		Method: internMethod(methodBytes),
		Path:   b2s(path),
		Header: make(map[string]string, 8),
		owned:  owned,
	}

	if len(query) > 0 {
		// url.ParseQuery copies all keys/values — b2s(query) is transient.
		q, err := url.ParseQuery(b2s(query))
		if err == nil {
			req.Query = q
		}
	}

	// ── Single-pass header scan ────────────────────────────────────────────
	// Builds req.Header and extracts Content-Length in one traversal.
	contentLength := -1
	pos := lineEnd + 2 // skip past \r\n of the request line

	for pos < len(header) {
		end := bytes.IndexByte(header[pos:], '\r')
		if end < 0 {
			end = len(header) - pos
		}
		line := header[pos : pos+end]
		pos += end + 2

		if len(line) == 0 {
			continue
		}
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}

		key := toLowerASCII(line[:colon])
		val := b2s(bytes.TrimSpace(line[colon+1:]))
		req.Header[key] = val

		// Capture Content-Length without a second scan.
		if contentLength == -1 && key == "content-length" {
			cl, err := strconv.Atoi(val)
			if err != nil || cl < 0 {
				return nil, 0, fmt.Errorf("invalid content-length")
			}
			contentLength = cl
		}
	}

	// ── Body (zero-copy) ───────────────────────────────────────────────────
	consumed := headerEnd + 4
	if contentLength > 0 {
		total := consumed + contentLength
		if len(data) < total {
			return nil, 0, nil // body not fully received yet
		}
		// Body slices the caller's buffer directly. Safe because:
		// - buf[consumed:] (the leftover) never overlaps buf[:consumed] (the body).
		// - OnTraffic only mutates/discards bytes at consumed and beyond.
		req.Body = data[consumed:total]
		consumed = total
	}

	return req, consumed, nil
}

// splitPathQuery splits rawPath at the first '?' without allocating.
func splitPathQuery(raw []byte) (path, query []byte) {
	i := bytes.IndexByte(raw, '?')
	if i < 0 {
		return raw, nil
	}
	return raw[:i], raw[i+1:]
}

// toLowerASCII converts b to a lowercase ASCII string.
// When no uppercase bytes are present it uses b2s — a zero-alloc view into
// the owned slice. When recasing is needed it allocates a fresh string.
func toLowerASCII(b []byte) string {
	for _, c := range b {
		if c >= 'A' && c <= 'Z' {
			buf := make([]byte, len(b))
			for i, ch := range b {
				if ch >= 'A' && ch <= 'Z' {
					buf[i] = ch + 32
				} else {
					buf[i] = ch
				}
			}
			return string(buf)
		}
	}
	return b2s(b)
}

// internMethod returns a package-level Method constant without allocation.
// Falls back to Method(string(b)) for unknown methods (not in hot path).
//
// FIX: Removed the 6-byte "OPTION" branch (not a real HTTP method) and added
// a 7-byte "OPTIONS" branch matching RFC 9110. The old code never matched a
// real OPTIONS preflight request, causing CORS preflight to 404.
func internMethod(b []byte) Method {
	switch len(b) {
	case 3:
		if b[0] == 'G' && b[1] == 'E' && b[2] == 'T' {
			return GET
		}
		if b[0] == 'P' && b[1] == 'U' && b[2] == 'T' {
			return PUT
		}
	case 4:
		if b[0] == 'P' && b[1] == 'O' && b[2] == 'S' && b[3] == 'T' {
			return POST
		}
	case 5:
		if b[0] == 'P' && b[1] == 'A' && b[2] == 'T' && b[3] == 'C' && b[4] == 'H' {
			return PATCH
		}
	case 6:
		if b[0] == 'D' && b[1] == 'E' && b[2] == 'L' && b[3] == 'E' && b[4] == 'T' && b[5] == 'E' {
			return DELETE
		}
	case 7:
		// FIX: OPTIONS is 7 bytes, not 6. The old code checked for the
		// non-existent 6-byte "OPTION" method and never matched real
		// CORS preflight requests.
		if b[0] == 'O' && b[1] == 'P' && b[2] == 'T' && b[3] == 'I' && b[4] == 'O' && b[5] == 'N' && b[6] == 'S' {
			return OPTIONS
		}
	}
	return Method(string(b))
}

// b2s converts a byte slice to a string without allocation.
//
// SAFETY CONTRACT: the returned string must not outlive b. Within this package
// b is always a subslice of req.owned, which the GC keeps alive as long as
// *HTTPRequest is reachable. Do not use b2s on slices that are not anchored
// to a live GC-traced object.
func b2s(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
