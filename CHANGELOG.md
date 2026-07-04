# Breeze — Changelog

All changes made to the Breeze framework.

---

## Bug Fixes

### 1. `types.go` — HTTP Method Typo

**Bug:** `OPTION Method = "OPTION"` — missing the trailing `S`.
RFC 9110 defines the method as `OPTIONS` (7 characters).

**Impact:** All CORS preflight requests (`OPTIONS /path`) failed to match
the constant, causing 404s on every cross-origin browser request.

**Fix:** `OPTIONS Method = "OPTIONS"`

---

### 2. `request.go` — `internMethod` Never Matched OPTIONS

**Bug:** `internMethod` had a case-6 branch checking for `"OPTION"` (6 bytes),
which is not a real HTTP method. Real OPTIONS requests are 7 bytes
(`"OPTIONS"`), so they fell through to `Method(string(b))` — an allocation.
Worse, the returned `Method("OPTIONS")` never matched the `OPTION` constant.

**Fix:** Removed the 6-byte branch. Added a case-7 branch using the same
zero-alloc byte-comparison pattern:

```go
case 7:
    if b[0] == 'O' && b[1] == 'P' && b[2] == 'T' &&
       b[3] == 'I' && b[4] == 'O' && b[5] == 'N' && b[6] == 'S' {
        return OPTIONS
    }
```

---

### 3. `websocket_engine.go` — Use-After-Put in Close Frame

**Bug:** In the `wsOpClose` handler:

```go
code, reason := parseClosePayload(frame.payload)
wsFramePool.Put(frame)                                    // returned to pool
echo := buildWSFrame(wsOpClose, frame.payload)            // reads stale data!
```

After `wsFramePool.Put(frame)`, another goroutine calling `parseWSFrame`
could grab the same `*wsFrame` and overwrite `frame.payload`. The subsequent
`buildWSFrame` call would read corrupted data — a use-after-free in pooled
memory.

**Fix:** Reorder — use `frame.payload` first, then return to pool:

```go
code, reason := parseClosePayload(frame.payload)
echo := buildWSFrame(wsOpClose, frame.payload)
wsFramePool.Put(frame)
```

---

### 3b. `websocket.go` — RFC 6455 Control Frame Validation

**Bug:** `parseWSFrame` did not enforce RFC 6455 §5.5 requirements for
control frames (Close, Ping, Pong):

1. Control frames MUST have a payload ≤ 125 bytes (no extended length
   encoding allowed).
2. Control frames MUST NOT be fragmented (FIN must be 1).

A malicious client could send an oversized or fragmented control frame,
which the parser would accept — potentially causing excessive memory
allocation or confusing the defragmentation logic.

**Fix:** Added validation early in `parseWSFrame`, after the opcode and
initial payload length are parsed:

```go
isControl := opcode >= wsOpClose
if isControl {
    if payLen > wsMaxControlPayload {
        return nil, -1 // control frame payload exceeds 125 bytes
    }
    if !fin {
        return nil, -1 // control frames must not be fragmented
    }
}
```

Also added a defensive invariant check using `wsMaxFrameHeader` (14 bytes =
2 + 8 + 4) to validate the parsed header size never exceeds the maximum:

```go
if offset > wsMaxFrameHeader {
    return nil, -1
}
```

This also silences the `unusedfunc` warnings for `wsMaxControlPayload` and
`wsMaxFrameHeader` — both constants are now referenced in the parser.

---

### 4. `context.go` — Added Typed Store (Set/Get/MustGet)

**Why:** Needed for the JWT fix (#8 below). The existing `params` field is
`map[string]string`, which can't hold structured data like `jwt.MapClaims`.

**Added:**
```go
type Context struct {
    // ... existing fields ...
    store map[string]any  // lazy-initialized, nil until first Set
}

func (ctx *Context) Set(key string, val any)
func (ctx *Context) Get(key string) (any, bool)
func (ctx *Context) MustGet(key string) any
```

**Performance:** The `store` field is `nil` until the first `Set` call —
zero allocation for requests that don't use it. Same pattern as Gin/Echo/Fiber.

---

### 5. `middlewares/compression.go` — Pre-Next Ordering Bug

**Bug:** The middleware checked `ctx.Res` **before** calling `ctx.Next()`.
At that point `ctx.Res` is always `nil` (handler hasn't run), so the
middleware short-circuited and **compression never ran** — the entire
feature was dead code.

**Fix:** Call `ctx.Next()` first, then post-process the response.

**Additional improvements:**
- Early-return on empty `Accept-Encoding`
- Early-return if `Content-Encoding` is already set (prevent double-compress)
- Added `Vary: Accept-Encoding` header for proper cache behavior
- Properly check `Close()` return value

---

### 6. `middlewares/cache.go` — ETag Ordering + Query Key Collision

**Bug 1 (ordering):** Same as compression — checked `ctx.Res` before
`ctx.Next()`, so ETag generation never ran.

**Bug 2 (key collision):** The cache key was `ctx.Req.Path` only, so
`/api/users?page=1` and `/api/users?page=2` shared the same ETag entry,
causing false 304s.

**Fix:**
- Call `ctx.Next()` first, then compute ETag from the fresh response body
- Include query string in the cache key (only allocates when a query exists)
- Use `RLock` for the If-None-Match pre-check (concurrent 304 checks)
- Pre-check: skip the handler entirely on a known ETag match

---

### 7. `middlewares/cors.go` — Missing Abort() on OPTIONS

**Bug:** On OPTIONS preflight, the middleware called `return` without
`ctx.Abort()`, leaving `ctx.index` at its current position. If any code
later called `ctx.Next()` on the same context, the chain would resume past
the CORS short-circuit.

**Fix:**
```go
if ctx.Req.Method == breeze.OPTIONS {
    ctx.Status(204)
    ctx.Abort()
    return
}
```

---

### 8. `middlewares/rate_limiter.go` — Lock Held Across Next()

**Bug (critical performance):** The middleware held `mu.Lock()` with
`defer rl.mu.Unlock()` across `ctx.Next()`:

```go
rl.mu.Lock()
defer rl.mu.Unlock()
// ... counter update ...
ctx.Next()  // ← handler runs under lock!
```

This serialized **every request** through a single mutex, completely
defeating the WorkerPool's concurrency. A 16-core server would process
requests one at a time.

**Fix:**
- Do the map lookup + counter update under the lock
- Release the lock before `ctx.Next()`
- Pre-compute the 429 message at construction time (avoid `fmt.Sprintf`
  on every rejected request)

**Impact:** Before — lock held for entire handler duration (ms to seconds).
After — lock held for map ops only (microseconds).

---

### 9. `middlewares/jwt.go` — Claims Stored as Unparseable String

**Bug:** The middleware stored JWT claims via:

```go
ctx.SetParam(opts.UserContextKey, fmt.Sprintf("%v", claims))
```

`fmt.Sprintf("%v", map[string]any{...})` produces a Go-specific
representation like `map[exp:1234 role:admin user_id:42]`. Downstream
handlers could not parse this back into structured data.

**Fix:** Use the new typed store:

```go
ctx.Set(opts.UserContextKey, claims)
```

Handlers retrieve claims with a type assertion:

```go
claims, ok := ctx.Get("user").(jwt.MapClaims)
```

---

## File Inventory

This package contains the **complete** framework — every file is either the
original unchanged source, or a bug-fix version. Replace your entire `breeze/`
directory with these files to avoid any mixing of versions.

```
breeze-final/
├── CHANGELOG.md
├── breeze.go               ← ORIGINAL (unchanged)
├── types.go                ← BUG FIX: OPTIONS method
├── request.go              ← BUG FIX: internMethod case 7
├── context.go              ← BUG FIX: typed Set/Get store
├── response.go             ← ORIGINAL (unchanged)
├── router.go               ← ORIGINAL (unchanged)
├── router_static.go        ← ORIGINAL (unchanged)
├── workerpool.go           ← ORIGINAL (unchanged)
├── websocket.go            ← BUG FIX: RFC 6455 control frame validation
├── websocket_engine.go     ← BUG FIX: use-after-Put
├── file.go                 ← ORIGINAL (unchanged)
├── template.go             ← ORIGINAL (unchanged)
└── middlewares/
    ├── compression.go      ← BUG FIX: post-Next ordering
    ├── cache.go            ← BUG FIX: post-Next + query key + RLock
    ├── cors.go             ← BUG FIX: Abort() on OPTIONS
    ├── rate_limiter.go     ← BUG FIX: lock released before Next()
    └── jwt.go              ← BUG FIX: typed claims storage
```

**Bug-fixed files (10):** `types.go`, `request.go`, `context.go`,
`websocket.go`, `websocket_engine.go`, and all 5 middlewares.

**Original files (7):** `breeze.go`, `response.go`, `router.go`,
`router_static.go`, `workerpool.go`, `file.go`, `template.go`.
