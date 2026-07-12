<div align="center">

# Breeze

**A ridiculously fast, event-driven Go web framework built for maximum
throughput, minimal allocations, native WebSockets, and
production-ready APIs.**

[![Documentation](https://img.shields.io/badge/Documentation-Latest-blue?style=for-the-badge)](https://nelthaarion.github.io/breeze)
[![GitHub](https://img.shields.io/badge/GitHub-nelthaarion%2Fbreeze-181717?style=for-the-badge&logo=github)](https://github.com/nelthaarion/breeze)
[![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](./LICENSE)

</div>

---

Breeze is a modern, high-performance Go web framework engineered for
developers who demand speed without sacrificing developer experience.
Built on an event-driven architecture, Breeze minimizes allocations,
optimizes every request path, and provides first-class support for REST
APIs, WebSockets, middleware, and automatic OpenAPI documentation.

Whether you're building microservices, real-time applications, or
high-throughput APIs, Breeze is designed to handle millions of requests
efficiently while keeping your code clean and maintainable.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Docker](#docker)
- [CLI — Scaffolding & Code Generation](#-cli--scaffolding--code-generation)
- [Features](#features)
  - [Built for Extreme Performance](#-built-for-extreme-performance)
  - [High-Performance Routing](#-high-performance-routing)
  - [Native WebSocket Engine](#-native-websocket-engine)
    - [Built-in OpenAPI / Scalar](#-built-in-openapi--scalar)
  - [Production Middleware](#-production-middleware)
  - [Built-in Developer Dashboard](#-built-in-developer-dashboard)
  - [Developer Experience](#-developer-experience)
  - [Performance Optimizations](#-performance-optimizations)
- [Support the Project](#-support-the-project)
- [Contributing](#-contributing)
- [Security Scanning](#-security-scanning)
- [License](#license)

## Installation

Requires **Go 1.24.3** or later.

```bash
go get github.com/nelthaarion/breeze
```

The module pulls in **gnet v2** for the event loop, **go-json** for fast
JSON marshaling, **brotli** for compression, and **golang-jwt/jwt** for
authentication.

📖 **Full documentation:** <https://nelthaarion.github.io/breeze>

## Quick Start

A complete working server in under 20 lines:

```go
package main

import (
    "runtime"

    "github.com/nelthaarion/breeze"
    middleware "github.com/nelthaarion/breeze/middlewares"
)

func main() {
    router := breeze.NewRouter()

    router.Use(middleware.RecoveryMiddleware())
    router.Use(middleware.LoggingMiddleware())

    router.Handle(breeze.GET, "/", func(ctx *breeze.Context) {
        ctx.JSON(map[string]string{"status": "ok"})
    })

    router.Handle(breeze.GET, "/users/:id", func(ctx *breeze.Context) {
        ctx.JSON(map[string]string{"id": ctx.Param("id")})
    })

    pool := breeze.NewWorkerPool(runtime.NumCPU())
    app  := breeze.New(router, pool)
    app.Run(3000, true) // port, multiCore
}
```

Run it:

```bash
go run main.go
# → curl http://localhost:3000/        → {"status":"ok"}
# → curl http://localhost:3000/users/42 → {"id":"42"}
```

## Docker

The repository ships a multi-stage `Dockerfile` and `docker-compose.yml`
that containerize the example server in `./cmd` (~25 MB image, static
binary, non-root user, built-in healthcheck):

```bash
# Plain Docker
docker build -t breeze-example .
docker run --rm -p 3000:3000 breeze-example

# Or with Compose
docker compose up --build
```

Breeze itself is a library — to containerize your own application, point
the `BREEZE_TARGET` build argument at any main package in the module:

```bash
docker build --build-arg BREEZE_TARGET=./cmd/dashboard-example -t my-app .
```

## 🧰 CLI — Scaffolding & Code Generation

Breeze ships a `rails`-style CLI for scaffolding projects and generating
CRUD boilerplate.

```bash
go install github.com/nelthaarion/breeze/cmd/breeze@latest
```

**Start a new project:**

```bash
breeze new myapp                    # minimal REST API layout (default)
breeze new myapp --template=views   # + views/components/template engine
```

**Generate a full CRUD resource** — structs, handlers, an in-memory store,
and OpenAPI docs, wired into the router automatically:

```bash
breeze generate resource User name:string email:string age:int
```

**Generate a bare handler stub** (no structs, no docs):

```bash
breeze generate handler Session --methods=get,create
```

Both generators write to `handlers/<name>.go` and register routes in a
single `routes_generated.go` file — your hand-written `main.go` is never
touched. Re-running `generate` for the same resource replaces its block,
so it's safe to regenerate after adding fields (pass `--force` to overwrite
the handler file too).

Supported field types: `string`, `int`, `int64`, `float64`, `bool`, `time.Time`.

## Features

### 🚀 Built for Extreme Performance

- ⚡ Event-driven architecture powered by `gnet`
- 🧠 Zero-copy HTTP request parsing where possible
- 📦 Minimal allocations with `sync.Pool`
- 🔥 Optimized response serialization (no `fmt.Sprintf`)
- 💨 Lock-free fast paths for critical operations
- 🎯 Preallocated buffers & cached status codes
- 📈 Worker Pool for scalable request processing

### 🌐 High-Performance Routing

- ⚡ Fast HTTP router
- 🎯 Dynamic route parameters
- 🌲 Wildcard routing
- 📂 Static file serving
- 🧩 Global middleware pipeline
- 🔍 Optimized route matching

### 🔌 Native WebSocket Engine

- ⚡ Zero-overhead HTTP → WebSocket upgrade
- 🔥 Dedicated WebSocket fast path
- 📡 Binary & Text frames
- ❤️ Ping / Pong support
- 🔄 Fragmented frame handling
- 🚪 Graceful close frames
- 🧵 Concurrent connection management

### 📚 Built-in OpenAPI / Scalar

- 📖 Automatic OpenAPI 3.1 generation
- 📝 Route registration
- 🎯 Schema generation
- 🔍 Typed request & response definitions
- 🌍 Ready for Scalar API Reference

### 🛡 Production Middleware

- 🚦 Rate Limiter
- 🗜 Compression
- 💾 Response Cache
- 🔑 JWT Authentication
- 🌍 CORS
- 🪖 Security Headers
- 📝 Request Logger
- 💥 Panic Recovery

### 📊 Built-in Developer Dashboard

- 🔧 Native module under `/dashboard` (zero-overhead when disabled)
- 📈 Real-time overview: RPS, latency, memory, goroutines, CPU
- 🛣 Routes Explorer with per-route latency stats
- 🧪 API Explorer with multi-language code generation (curl / Go / JS / Python / C# / PHP)
- 📡 Live Requests feed with WebSocket push
- 🗄 Database Browser (read-only, paginated)
- 🔍 ORM Query Monitor with slow-query detection
- 💾 Cache, Queue, and Scheduler monitors
- 📝 Logs with five tabs (App / HTTP / Errors / Panics / Warnings)
- ❤️ Health checks with green / yellow / red indicators
- ⚡ Go runtime performance metrics with charts
- 🕒 Developer Timeline — per-request profiler with expandable steps
- 🔒 HTTP Basic Auth + secret masking (Authorization, Cookie, API keys…)
- 🌑 Modern dark mode, responsive, single-file SPA (no external deps)

See [`dashboard/README.md`](./dashboard/README.md) for full documentation.

### ⚙️ Developer Experience

- 📦 Lightweight architecture
- 🎨 JSON responses out of the box
- 📄 Template rendering
- 📁 Static assets
- 🔍 Request validation
- 🧩 Simple Context API

### 🧠 Performance Optimizations

- Zero-copy body handling
- Header reuse
- Copy-on-write headers
- Cached HTTP status text
- Unsafe string conversions
- Compact receive buffers
- Optimized HTTP parser
- Single-pass header parsing
- Reduced GC pressure

---

## ❤️ Support the Project

If Breeze saves you time, consider buying me a coffee. Every contribution
keeps the framework maintained and the benchmarks honest.

<div align="center">

<img width="185" height="215" alt="Support Breeze" src="https://github.com/user-attachments/assets/def5c7fc-4c6e-480b-91a4-2a574f23a533" />

**USDT (BEP20)**

`0x2EF70423BC989C5203c9031811179DD6EDe32793`

</div>

---

## 🤝 Contributing

We welcome contributions of all sizes.

Whether it's fixing bugs, improving documentation, optimizing
performance, or adding new features — every contribution helps make
Breeze better.

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feat/my-thing`)
3. **Commit** your changes with a clear message
4. **Open** a pull request describing what and why

Please open an issue first for non-trivial changes so we can align on
the approach before you spend time on code.

## 🔐 Security Scanning

Breeze now includes automated security checks in GitHub Actions:

- **CodeQL** static analysis (`.github/workflows/codeql.yml`)
- **govulncheck** vulnerability scanning for Go packages and reachable code (`.github/workflows/govulncheck.yml`)
- **Gitleaks** secret scanning (`.github/workflows/secret-scan.yml`)
- **Dependabot** weekly updates for Go modules and GitHub Actions (`.github/dependabot.yml`)

For repository admins:

- Enable GitHub Advanced Security **secret scanning** and **push protection** in repository settings when available.
- Configure branch protection to require the three security workflow checks before merge.
- Use the triage process in `.github/SECURITY_TRIAGE.md` to classify and resolve alerts.

## License

Breeze is released under the [MIT License](./LICENSE).

© Nelthaarion
