package breeze

import (
        "io"
        "mime"
        "net/http"
        "os"
        "path/filepath"
        "strings"
)

// ServeStatic registers handlers to serve files under `root` at URL prefix `prefix`.
// Example: ServeStatic("/static", "./public") will serve ./public/* at /static/*
func (r *Router) ServeStatic(prefix, root string) {
        // ensure prefix has no trailing slash when registering pattern,
        // the pattern we register will be prefix + "/*filepath"
        cleanPrefix := strings.TrimSuffix(prefix, "/")

        // handler for files: pattern: prefix + "/*filepath"
        pattern := cleanPrefix + "/*filepath"
        r.Handle(GET, pattern, func(ctx *Context) {
                fp := ctx.Param("filepath")
                // if client requested exactly '/static' (no trailing slash) treat as root index
                if fp == "" || fp == "/" {
                        fp = "index.html"
                }
                // sanitize path to avoid directory traversal
                fp = filepath.Clean("/" + fp)[1:] // make it relative and cleaned

                full := filepath.Join(root, fp)

                // open and serve file
                f, err := os.Open(full)
                if err != nil {
                        ctx.Status(404)
                        ctx.WriteString("File not found")
                        return
                }
                defer f.Close()

                info, err := f.Stat()
                if err != nil || info.IsDir() {
                        ctx.Status(404)
                        ctx.WriteString("File not found")
                        return
                }

                // For small/medium files: read into memory (simple)
                // If you want streaming for big files, use ctx.StreamFile or implement chunked writes.
                data, err := io.ReadAll(f)
                if err != nil {
                        ctx.Status(500)
                        ctx.WriteString("Error reading file")
                        return
                }

                ctype := mime.TypeByExtension(filepath.Ext(full))
                if ctype == "" {
                        ctype = http.DetectContentType(data)
                }

                ctx.Res = &HTTPResponse{
                        Status:  200,
                        Headers: map[string]string{"Content-Type": ctype},
                        Body:    data,
                }
        })
}
