# Breeze

> **A ridiculously fast, event-driven Go web framework built for maximum
> throughput, minimal allocations, native WebSockets, and
> production-ready APIs.**

Breeze is a modern, high-performance Go web framework engineered for
developers who demand speed without sacrificing developer experience.
Built on an event-driven architecture, Breeze minimizes allocations,
optimizes every request path, and provides first-class support for REST
APIs, WebSockets, middleware, and automatic OpenAPI documentation.

Whether you're building microservices, real-time applications, or
high-throughput APIs, Breeze is designed to handle millions of requests
efficiently while keeping your code clean and maintainable.

------------------------------------------------------------------------

# 🚀 Installation

## Install

``` bash
go get github.com/nelthaarion/breeze
```
 

------------------------------------------------------------------------

 [![Documentation](https://img.shields.io/badge/Documentation-Latest-blue?style=for-the-badge)](https://nelthaarion.github.io/breeze)

------------------------------------------------------------------------

------------------------------------------------------------------------

 [![Donate](https://donatr.ee/ethanwinters) 
 <img width="185" height="215" alt="download" src="https://github.com/user-attachments/assets/def5c7fc-4c6e-480b-91a4-2a574f23a533" />


------------------------------------------------------------------------

# ✨ Features

### 🚀 Built for Extreme Performance

-   ⚡ Event-driven architecture powered by `gnet`
-   🧠 Zero-copy HTTP request parsing where possible
-   📦 Minimal allocations with `sync.Pool`
-   🔥 Optimized response serialization (no `fmt.Sprintf`)
-   💨 Lock-free fast paths for critical operations
-   🎯 Preallocated buffers & cached status codes
-   📈 Worker Pool for scalable request processing

### 🌐 High-Performance Routing

-   ⚡ Fast HTTP router
-   🎯 Dynamic route parameters
-   🌲 Wildcard routing
-   📂 Static file serving
-   🧩 Global middleware pipeline
-   🔍 Optimized route matching

### 🔌 Native WebSocket Engine

-   ⚡ Zero-overhead HTTP → WebSocket upgrade
-   🔥 Dedicated WebSocket fast path
-   📡 Binary & Text frames
-   ❤️ Ping / Pong support
-   🔄 Fragmented frame handling
-   🚪 Graceful close frames
-   🧵 Concurrent connection management

### 📚 Built-in OpenAPI / Swagger

-   📖 Automatic OpenAPI 3.1 generation
-   📝 Route registration
-   🎯 Schema generation
-   🔍 Typed request & response definitions
-   🌍 Ready for Swagger UI

### 🛡 Production Middleware

-   🚦 Rate Limiter
-   🗜 Compression
-   💾 Response Cache
-   🔑 JWT Authentication
-   🌍 CORS
-   🪖 Security Headers
-   📝 Request Logger
-   💥 Panic Recovery

### ⚙️ Developer Experience

-   📦 Lightweight architecture
-   🎨 JSON responses out of the box
-   📄 Template rendering
-   📁 Static assets
-   🔍 Request validation
-   🧩 Simple Context API

### 🧠 Performance Optimizations

-   Zero-copy body handling
-   Header reuse
-   Copy-on-write headers
-   Cached HTTP status text
-   Unsafe string conversions
-   Compact receive buffers
-   Optimized HTTP parser
-   Single-pass header parsing
-   Reduced GC pressure

🤝 Contributing

We welcome contributions of all sizes.

Whether it's fixing bugs, improving documentation, optimizing performance, or adding new features, every contribution helps make Breeze better.
