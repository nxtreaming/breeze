package breeze

import (
        "fmt"
        "runtime/debug"
        "sync"

        "github.com/panjf2000/gnet/v2"
)

type Breeze struct {
        *gnet.BuiltinEventEngine
        Router *Router
        bufs   sync.Map // fd(int) → []byte ; per-connection HTTP reassembly buffer
        Pool   *WorkerPool

        // WebSocket support — initialised lazily by WebSocket().
        wsHubFields
}

// compactThreshold: compact the leftover slice when the unused capacity
// exceeds this many bytes, to avoid keeping large receive buffers alive.
const compactThreshold = 512

// New creates a Breeze server with the given router and worker pool.
//
// The pool's OverflowPolicy MUST be OverflowSpawn (or OverflowReject) —
// NOT OverflowBlock — because OnTraffic calls pool.Submit from the gnet
// event-loop goroutine, and a blocking Submit would stall ALL connections
// on that reactor.
//
// Use breeze.NewEventLoopWorkerPool(n) to create a suitable pool. The
// deprecated breeze.NewWorkerPool(n) also works (it uses OverflowSpawn
// for backward compatibility).
func New(router *Router, pool *WorkerPool) *Breeze {
        return &Breeze{
                BuiltinEventEngine: &gnet.BuiltinEventEngine{},
                Router:             router,
                Pool:               pool,
        }
}

// OnTraffic is called by gnet for every incoming data event.
//
// Routing strategy (zero-overhead fast path):
//  1. Check wsConns sync.Map — O(1) lock-free Load.
//     If the fd is a promoted WebSocket connection, hand off to
//     handleWSTraffic immediately (no HTTP parsing whatsoever).
//  2. Otherwise run the normal HTTP parse → route → dispatch pipeline.
//
// This means WebSocket connections have no HTTP overhead after the upgrade,
// and HTTP connections have no WebSocket overhead (a single sync.Map miss).
func (s *Breeze) OnTraffic(c gnet.Conn) gnet.Action {
        fd := c.Fd()

        // ── WebSocket fast path ──────────────────────────────────────────────
        if state, ok := s.isWSConn(fd); ok {
                return s.handleWSTraffic(c, state)
        }

        // ── HTTP path ────────────────────────────────────────────────────────
        data, _ := c.Next(-1)
        if len(data) == 0 {
                return gnet.None
        }

        // Always copy incoming data into a Go-owned buffer.
        //
        // gnet's data slice is a view into gnet's internal ring buffer.
        // gnet is free to overwrite that memory as soon as OnTraffic returns,
        // but req.Body may still reference it from a worker goroutine.
        // By appending into an existing Go slice (or a fresh one when existing
        // is nil), we ensure buf is always GC-managed: req.Body = buf[x:y]
        // keeps the backing array alive for exactly as long as the request lives.
        var existing []byte
        if v, ok := s.bufs.Load(fd); ok {
                existing = v.([]byte)
        }
        buf := append(existing, data...)

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

                handler, routeMWs, params := s.Router.findRoute(req)

                if handler == nil {
                        c.AsyncWrite([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nNot Found"), nil)
                } else {
                        // Build the full middleware chain in ONE allocation:
                        //   [global_mw..., route_mw..., handler]
                        globalMWs := s.Router.middlewares
                        chain := make([]HandlerFunc, 0, len(globalMWs)+len(routeMWs)+1)
                        chain = append(chain, globalMWs...)
                        chain = append(chain, routeMWs...)
                        chain = append(chain, handler)

                        // Acquire Context from the pool (Phase 1.3.4).
                        // Released in the exec closure's deferred cleanup
                        // after the response is written.
                        ctx := acquireContext()
                        ctx.Conn = c
                        ctx.Req = req
                        ctx.params = params
                        ctx.middlewares = chain
                        ctx.index = -1

                        exec := func() {
                                // Release defer is registered FIRST so it
                                // runs LAST (after the recover defer). This
                                // ensures the response is fully written
                                // before the Context is returned to the pool.
                                defer releaseContext(ctx)
                                // Recover from panics in handlers so a buggy
                                // handler does not crash the worker goroutine.
                                defer func() {
                                        if r := recover(); r != nil {
                                                fmt.Printf("[Breeze][PANIC] %v\n%s\n", r, debug.Stack())
                                                if ctx.Res == nil {
                                                        ctx.Res = &HTTPResponse{
                                                                Status:  500,
                                                                Headers: map[string]string{"Content-Type": "text/plain"},
                                                                Body:    []byte("Internal Server Error"),
                                                        }
                                                }
                                                c.AsyncWrite(ctx.Res.Bytes(), nil)
                                        }
                                }()
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
                if cap(buf)-len(buf) > compactThreshold {
                        compact := make([]byte, len(buf))
                        copy(compact, buf)
                        buf = compact
                }
                s.bufs.Store(fd, buf)
        }

        return gnet.None
}

// OnClose cleans up all per-connection state when a connection closes.
// For WebSocket connections that closed unexpectedly (no Close frame received),
// we still call OnClose so the application can clean up its own state.
func (s *Breeze) OnClose(c gnet.Conn, err error) gnet.Action {
        fd := c.Fd()
        s.bufs.Delete(fd)

        // WebSocket cleanup on unexpected close (e.g. TCP RST, network drop).
        if state, ok := s.isWSConn(fd); ok {
                s.cleanupWS(fd, state.wc, state.handler, 1006, "abnormal closure")
        }

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
