package breeze

import (
	"fmt"
	"sync"

	"github.com/panjf2000/gnet/v2"
)

// Breeze is the main server struct, embedding gnet's event engine.
//
// Performance decisions:
//   - s.mu + s.Bufs[fd] uses per-connection buffering. The single mutex
//     creates contention between gnet reactors when multicore=true.
//     We mitigate this with sync.Map so each fd's read/write is independent.
//   - buf reslicing (buf = buf[consumed:]) kept the full backing array alive,
//     leaking memory under pipelining. We now compact: when leftover bytes
//     are small we copy them to a fresh slice so the large receive buffer can
//     be GC'd.
//   - The exec closure captures ctx and c by value so the goroutine/worker
//     doesn't pin the loop variable across iterations.
type Breeze struct {
	*gnet.BuiltinEventEngine
	Router *Router
	bufs   sync.Map // fd(int) → []byte ; replaces map+Mutex
	Pool   *WorkerPool
}

// compactThreshold: if leftover bytes after consuming a request are less than
// this fraction of the buffer, compact into a fresh slice.
const compactThreshold = 512

func New(router *Router, pool *WorkerPool) *Breeze {
	return &Breeze{
		BuiltinEventEngine: &gnet.BuiltinEventEngine{},
		Router:             router,
		Pool:               pool,
	}
}

func (s *Breeze) OnTraffic(c gnet.Conn) gnet.Action {
	fd := c.Fd()
	data, _ := c.Next(-1)
	if len(data) == 0 {
		return gnet.None
	}

	// Load existing buffer for this connection (nil if first read).
	var buf []byte
	if v, ok := s.bufs.Load(fd); ok {
		existing := v.([]byte)
		buf = append(existing, data...)
	} else {
		// First read: avoid a copy when data already contains a full request.
		buf = data
	}

	for len(buf) > 0 {
		req, consumed, err := ParseHTTPRequest(buf)
		if err != nil {
			c.AsyncWrite([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 11\r\n\r\nBad Request"), nil)
			buf = nil
			break
		}
		if req == nil {
			break // incomplete — wait for more data
		}

		handler, middlewares, params := s.Router.Find(req)

		if handler == nil {
			c.AsyncWrite([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nNot Found"), nil)
		} else {
			// Capture loop variables explicitly so the closure is safe to
			// run concurrently (each iteration of the for-loop gets its own
			// req, params, handler).
			ctx := &Context{
				Conn:        c,
				Req:         req,
				params:      params,
				middlewares: append(middlewares, handler),
				index:       -1,
			}

			exec := func() {
				ctx.Next()
				if ctx.Res != nil {
					c.AsyncWrite(ctx.Res.Bytes(), nil)
				}
			}

			if s.Pool != nil {
				s.Pool.Submit(exec)
			} else {
				go exec()
			}
		}

		if consumed >= len(buf) {
			buf = nil
			break
		}
		buf = buf[consumed:]
	}

	// Store leftover bytes (partial next request).
	if len(buf) == 0 {
		s.bufs.Delete(fd)
	} else {
		// Compact: if the leftover is small relative to the backing array,
		// copy it so the large receive buffer can be GC'd.
		if cap(buf)-len(buf) > compactThreshold {
			compact := make([]byte, len(buf))
			copy(compact, buf)
			buf = compact
		}
		s.bufs.Store(fd, buf)
	}

	return gnet.None
}

// OnClose cleans up the per-connection buffer when a connection closes.
func (s *Breeze) OnClose(c gnet.Conn, err error) gnet.Action {
	s.bufs.Delete(c.Fd())
	return gnet.None
}

func (s *Breeze) Run(port int, multiCore bool) error {
	return gnet.Run(
		s,
		fmt.Sprintf("tcp://:%d", port),
		gnet.WithTCPNoDelay(gnet.TCPNoDelay),
		gnet.WithMulticore(multiCore),
		gnet.WithLoadBalancing(gnet.RoundRobin),
	)
}
