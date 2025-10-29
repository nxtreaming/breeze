# 🌀 **Breeze** — High-Performance Golang Network Framework

> **Breeze** is a blazing-fast, asynchronous network framework built on top of [`gnet`](https://github.com/panjf2000/gnet).
> It combines the **raw speed of event-driven networking** with a **modern HTTP-style routing and context system**, offering a clean developer experience without sacrificing performance.

---

## 🚀 Features

* ⚡ **gnet-powered event loop** — built for millions of concurrent connections
* 🧠 **Context-based request lifecycle** — no unsafe global states
* 📦 **Router with method/path matching** (`GET`, `POST`, etc.)
* 🔄 **Async response writes** using `ctx.AsyncWrite()`
* 🧾 **Built-in JSON encoder** and custom response helpers (`ctx.JSON`, `ctx.String`, `ctx.Status`)
* 📁 **Multipart form/file upload & download** (any file type supported)
* 🧵 **Worker pool and concurrency-safe design**
* 🧩 **Middleware support** — easily attach pre/post request logic
* 🧰 **Lightweight and extensible** — embed your own logic or protocol layers

---

## 🧱 Architecture Overview

```
┌──────────────────────────────────────────┐
│                 Breeze                   │
│──────────────────────────────────────────│
│   gnet EventLoop                         │
│     ├─ Accepts TCP connections           │
│     ├─ Dispatches data frames            │
│     └─ Async I/O                         │
│──────────────────────────────────────────│
│   Context Layer (ctx)                    │
│     ├─ Request (headers, body, params)   │
│     ├─ Response (status, async write)    │
│     ├─ Middleware pre/post handling      │
│     └─ JSON / File helpers               │
│──────────────────────────────────────────│
│   Router                                 │
│     ├─ Match method + path               │
│     └─ Call handler(ctx)                 │
└──────────────────────────────────────────┘
```

---

## ⚙️ Installation

```bash
go get github.com/nelthaarion/breeze
```

---

## 🧩 Basic Usage

```go
package main

import (
	"github.com/nelthaarion/breeze"
	"runtime"
)

func main() {
	router := breeze.NewRouter()

	// Global middleware
	router.Use(func(ctx *breeze.Context, next breeze.HandlerFunc) {
		println("Incoming request:", ctx.Req.Path)
		next(ctx)
	})

	router.Handle(breeze.GET, "/", func(ctx *breeze.Context) {
		ctx.JSON(map[string]string{"message": "Welcome to Breeze!"})
	})

	router.Handle(breeze.POST, "/echo", func(ctx *breeze.Context) {
		ctx.JSON(map[string]string{"echo": string(ctx.Req.Body)})
	})
	app := breeze.New(router, breeze.NewWorkerPool(runtime.NumCPU()))
	app.Run(8080, router)
}
```

---

## 🧩 Middleware Example

```go
router.Use(func(ctx *breeze.Context, next breeze.HandlerFunc) {
	start := time.Now()
	next(ctx)
	fmt.Println("Handled in", time.Since(start))
})
```

Middlewares are executed in the order they are added before reaching the route handler.

---

## 📁 File Upload & Download

### Upload

```go
router.Handle(breeze.POST, "/upload", func(ctx *breeze.Context) {
	filename, err := ctx.SaveUploadedFile("file", "./uploads/received.bin", 50<<20) // 50 MB
	if err != nil {
		ctx.Status(400)
		ctx.String("Upload failed: " + err.Error())
		return
	}
	ctx.JSON(map[string]string{"saved_as": filename})
})
```

### Download

```go
router.Handle(breeze.GET, "/file/:name", func(ctx *breeze.Context) {
	name := ctx.Param("name")
	ctx.SendFile("./uploads/" + name)
})
```

---

## 🧠 Context Reference

| Method                                       | Description                        |
| -------------------------------------------- | ---------------------------------- |
| `ctx.JSON(data any)`                         | Write JSON response asynchronously |
| `ctx.String(str string)`                     | Write plain text                   |
| `ctx.Status(code int)`                       | Set response status                |
| `ctx.AsyncWrite(data []byte)`                | Low-level async writer             |
| `ctx.ParseMultipart(maxSize int64)`          | Parse multipart form               |
| `ctx.SaveUploadedFile(field, dest, maxSize)` | Save uploaded file to disk         |
| `ctx.Param(name string)`                     | Access route param                 |
| `ctx.Req.Body`                               | Raw request body bytes             |
| `ctx.Req.Header`                             | Request headers map                |

---

 
## 🧪 Example Folder Layout

```
breeze/
├─ main.go
├─ router.go
├─ context.go
├─ middleware.go
├─ request.go
├─ response.go
├─ file.go
└─ internal/
   └─ utils.go
```

---

## 🧑‍💻 Contributing

Contributions are welcome!
Please open issues and pull requests for new features, bug fixes, and optimizations.

---

## 📄 License

MIT License © 2025 Farhsad Khazaei Fard(https://github.com/nelthaarion)
