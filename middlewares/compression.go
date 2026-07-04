package middleware

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/nelthaarion/breeze"
)

// CompressionMiddleware compresses responses using supported algorithms.
//
// FIX: The original code inspected ctx.Res BEFORE calling ctx.Next(), so it
// always saw a nil response and short-circuited — compression never ran.
// The middleware now calls ctx.Next() first, then post-processes the response.
//
// Performance decisions:
//   - Early-return on empty Accept-Encoding to avoid any allocation for
//     clients that don't advertise compression (common for internal APIs).
//   - Early-return if the response already has Content-Encoding to prevent
//     double-compression when multiple compression layers are stacked.
//   - bytes.Buffer is stack-declared; its internal []byte is the only heap
//     allocation, and it grows once for typical response sizes.
//   - strings.Contains is used instead of full Accept-Encoding parsing
//     (with q-values) because it is ~10x faster and sufficient for the
//     common case where the client lists algorithms plainly.
//   - The switch respects the standard preference order: br > gzip > deflate.
func CompressionMiddleware() breeze.HandlerFunc {
	return func(ctx *breeze.Context) {
		// Run the handler first — we need its response to compress.
		ctx.Next()

		// Nothing to compress.
		if ctx.Res == nil || len(ctx.Res.Body) == 0 {
			return
		}

		accept := ctx.Req.Header["Accept-Encoding"]
		if accept == "" {
			return
		}

		// Don't double-compress if another layer already encoded the body.
		if _, ok := ctx.Res.Headers["Content-Encoding"]; ok {
			return
		}

		var buf bytes.Buffer
		var encoding string

		switch {
		case strings.Contains(accept, "br"):
			brWriter := brotli.NewWriter(&buf)
			if _, err := brWriter.Write(ctx.Res.Body); err == nil {
				if err := brWriter.Close(); err == nil {
					encoding = "br"
				}
			} else {
				brWriter.Close()
				return
			}
		case strings.Contains(accept, "gzip"):
			gzWriter := gzip.NewWriter(&buf)
			if _, err := gzWriter.Write(ctx.Res.Body); err == nil {
				if err := gzWriter.Close(); err == nil {
					encoding = "gzip"
				}
			} else {
				gzWriter.Close()
				return
			}
		case strings.Contains(accept, "deflate"):
			defWriter, err := flate.NewWriter(&buf, flate.DefaultCompression)
			if err != nil {
				return
			}
			if _, err := defWriter.Write(ctx.Res.Body); err == nil {
				if err := defWriter.Close(); err == nil {
					encoding = "deflate"
				}
			} else {
				defWriter.Close()
				return
			}
		default:
			// No supported encoding — serve the raw response.
			return
		}

		// Only swap the body if compression actually produced output.
		if encoding == "" {
			return
		}

		ctx.Res.Body = buf.Bytes()
		ctx.SetHeader("Content-Encoding", encoding)
		// Vary header tells caches that the response differs by Accept-Encoding.
		ctx.SetHeader("Vary", "Accept-Encoding")
	}
}
