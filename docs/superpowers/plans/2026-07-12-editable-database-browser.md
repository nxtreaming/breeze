# Editable Database Browser Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional Create/Update/Delete support to breeze's existing read-only dashboard Database Browser, gated behind a double opt-in (`Config.AllowWrites` + a host-app-supplied `DBWriter`), so it stays a natural, backward-compatible extension of the framework.

**Architecture:** A new `DBWriter` interface (separate from the existing `DBInspector`) is implemented by host apps and registered via `Collector.SetDBWriter`. Three new REST routes (`POST/PUT/DELETE .../rows[...]`) call it through a `writableGuard` helper that enforces the opt-in and validates the table name. Writes invalidate the existing 30s `cachedDBInspector` cache and append an audit entry to the existing "app" log channel. The frontend SPA (`dashboard/spajavascript.go`) renders edit/delete/new-row controls only when the server reports `TableData.Writable == true`.

**Tech Stack:** Go 1.24.3+, stdlib only (`errors`, `net/url`, `strings`, `fmt`) — no new dependencies. Frontend is vanilla JS (no framework, no external deps), matching the existing single-file SPA.

**Spec:** `docs/superpowers/specs/2026-07-12-editable-database-browser-design.md`

## Global Constraints

- Go 1.24.3 or later (per README.md "Installation").
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`) per CONTRIBUTING.md.
- Run `gofmt -w` on every modified `.go` file before committing; `gofmt -l .` must report nothing.
- No changes to the existing `DBInspector`, `TableInfo`, or `TableColumn` types — public API backward compatibility is required (CONTRIBUTING.md: "Keep public APIs backward compatible whenever possible").
- No new external Go dependencies. No new frontend dependencies (dashboard SPA is "single-file, no external deps" per `dashboard/README.md`).
- `go vet ./...` and `go test ./...` must pass before every commit (matches `.github/workflows/ci.yml`).
- All new backend code lives in package `dashboard` (`github.com/nelthaarion/breeze/dashboard`), following the existing module layout — no new package.

---

### Task 1: `DBWriter` interface, `ErrRowNotFound`, Collector wiring, `AllowWrites` config

**Files:**
- Create: `dashboard/db_writer.go`
- Modify: `dashboard/collector.go` (add `dbWriter` field + accessors, near the existing `dbInspector` field at line 80 and `DBInspector`/`SetDBInspector` methods at lines 133-146)
- Modify: `dashboard/config.go` (add `AllowWrites` field to `Config` struct, after `DisableAuth` at line 57)
- Test: `dashboard/db_writer_test.go`

**Interfaces:**
- Produces: `type DBWriter interface { InsertRow(table string, values map[string]any) (map[string]any, error); UpdateRow(table string, pk map[string]any, values map[string]any) error; DeleteRow(table string, pk map[string]any) error }`, `var ErrRowNotFound error`, `func (c *Collector) SetDBWriter(w DBWriter)`, `func (c *Collector) DBWriter() DBWriter`, `Config.AllowWrites bool`.

- [ ] **Step 1: Write the failing test**

Create `dashboard/db_writer_test.go`:

```go
package dashboard

import "testing"

// mockWriter is a minimal DBWriter used across db_writer_test.go.
type mockWriter struct {
	insertErr error
	updateErr error
	deleteErr error
}

func (m *mockWriter) InsertRow(table string, values map[string]any) (map[string]any, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	out := map[string]any{"id": "1"}
	for k, v := range values {
		out[k] = v
	}
	return out, nil
}

func (m *mockWriter) UpdateRow(table string, pk map[string]any, values map[string]any) error {
	return m.updateErr
}

func (m *mockWriter) DeleteRow(table string, pk map[string]any) error {
	return m.deleteErr
}

// TestSetDBWriter_Nil verifies that passing nil to SetDBWriter clears the
// writer (no nil pointer panic), mirroring TestSetDBInspector_Nil in
// cached_inspector_test.go.
func TestSetDBWriter_Nil(t *testing.T) {
	cfg := DefaultConfig()
	c := newCollector(cfg, nil)
	c.SetDBWriter(&mockWriter{})
	if c.DBWriter() == nil {
		t.Fatal("writer not set")
	}
	c.SetDBWriter(nil)
	if c.DBWriter() != nil {
		t.Fatal("writer not cleared")
	}
}

// TestConfigAllowWritesDefaultsFalse verifies AllowWrites is false unless
// explicitly set, so upgrading breeze never silently makes data editable.
func TestConfigAllowWritesDefaultsFalse(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.AllowWrites {
		t.Error("AllowWrites should default to false")
	}
	cfg2 := DefaultConfig()
	if cfg2.AllowWrites {
		t.Error("DefaultConfig().AllowWrites should be false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dashboard/... -run 'TestSetDBWriter_Nil|TestConfigAllowWritesDefaultsFalse' -v`
Expected: FAIL to compile — `mockWriter` does not implement an interface named `DBWriter` (undefined), `c.SetDBWriter` undefined, `cfg.AllowWrites` undefined.

- [ ] **Step 3: Create `dashboard/db_writer.go`**

```go
package dashboard

import "errors"

// DBWriter provides write access to database tables for the Database
// Browser's Create/Update/Delete UI. It is deliberately separate from
// DBInspector so existing DBInspector implementations keep compiling
// unmodified — CRUD only activates when a host app also implements this
// interface AND Config.AllowWrites is true (see Collector.writableGuard).
//
// pk is keyed by the primary-key column name(s) reported via
// TableColumn.PrimaryKey in the table's TableData.Columns, so composite
// keys work without extra API surface. Values in pk and values are always
// strings as received from the HTTP layer — implementations are
// responsible for converting to the underlying column type, the same
// division of responsibility DBInspector already has for reads.
type DBWriter interface {
	// InsertRow creates a new row and returns it as stored (including any
	// DB-assigned defaults such as auto-increment primary keys).
	InsertRow(table string, values map[string]any) (map[string]any, error)

	// UpdateRow updates the row identified by pk with the given column
	// values. Returns ErrRowNotFound if no row matches pk.
	UpdateRow(table string, pk map[string]any, values map[string]any) error

	// DeleteRow deletes the row identified by pk. Returns ErrRowNotFound
	// if no row matches pk.
	DeleteRow(table string, pk map[string]any) error
}

// ErrRowNotFound is returned by DBWriter.UpdateRow/DeleteRow when pk does
// not match any existing row.
var ErrRowNotFound = errors.New("dashboard: row not found")
```

- [ ] **Step 4: Add `dbWriter` field and accessors to `dashboard/collector.go`**

In the `Collector` struct, immediately after the existing `dbInspector DBInspector` field (line 80):

```go
	// Database inspector used by the database browser.
	dbInspector DBInspector

	// Database writer used by the database browser's CRUD UI. Optional —
	// nil means the Database Browser stays read-only regardless of
	// Config.AllowWrites (see writableGuard in api.go).
	dbWriter DBWriter
```

Immediately after the existing `SetDBInspector` method (after line 146):

```go
// DBWriter exposes the current database writer, if one was set.
func (c *Collector) DBWriter() DBWriter {
	return c.dbWriter
}

// SetDBWriter installs a database writer, enabling Create/Update/Delete in
// the Database Browser (still gated by Config.AllowWrites). Passing nil
// clears the writer and reverts the Database Browser to read-only.
func (c *Collector) SetDBWriter(w DBWriter) {
	c.dbWriter = w
}
```

- [ ] **Step 5: Add `AllowWrites` to `dashboard/config.go`**

In the `Config` struct, immediately after `DisableAuth` (line 57):

```go
	// AllowWrites enables Create/Update/Delete in the Database Browser.
	// Defaults to false. Even when a DBWriter is configured via
	// Collector.SetDBWriter, writes stay disabled until this is
	// explicitly set — a deliberate double opt-in (operator config +
	// application code) so upgrading breeze or wiring a DBWriter for
	// read-side reasons never silently makes production data editable.
	AllowWrites bool `yaml:"allow_writes" json:"allow_writes"`
```

No change is needed in `withDefaults()` — Go's zero value for `bool` is already `false`, which is the desired default.

- [ ] **Step 6: Run gofmt and the test again**

Run: `gofmt -w dashboard/db_writer.go dashboard/db_writer_test.go dashboard/collector.go dashboard/config.go`
Run: `go test ./dashboard/... -run 'TestSetDBWriter_Nil|TestConfigAllowWritesDefaultsFalse' -v`
Expected: `ok` — both tests PASS.

- [ ] **Step 7: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`, no vet errors.

- [ ] **Step 8: Commit**

```bash
git add dashboard/db_writer.go dashboard/db_writer_test.go dashboard/collector.go dashboard/config.go
git commit -m "feat(dashboard): add DBWriter interface and AllowWrites config gate"
```

---

### Task 2: Cache invalidation on write

**Files:**
- Modify: `dashboard/db_inspector.go` (add `Invalidate` method to `cachedDBInspector`, after `TableData` at line ~87)
- Modify: `dashboard/collector.go` (add `invalidateTableCache` helper)
- Test: `dashboard/cached_inspector_test.go`

**Interfaces:**
- Consumes: `cachedDBInspector.data map[dbTableDataKey]cachedTableData` and `dataMu sync.RWMutex` (both already defined in `db_inspector.go`); `Collector.dbInspector DBInspector` (Task 1's neighbor field, already existing).
- Produces: `func (c *cachedDBInspector) Invalidate(table string)`, `func (c *Collector) invalidateTableCache(table string)` — used by Tasks 5-7's write handlers.

- [ ] **Step 1: Write the failing test**

Add to `dashboard/cached_inspector_test.go`:

```go
// TestCachedInspector_Invalidate verifies that Invalidate(table) clears
// only that table's cached TableData pages, leaving other tables' cached
// data (and the cached Tables() list) untouched.
func TestCachedInspector_Invalidate(t *testing.T) {
	mock := &mockInspector{}
	cfg := DefaultConfig()
	c := newCollector(cfg, nil)
	c.SetDBInspector(mock)

	ci, ok := c.dbInspector.(*cachedDBInspector)
	if !ok {
		t.Fatal("SetDBInspector did not wrap the inspector in a cachedDBInspector")
	}

	// Populate the cache for two tables.
	_, _ = ci.TableData("users", 1, 50, "")
	_, _ = ci.TableData("posts", 1, 50, "")
	if calls := atomic.LoadInt32(&mock.dataCalls); calls != 2 {
		t.Fatalf("dataCalls = %d, want 2 after populating cache", calls)
	}

	ci.Invalidate("users")

	// "users" was invalidated — this call must hit the underlying inspector.
	_, _ = ci.TableData("users", 1, 50, "")
	if calls := atomic.LoadInt32(&mock.dataCalls); calls != 3 {
		t.Errorf("dataCalls = %d, want 3 (users should be a cache miss after Invalidate)", calls)
	}

	// "posts" was NOT invalidated — this call must stay cached.
	_, _ = ci.TableData("posts", 1, 50, "")
	if calls := atomic.LoadInt32(&mock.dataCalls); calls != 3 {
		t.Errorf("dataCalls = %d, want 3 (posts should still be cached)", calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dashboard/... -run TestCachedInspector_Invalidate -v`
Expected: FAIL to compile — `ci.Invalidate` undefined on `*cachedDBInspector`.

- [ ] **Step 3: Implement `Invalidate` in `dashboard/db_inspector.go`**

Add after the existing `TableData` method (after line 87, the closing brace of `func (c *cachedDBInspector) TableData(...)`):

```go
// Invalidate clears cached TableData pages for the given table, forcing
// the next read to hit the underlying inspector. It does not affect the
// cached Tables() result (row counts may lag up to the cache TTL after a
// write — the same staleness the read path already tolerates).
func (c *cachedDBInspector) Invalidate(table string) {
	c.dataMu.Lock()
	defer c.dataMu.Unlock()
	for key := range c.data {
		if key.name == table {
			delete(c.data, key)
		}
	}
}
```

- [ ] **Step 4: Add `invalidateTableCache` to `dashboard/collector.go`**

Add after the `SetDBWriter`/`DBWriter` methods added in Task 1:

```go
// invalidateTableCache clears the cached Database Browser rows for table
// after a successful write. It is a no-op if no inspector is configured.
func (c *Collector) invalidateTableCache(table string) {
	if ci, ok := c.dbInspector.(*cachedDBInspector); ok {
		ci.Invalidate(table)
	}
}
```

- [ ] **Step 5: Run gofmt and the test again**

Run: `gofmt -w dashboard/db_inspector.go dashboard/collector.go dashboard/cached_inspector_test.go`
Run: `go test ./dashboard/... -run TestCachedInspector_Invalidate -v`
Expected: PASS.

- [ ] **Step 6: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add dashboard/db_inspector.go dashboard/collector.go dashboard/cached_inspector_test.go
git commit -m "feat(dashboard): invalidate table cache after writes"
```

---

### Task 3: `TableData.Writable` field

**Files:**
- Modify: `dashboard/types.go` (add `Writable` to `TableData`, line 55-62)
- Modify: `dashboard/api.go` (set it in `handleDBTableData`, lines 445-460)
- Test: `dashboard/db_writer_test.go`

**Interfaces:**
- Consumes: `Collector.cfg.AllowWrites` (Task 1), `Collector.DBWriter()` (Task 1).
- Produces: `TableData.Writable bool` (JSON key `"writable"`) — consumed by the frontend in Task 8.

- [ ] **Step 1: Write the failing test**

Replace the file's `import "testing"` line (from Task 1) with:

```go
import (
	"encoding/json"
	"testing"

	"github.com/nelthaarion/breeze"
)
```

Then add to `dashboard/db_writer_test.go`:

```go
// TestHandleDBTableData_WritableFlag verifies the "writable" JSON field
// reflects both the AllowWrites config and whether a DBWriter is set.
func TestHandleDBTableData_WritableFlag(t *testing.T) {
	cases := []struct {
		name        string
		allowWrites bool
		setWriter   bool
		want        bool
	}{
		{"disabled by default", false, false, false},
		{"writer set but AllowWrites false", false, true, false},
		{"AllowWrites true but no writer", true, false, false},
		{"both set", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.AllowWrites = tc.allowWrites
			c := newCollector(cfg, nil)
			c.SetDBInspector(&mockInspector{})
			if tc.setWriter {
				c.SetDBWriter(&mockWriter{})
			}

			ctx := breeze.NewContext(breeze.GET, "/api/db/tables/users")
			ctx.SetParam("name", "users")
			c.handleDBTableData(ctx)

			var data TableData
			if err := json.Unmarshal(ctx.Res.Body, &data); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if data.Writable != tc.want {
				t.Errorf("Writable = %v, want %v", data.Writable, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dashboard/... -run TestHandleDBTableData_WritableFlag -v`
Expected: FAIL to compile — `TableData.Writable` undefined (or, once it compiles as a zero-value bool, the "both set" case fails since nothing sets it yet).

- [ ] **Step 3: Add the field to `dashboard/types.go`**

Change the `TableData` struct (lines 55-62):

```go
// TableData is one paginated table view used by the database browser.
type TableData struct {
	Table    string            `json:"table"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int64             `json:"total"`
	// Writable is true when the Database Browser's Create/Update/Delete UI
	// is enabled for this response — i.e. Config.AllowWrites is true AND
	// a DBWriter is configured. The frontend uses this as the single
	// source of truth for whether to render edit controls; it never
	// infers writability on its own.
	Writable bool                   `json:"writable,omitempty"`
	Columns  []TableColumn          `json:"columns,omitempty"`
	Rows     []map[string]any       `json:"rows,omitempty"`
}
```

- [ ] **Step 4: Set it in `dashboard/api.go`'s `handleDBTableData`**

Replace the body of `handleDBTableData` (lines 445-460):

```go
func (c *Collector) handleDBTableData(ctx *breeze.Context) {
	inspector := c.DBInspector()
	if inspector == nil {
		ctx.JSON(TableData{Table: ctx.Param("name"), Page: 1, PageSize: 50, Total: 0, Rows: []map[string]any{}, Columns: []TableColumn{}})
		return
	}
	page := atoiDefault(ctx.Query("page"), 1)
	pageSize := atoiDefault(ctx.Query("page_size"), 50)
	data, err := inspector.TableData(ctx.Param("name"), page, pageSize, ctx.Query("search"))
	if err != nil {
		ctx.Status(500)
		ctx.JSON(map[string]any{"error": err.Error()})
		return
	}
	data.Writable = c.cfg.AllowWrites && c.DBWriter() != nil
	ctx.JSON(data)
}
```

- [ ] **Step 5: Run gofmt and the test again**

Run: `gofmt -w dashboard/types.go dashboard/api.go dashboard/db_writer_test.go`
Run: `go test ./dashboard/... -run TestHandleDBTableData_WritableFlag -v`
Expected: all 4 subtests PASS.

- [ ] **Step 6: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add dashboard/types.go dashboard/api.go dashboard/db_writer_test.go
git commit -m "feat(dashboard): report writable status on TableData"
```

---

### Task 4: `parsePK` helper

**Files:**
- Modify: `dashboard/db_writer.go` (add `parsePK`)
- Test: `dashboard/db_writer_test.go`

**Interfaces:**
- Produces: `func parsePK(raw string) map[string]any` — consumed by Tasks 6-7's update/delete handlers.

- [ ] **Step 1: Write the failing test**

Add to `dashboard/db_writer_test.go`:

```go
func TestParsePK(t *testing.T) {
	cases := []struct {
		in   string
		want map[string]any
	}{
		{"", map[string]any{}},
		{"id=42", map[string]any{"id": "42"}},
		{"a=1,b=2", map[string]any{"a": "1", "b": "2"}},
		{"name=John%20Doe", map[string]any{"name": "John Doe"}},
	}
	for _, c := range cases {
		got := parsePK(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("parsePK(%q) = %v, want %v", c.in, got, c.want)
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("parsePK(%q)[%q] = %v, want %v", c.in, k, got[k], v)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dashboard/... -run TestParsePK -v`
Expected: FAIL to compile — `parsePK` undefined.

- [ ] **Step 3: Implement `parsePK` in `dashboard/db_writer.go`**

Replace the file's `import "errors"` line (from Task 1) with:

```go
import (
	"errors"
	"net/url"
	"strings"
)
```

Then add at the end of the file:

```go
// parsePK parses a URL path segment of the form "col1=val1,col2=val2" into
// a primary-key map for DBWriter.UpdateRow/DeleteRow. Each key and value
// is percent-decoded independently, so callers building the URL (see the
// frontend's pkPathFor in dashboard/spajavascript.go) must
// encodeURIComponent each key and value before joining them. Malformed
// pairs (missing "=", undecodable escapes) are silently skipped rather
// than erroring — DBWriter implementations naturally reject an incomplete
// pk via ErrRowNotFound when the row can't be found.
func parsePK(raw string) map[string]any {
	pk := make(map[string]any)
	if raw == "" {
		return pk
	}
	for _, pair := range strings.Split(raw, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, err1 := url.QueryUnescape(kv[0])
		val, err2 := url.QueryUnescape(kv[1])
		if err1 != nil || err2 != nil {
			continue
		}
		pk[key] = val
	}
	return pk
}
```

- [ ] **Step 4: Run gofmt and the test again**

Run: `gofmt -w dashboard/db_writer.go dashboard/db_writer_test.go`
Run: `go test ./dashboard/... -run TestParsePK -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add dashboard/db_writer.go dashboard/db_writer_test.go
git commit -m "feat(dashboard): add parsePK helper for composite primary keys"
```

---

### Task 5: `POST /rows` insert handler + route + `writableGuard`

**Files:**
- Modify: `dashboard/api.go` (add `writableGuard`, `handleDBTableInsert`, register the route, add `"errors"` import, remove the now-unused `var _ = fmt.Sprintf` dummy at line 654)
- Test: `dashboard/db_writer_test.go`

**Interfaces:**
- Consumes: `Collector.cfg.AllowWrites`/`DBWriter()`/`DBInspector()` (Task 1), `Collector.invalidateTableCache` (Task 2), `Collector.RecordLog` (existing, `collector.go:231`), `jsonUnmarshal` (existing, `api.go:18`).
- Produces: `func (c *Collector) writableGuard(ctx *breeze.Context, table string) (DBWriter, bool)` — reused by Tasks 6-7. `func (c *Collector) handleDBTableInsert(ctx *breeze.Context)`.

- [ ] **Step 1: Write the failing test**

Add to `dashboard/db_writer_test.go`:

```go
func TestHandleDBTableInsert(t *testing.T) {
	newCollectorWithWriter := func(allowWrites bool, writer DBWriter) *Collector {
		cfg := DefaultConfig()
		cfg.AllowWrites = allowWrites
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		if writer != nil {
			c.SetDBWriter(writer)
		}
		return c
	}

	t.Run("success", func(t *testing.T) {
		c := newCollectorWithWriter(true, &mockWriter{})
		ctx := breeze.NewContext(breeze.POST, "/api/db/tables/users/rows")
		ctx.SetParam("name", "users")
		ctx.Req.Body = []byte(`{"values":{"name":"Alice"}}`)

		c.handleDBTableInsert(ctx)

		if ctx.Res.Status != 201 {
			t.Fatalf("Status = %d, want 201; body=%s", ctx.Res.Status, ctx.Res.Body)
		}
		var row map[string]any
		if err := json.Unmarshal(ctx.Res.Body, &row); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if row["name"] != "Alice" {
			t.Errorf("row[name] = %v, want Alice", row["name"])
		}
	})

	t.Run("writes disabled", func(t *testing.T) {
		c := newCollectorWithWriter(false, &mockWriter{})
		ctx := breeze.NewContext(breeze.POST, "/api/db/tables/users/rows")
		ctx.SetParam("name", "users")
		ctx.Req.Body = []byte(`{"values":{"name":"Alice"}}`)

		c.handleDBTableInsert(ctx)

		if ctx.Res.Status != 403 {
			t.Fatalf("Status = %d, want 403", ctx.Res.Status)
		}
	})

	t.Run("no writer configured", func(t *testing.T) {
		c := newCollectorWithWriter(true, nil)
		ctx := breeze.NewContext(breeze.POST, "/api/db/tables/users/rows")
		ctx.SetParam("name", "users")
		ctx.Req.Body = []byte(`{"values":{"name":"Alice"}}`)

		c.handleDBTableInsert(ctx)

		if ctx.Res.Status != 403 {
			t.Fatalf("Status = %d, want 403", ctx.Res.Status)
		}
	})

	t.Run("unknown table", func(t *testing.T) {
		c := newCollectorWithWriter(true, &mockWriter{})
		ctx := breeze.NewContext(breeze.POST, "/api/db/tables/ghost/rows")
		ctx.SetParam("name", "ghost")
		ctx.Req.Body = []byte(`{"values":{"name":"Alice"}}`)

		c.handleDBTableInsert(ctx)

		if ctx.Res.Status != 400 {
			t.Fatalf("Status = %d, want 400", ctx.Res.Status)
		}
	})

	t.Run("writer error", func(t *testing.T) {
		c := newCollectorWithWriter(true, &mockWriter{insertErr: errors.New("constraint violation")})
		ctx := breeze.NewContext(breeze.POST, "/api/db/tables/users/rows")
		ctx.SetParam("name", "users")
		ctx.Req.Body = []byte(`{"values":{"name":"Alice"}}`)

		c.handleDBTableInsert(ctx)

		if ctx.Res.Status != 400 {
			t.Fatalf("Status = %d, want 400", ctx.Res.Status)
		}
	})

	t.Run("cache invalidated and log recorded on success", func(t *testing.T) {
		mock := &mockInspector{}
		cfg := DefaultConfig()
		cfg.AllowWrites = true
		c := newCollector(cfg, nil)
		c.SetDBInspector(mock)
		c.SetDBWriter(&mockWriter{})

		// Prime the cache for "users".
		ins := c.DBInspector()
		_, _ = ins.TableData("users", 1, 50, "")
		if calls := atomic.LoadInt32(&mock.dataCalls); calls != 1 {
			t.Fatalf("dataCalls = %d, want 1 after priming cache", calls)
		}

		ctx := breeze.NewContext(breeze.POST, "/api/db/tables/users/rows")
		ctx.SetParam("name", "users")
		ctx.Req.Body = []byte(`{"values":{"name":"Alice"}}`)
		c.handleDBTableInsert(ctx)

		// A subsequent read must miss the cache (it was invalidated).
		_, _ = ins.TableData("users", 1, 50, "")
		if calls := atomic.LoadInt32(&mock.dataCalls); calls != 2 {
			t.Errorf("dataCalls = %d, want 2 (cache should have been invalidated)", calls)
		}

		logs := c.Logs("app", 0)
		if len(logs) != 1 {
			t.Fatalf("Logs(app) = %d entries, want 1", len(logs))
		}
	})
}
```

Add `"encoding/json"`, `"errors"`, and `"sync/atomic"` to the test file's imports (alongside the existing `"testing"` and `"github.com/nelthaarion/breeze"` added in Task 3).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dashboard/... -run TestHandleDBTableInsert -v`
Expected: FAIL to compile — `c.handleDBTableInsert` undefined.

- [ ] **Step 3: Add `"errors"` import and `writableGuard` to `dashboard/api.go`**

Add `"errors"` to the import block (line 3-14):

```go
import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/nelthaarion/breeze"
)
```

Add after `handleDBTableData` (after the code from Task 3's Step 4):

```go
// writableGuard checks that the Database Browser's write path is enabled
// (Config.AllowWrites + a configured DBWriter) and that table is a table
// the inspector actually reports, so writes can't target an unlisted or
// hand-crafted table name. On failure it writes the appropriate error
// response to ctx and returns ok=false; callers must return immediately.
func (c *Collector) writableGuard(ctx *breeze.Context, table string) (DBWriter, bool) {
	if !c.cfg.AllowWrites {
		ctx.Status(403)
		ctx.JSON(map[string]any{"error": "writes are not enabled"})
		return nil, false
	}
	writer := c.DBWriter()
	inspector := c.DBInspector()
	if writer == nil || inspector == nil {
		ctx.Status(403)
		ctx.JSON(map[string]any{"error": "writes are not enabled"})
		return nil, false
	}
	tables, err := inspector.Tables()
	if err != nil {
		ctx.Status(500)
		ctx.JSON(map[string]any{"error": err.Error()})
		return nil, false
	}
	for _, t := range tables {
		if t.Name == table {
			return writer, true
		}
	}
	ctx.Status(400)
	ctx.JSON(map[string]any{"error": "unknown table: " + table})
	return nil, false
}
```

- [ ] **Step 4: Add `handleDBTableInsert`**

Add immediately after `writableGuard`:

```go
func (c *Collector) handleDBTableInsert(ctx *breeze.Context) {
	table := ctx.Param("name")
	writer, ok := c.writableGuard(ctx, table)
	if !ok {
		return
	}
	var req struct {
		Values map[string]any `json:"values"`
	}
	if err := jsonUnmarshal(ctx.Req.Body, &req); err != nil {
		ctx.Status(400)
		ctx.JSON(map[string]any{"error": "invalid request body"})
		return
	}
	row, err := writer.InsertRow(table, req.Values)
	if err != nil {
		ctx.Status(400)
		ctx.JSON(map[string]any{"error": err.Error()})
		return
	}
	c.invalidateTableCache(table)
	c.RecordLog("app", LogEntry{Time: now(), Message: fmt.Sprintf("db write: insert into %s", table)})
	ctx.Status(201)
	ctx.JSON(row)
}
```

- [ ] **Step 5: Register the route**

In `registerRoutes` (`dashboard/api.go`), immediately after the existing `api+"/db/tables/:name"` registration (line 198):

```go
	router.Handle(breeze.GET, api+"/db/tables", c.wrap(auth, c.handleDBTables))
	router.Handle(breeze.GET, api+"/db/tables/:name", c.wrap(auth, c.handleDBTableData))
	router.Handle(breeze.POST, api+"/db/tables/:name/rows", c.wrap(auth, c.handleDBTableInsert))
```

- [ ] **Step 6: Remove the now-dead `fmt` placeholder**

At the end of `dashboard/api.go` (line 653-654), delete:

```go
// dummy use of fmt to keep the import alive when handlers are simplified later.
var _ = fmt.Sprintf
```

`fmt` is now genuinely used by `handleDBTableInsert`'s `fmt.Sprintf` call, so the placeholder is no longer needed (and would be a duplicate import use, not a compile error, but it's dead code that no longer serves its stated purpose).

- [ ] **Step 7: Run gofmt and the test again**

Run: `gofmt -w dashboard/api.go dashboard/db_writer_test.go`
Run: `go test ./dashboard/... -run TestHandleDBTableInsert -v`
Expected: all 6 subtests PASS.

- [ ] **Step 8: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`.

- [ ] **Step 9: Commit**

```bash
git add dashboard/api.go dashboard/db_writer_test.go
git commit -m "feat(dashboard): add POST /rows insert endpoint to Database Browser"
```

---

### Task 6: `PUT /rows/:pk` update handler + route

**Files:**
- Modify: `dashboard/api.go` (add `handleDBTableUpdate`, register the route)
- Test: `dashboard/db_writer_test.go`

**Interfaces:**
- Consumes: `writableGuard` (Task 5), `parsePK` (Task 4), `ErrRowNotFound` (Task 1), `invalidateTableCache` (Task 2).
- Produces: `func (c *Collector) handleDBTableUpdate(ctx *breeze.Context)`.

- [ ] **Step 1: Write the failing test**

Add to `dashboard/db_writer_test.go`:

```go
func TestHandleDBTableUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowWrites = true
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		c.SetDBWriter(&mockWriter{})

		ctx := breeze.NewContext(breeze.PUT, "/api/db/tables/users/rows/id=1")
		ctx.SetParam("name", "users")
		ctx.SetParam("pk", "id=1")
		ctx.Req.Body = []byte(`{"values":{"name":"Bob"}}`)

		c.handleDBTableUpdate(ctx)

		if ctx.Res.Status != 200 {
			t.Fatalf("Status = %d, want 200; body=%s", ctx.Res.Status, ctx.Res.Body)
		}
	})

	t.Run("row not found", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowWrites = true
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		c.SetDBWriter(&mockWriter{updateErr: ErrRowNotFound})

		ctx := breeze.NewContext(breeze.PUT, "/api/db/tables/users/rows/id=999")
		ctx.SetParam("name", "users")
		ctx.SetParam("pk", "id=999")
		ctx.Req.Body = []byte(`{"values":{"name":"Bob"}}`)

		c.handleDBTableUpdate(ctx)

		if ctx.Res.Status != 404 {
			t.Fatalf("Status = %d, want 404", ctx.Res.Status)
		}
	})

	t.Run("writes disabled", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowWrites = false
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		c.SetDBWriter(&mockWriter{})

		ctx := breeze.NewContext(breeze.PUT, "/api/db/tables/users/rows/id=1")
		ctx.SetParam("name", "users")
		ctx.SetParam("pk", "id=1")
		ctx.Req.Body = []byte(`{"values":{"name":"Bob"}}`)

		c.handleDBTableUpdate(ctx)

		if ctx.Res.Status != 403 {
			t.Fatalf("Status = %d, want 403", ctx.Res.Status)
		}
	})

	t.Run("writer error", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowWrites = true
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		c.SetDBWriter(&mockWriter{updateErr: errors.New("constraint violation")})

		ctx := breeze.NewContext(breeze.PUT, "/api/db/tables/users/rows/id=1")
		ctx.SetParam("name", "users")
		ctx.SetParam("pk", "id=1")
		ctx.Req.Body = []byte(`{"values":{"name":"Bob"}}`)

		c.handleDBTableUpdate(ctx)

		if ctx.Res.Status != 400 {
			t.Fatalf("Status = %d, want 400", ctx.Res.Status)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dashboard/... -run TestHandleDBTableUpdate -v`
Expected: FAIL to compile — `c.handleDBTableUpdate` undefined.

- [ ] **Step 3: Add `handleDBTableUpdate` to `dashboard/api.go`**

Add immediately after `handleDBTableInsert`:

```go
func (c *Collector) handleDBTableUpdate(ctx *breeze.Context) {
	table := ctx.Param("name")
	writer, ok := c.writableGuard(ctx, table)
	if !ok {
		return
	}
	pk := parsePK(ctx.Param("pk"))
	var req struct {
		Values map[string]any `json:"values"`
	}
	if err := jsonUnmarshal(ctx.Req.Body, &req); err != nil {
		ctx.Status(400)
		ctx.JSON(map[string]any{"error": "invalid request body"})
		return
	}
	err := writer.UpdateRow(table, pk, req.Values)
	if errors.Is(err, ErrRowNotFound) {
		ctx.Status(404)
		ctx.JSON(map[string]any{"error": "row not found"})
		return
	}
	if err != nil {
		ctx.Status(400)
		ctx.JSON(map[string]any{"error": err.Error()})
		return
	}
	c.invalidateTableCache(table)
	c.RecordLog("app", LogEntry{Time: now(), Message: fmt.Sprintf("db write: update %s", table)})
	ctx.JSON(map[string]any{"ok": true})
}
```

- [ ] **Step 4: Register the route**

In `registerRoutes`, immediately after the `POST .../rows` route added in Task 5:

```go
	router.Handle(breeze.PUT, api+"/db/tables/:name/rows/:pk", c.wrap(auth, c.handleDBTableUpdate))
```

- [ ] **Step 5: Run gofmt and the test again**

Run: `gofmt -w dashboard/api.go dashboard/db_writer_test.go`
Run: `go test ./dashboard/... -run TestHandleDBTableUpdate -v`
Expected: all 4 subtests PASS.

- [ ] **Step 6: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add dashboard/api.go dashboard/db_writer_test.go
git commit -m "feat(dashboard): add PUT /rows/:pk update endpoint to Database Browser"
```

---

### Task 7: `DELETE /rows/:pk` handler + route

**Files:**
- Modify: `dashboard/api.go` (add `handleDBTableDelete`, register the route)
- Test: `dashboard/db_writer_test.go`

**Interfaces:**
- Consumes: `writableGuard` (Task 5), `parsePK` (Task 4), `ErrRowNotFound` (Task 1), `invalidateTableCache` (Task 2).
- Produces: `func (c *Collector) handleDBTableDelete(ctx *breeze.Context)`.

- [ ] **Step 1: Write the failing test**

Add to `dashboard/db_writer_test.go`:

```go
func TestHandleDBTableDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowWrites = true
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		c.SetDBWriter(&mockWriter{})

		ctx := breeze.NewContext(breeze.DELETE, "/api/db/tables/users/rows/id=1")
		ctx.SetParam("name", "users")
		ctx.SetParam("pk", "id=1")

		c.handleDBTableDelete(ctx)

		if ctx.Res.Status != 204 {
			t.Fatalf("Status = %d, want 204; body=%s", ctx.Res.Status, ctx.Res.Body)
		}
		if len(ctx.Res.Body) != 0 {
			t.Errorf("Body = %q, want empty for 204", ctx.Res.Body)
		}
	})

	t.Run("row not found", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowWrites = true
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		c.SetDBWriter(&mockWriter{deleteErr: ErrRowNotFound})

		ctx := breeze.NewContext(breeze.DELETE, "/api/db/tables/users/rows/id=999")
		ctx.SetParam("name", "users")
		ctx.SetParam("pk", "id=999")

		c.handleDBTableDelete(ctx)

		if ctx.Res.Status != 404 {
			t.Fatalf("Status = %d, want 404", ctx.Res.Status)
		}
	})

	t.Run("writes disabled", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.AllowWrites = false
		c := newCollector(cfg, nil)
		c.SetDBInspector(&mockInspector{})
		c.SetDBWriter(&mockWriter{})

		ctx := breeze.NewContext(breeze.DELETE, "/api/db/tables/users/rows/id=1")
		ctx.SetParam("name", "users")
		ctx.SetParam("pk", "id=1")

		c.handleDBTableDelete(ctx)

		if ctx.Res.Status != 403 {
			t.Fatalf("Status = %d, want 403", ctx.Res.Status)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dashboard/... -run TestHandleDBTableDelete -v`
Expected: FAIL to compile — `c.handleDBTableDelete` undefined.

- [ ] **Step 3: Add `handleDBTableDelete` to `dashboard/api.go`**

Add immediately after `handleDBTableUpdate`:

```go
func (c *Collector) handleDBTableDelete(ctx *breeze.Context) {
	table := ctx.Param("name")
	writer, ok := c.writableGuard(ctx, table)
	if !ok {
		return
	}
	pk := parsePK(ctx.Param("pk"))
	err := writer.DeleteRow(table, pk)
	if errors.Is(err, ErrRowNotFound) {
		ctx.Status(404)
		ctx.JSON(map[string]any{"error": "row not found"})
		return
	}
	if err != nil {
		ctx.Status(400)
		ctx.JSON(map[string]any{"error": err.Error()})
		return
	}
	c.invalidateTableCache(table)
	c.RecordLog("app", LogEntry{Time: now(), Message: fmt.Sprintf("db write: delete from %s", table)})
	ctx.Status(204)
}
```

- [ ] **Step 4: Register the route**

In `registerRoutes`, immediately after the `PUT .../rows/:pk` route added in Task 6:

```go
	router.Handle(breeze.DELETE, api+"/db/tables/:name/rows/:pk", c.wrap(auth, c.handleDBTableDelete))
```

- [ ] **Step 5: Run gofmt and the test again**

Run: `gofmt -w dashboard/api.go dashboard/db_writer_test.go`
Run: `go test ./dashboard/... -run TestHandleDBTableDelete -v`
Expected: all 3 subtests PASS.

- [ ] **Step 6: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add dashboard/api.go dashboard/db_writer_test.go
git commit -m "feat(dashboard): add DELETE /rows/:pk endpoint to Database Browser"
```

---

### Task 8: Frontend inline-edit UI

**Files:**
- Modify: `dashboard/spajavascript.go` (`renderDatabase()` at line 788, `api`/`apiPost` helpers around line 118-132)

**Interfaces:**
- Consumes: `TableData.writable` (JSON field from Task 3), the three new routes from Tasks 5-7.
- Produces: `apiSend(path, method, body)`, `pkPathFor(row, columns)`, `onDBCellBlur`, `deleteDBRow`, `saveNewDBRow` — page-local functions, no consumers outside this file.

There is no automated test harness for this file (the dashboard SPA has no JS test framework — verification is manual, via Task 9's example app). Steps below are implementation + a manual verification checklist.

- [ ] **Step 1: Add `apiSend` helper**

`dashboard/spajavascript.go` is a single Go file whose entire body (from `const spaJS = \`` on line 13 to the closing backtick near the end of the file) is one raw string literal of plain JavaScript — edits to "JS" in this task are plain text edits to that Go file, no Go syntax involved.

Immediately after the existing `apiPost` function (after line 132, which reads `}`), insert:

```js
function apiSend(path, method, body){
  return fetch(S.base + '/api/' + path, {
    method: method,
    headers: {'Content-Type': 'application/json'},
    body: body===undefined ? undefined : JSON.stringify(body)
  }).then(function(r){
    return r.json().catch(function(){return {};}).then(function(data){
      return {ok:r.ok, status:r.status, data:data};
    });
  });
}
```

- [ ] **Step 2: Replace `renderDatabase()` and add row-editing functions**

Replace the entire `renderDatabase()` function (lines 788-848) with:

```js
function renderDatabase(){
  var c = $('#page-database');
  var sel = S.dbTableSel;
  var html = '<div class="db-grid">'+
    '<div class="db-tables"><div style="padding:10px 12px;border-bottom:1px solid var(--border);font-size:11px;text-transform:uppercase;letter-spacing:.5px;color:var(--text-dim);font-weight:600">Tables ('+S.dbTables.length+')</div>';
  S.dbTables.forEach(function(t, i){
    html += '<div class="item '+(sel===i?'active':'')+'" data-idx="'+i+'">'+
      '<span>'+escapeHTML(t.name)+'</span>'+
      '<span class="count">'+fmtNum(t.rows)+'</span>'+
      '</div>';
  });
  if(!S.dbTables.length) html += '<div class="empty" style="padding:20px">No tables. Set up a DBInspector to enable.</div>';
  html += '</div><div class="db-data">';
  if(sel!=null && S.dbTables[sel]){
    var t = S.dbTables[sel];
    var writable = !!(S.dbData && S.dbData.writable);
    html += '<div class="toolbar"><strong style="font-family:var(--mono)">'+escapeHTML(t.name)+'</strong>'+
      '<input id="db-search" placeholder="Search..." value="'+escapeHTML(S._dbSearch||'')+'" style="max-width:240px">'+
      '<button id="db-refresh">Refresh</button>'+
      (writable ? '<button id="db-new-row">New row</button>' : '')+
      '</div>';
    if(S.dbData){
      var d = S.dbData;
      html += '<div class="table-scroll" style="flex:1;overflow:auto"><table><thead><tr>';
      d.columns.forEach(function(col){
        html += '<th>'+escapeHTML(col.name)+'<br><span style="font-size:9px;text-transform:none;color:var(--text-muted)">'+escapeHTML(col.type)+(col.primary_key?' PK':'')+(col.nullable?'':' NN')+'</span></th>';
      });
      if(d.writable) html += '<th></th>';
      html += '</tr></thead><tbody>';
      if(S._dbNewRow){
        html += '<tr class="db-new-row">';
        d.columns.forEach(function(col){
          html += '<td><input data-col="'+escapeHTML(col.name)+'" style="width:100%" '+(col.primary_key?'placeholder="auto"':'')+'></td>';
        });
        html += '<td><button id="db-new-row-save">Save</button> <button id="db-new-row-cancel">Cancel</button></td></tr>';
      }
      d.rows.forEach(function(row, ri){
        html += '<tr data-ri="'+ri+'">';
        d.columns.forEach(function(col){
          var v = row[col.name];
          var text = v==null?'':String(v);
          if(d.writable && !col.primary_key){
            html += '<td class="db-cell" data-col="'+escapeHTML(col.name)+'" contenteditable="true">'+escapeHTML(text)+'</td>';
          } else {
            html += '<td style="font-size:11px">'+(v==null?'<span style="color:var(--text-muted)">NULL</span>':escapeHTML(text))+'</td>';
          }
        });
        if(d.writable) html += '<td><button class="danger db-delete-row" data-ri="'+ri+'">Delete</button></td>';
        html += '</tr>';
      });
      if(!d.rows.length && !S._dbNewRow) html += '<tr><td colspan="'+(d.columns.length+(d.writable?1:0))+'" class="empty">No rows</td></tr>';
      html += '</tbody></table></div>';
      html += '<div class="pager"><div>Page '+d.page+' of '+Math.max(1, Math.ceil(d.total/d.page_size))+' ('+fmtNum(d.total)+' rows)</div>'+
        '<div><button id="db-prev" '+(d.page<=1?'disabled':'')+'>Prev</button> '+
        '<button id="db-next" '+(d.page>=Math.ceil(d.total/d.page_size)?'disabled':'')+'>Next</button></div></div>';
    } else {
      html += '<div class="empty" style="padding:40px"><div class="icon">&#x25a3;</div>Loading...</div>';
    }
  } else {
    html += '<div class="empty" style="padding:40px"><div class="icon">&#x25a3;</div>Select a table to browse</div>';
  }
  html += '</div></div>';
  c.innerHTML = html;
  $$('.db-tables .item').forEach(function(it){
    it.addEventListener('click', function(){
      S.dbTableSel = parseInt(it.dataset.idx);
      S.dbData = null;
      S.dbPage = 1;
      S._dbNewRow = false;
      renderDatabase();
      loadDBData();
    });
  });
  var ds = $('#db-search'); if(ds) ds.addEventListener('input', function(){S._dbSearch=ds.value;});
  var dr = $('#db-refresh'); if(dr) dr.addEventListener('click', function(){S.dbPage=1; loadDBData();});
  var dp = $('#db-prev'); if(dp) dp.addEventListener('click', function(){S.dbPage--; loadDBData();});
  var dn = $('#db-next'); if(dn) dn.addEventListener('click', function(){S.dbPage++; loadDBData();});
  var dnw = $('#db-new-row'); if(dnw) dnw.addEventListener('click', function(){S._dbNewRow=true; renderDatabase();});
  var dnwc = $('#db-new-row-cancel'); if(dnwc) dnwc.addEventListener('click', function(){S._dbNewRow=false; renderDatabase();});
  var dnws = $('#db-new-row-save'); if(dnws) dnws.addEventListener('click', saveNewDBRow);
  $$('.db-cell').forEach(function(cell){
    cell.addEventListener('blur', onDBCellBlur);
  });
  $$('.db-delete-row').forEach(function(btn){
    btn.addEventListener('click', function(){ deleteDBRow(parseInt(btn.dataset.ri)); });
  });
}
function pkPathFor(row, columns){
  var parts = [];
  columns.forEach(function(col){
    if(col.primary_key) parts.push(encodeURIComponent(col.name)+'='+encodeURIComponent(row[col.name]));
  });
  return parts.join(',');
}
function onDBCellBlur(e){
  var cell = e.target;
  var ri = parseInt(cell.closest('tr').dataset.ri);
  var col = cell.dataset.col;
  var row = S.dbData.rows[ri];
  var newVal = cell.textContent;
  if(row[col]!=null && String(row[col])===newVal) return;
  var t = S.dbTables[S.dbTableSel];
  var pk = pkPathFor(row, S.dbData.columns);
  var values = {}; values[col] = newVal;
  apiSend('db/tables/'+encodeURIComponent(t.name)+'/rows/'+pk, 'PUT', {values: values}).then(function(res){
    if(!res.ok){ alert('Update failed: '+((res.data&&res.data.error)||res.status)); loadDBData(); return; }
    row[col] = newVal;
  });
}
function deleteDBRow(ri){
  if(!confirm('Delete this row?')) return;
  var row = S.dbData.rows[ri];
  var t = S.dbTables[S.dbTableSel];
  var pk = pkPathFor(row, S.dbData.columns);
  apiSend('db/tables/'+encodeURIComponent(t.name)+'/rows/'+pk, 'DELETE').then(function(res){
    if(!res.ok){ alert('Delete failed: '+((res.data&&res.data.error)||res.status)); return; }
    loadDBData();
  });
}
function saveNewDBRow(){
  var t = S.dbTables[S.dbTableSel];
  var values = {};
  $$('#page-database tr.db-new-row input').forEach(function(input){
    if(input.value!=='') values[input.dataset.col] = input.value;
  });
  apiSend('db/tables/'+encodeURIComponent(t.name)+'/rows', 'POST', {values: values}).then(function(res){
    if(!res.ok){ alert('Insert failed: '+((res.data&&res.data.error)||res.status)); return; }
    S._dbNewRow = false;
    loadDBData();
  });
}
```

- [ ] **Step 3: Compile-check the Go file**

Run: `gofmt -w dashboard/spajavascript.go && go build ./...`
Expected: builds cleanly (the JS lives inside a Go string literal — this only catches Go-level syntax errors like an unterminated string, not JS bugs).

- [ ] **Step 4: Commit**

```bash
git add dashboard/spajavascript.go
git commit -m "feat(dashboard): add inline edit/delete/new-row UI to Database Browser"
```

(Manual browser verification of this task happens in Task 9, once the example app has a working `DBWriter` to point the UI at.)

---

### Task 9: Wire a working example `DBInspector`/`DBWriter` into `cmd/dashboard-example`

The example app currently never calls `coll.SetDBInspector(...)` at all, so its Database Browser page has always shown "No tables. Set up a DBInspector to enable." This task makes the example actually demonstrate the (previously unimplemented) Database Browser, plus the new write support — and gives us a real app to manually verify Task 8's frontend against.

**Files:**
- Modify: `cmd/dashboard-example/main.go`

**Interfaces:**
- Consumes: `dashboard.DBInspector`, `dashboard.DBWriter`, `dashboard.TableInfo`, `dashboard.TableColumn`, `dashboard.TableData`, `dashboard.ErrRowNotFound` (all from Tasks 1-4), `Collector.SetDBInspector`/`SetDBWriter` (existing + Task 1).
- Produces: `UserStore` implementing both `dashboard.DBInspector` and `dashboard.DBWriter` — no other file depends on this; it is a leaf/example.

- [ ] **Step 1: Add `Tables`/`TableData` methods to `UserStore`**

In `cmd/dashboard-example/main.go`, add after the existing `Count` method (after line 104), and add `"sort"` and `"strings"` to the import block:

```go
import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/nelthaarion/breeze"
	"github.com/nelthaarion/breeze/dashboard"
)
```

```go
// ─── dashboard.DBInspector / dashboard.DBWriter ────────────────────────────
//
// This is what wires the Database Browser's "users" table view. In a real
// app you'd implement these against your actual database/ORM instead of an
// in-memory map.

func (s *UserStore) Tables() ([]dashboard.TableInfo, error) {
	return []dashboard.TableInfo{{Name: "users", Rows: int64(s.Count())}}, nil
}

func (s *UserStore) TableData(name string, page, pageSize int, search string) (dashboard.TableData, error) {
	if name != "users" {
		return dashboard.TableData{}, fmt.Errorf("unknown table: %s", name)
	}
	s.mu.RLock()
	all := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		all = append(all, u)
	}
	s.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	if search != "" {
		q := strings.ToLower(search)
		filtered := all[:0]
		for _, u := range all {
			if strings.Contains(strings.ToLower(u.Name), q) || strings.Contains(strings.ToLower(u.Email), q) {
				filtered = append(filtered, u)
			}
		}
		all = filtered
	}

	total := int64(len(all))
	start := (page - 1) * pageSize
	if start > len(all) {
		start = len(all)
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}

	rows := make([]map[string]any, 0, end-start)
	for _, u := range all[start:end] {
		rows = append(rows, map[string]any{
			"id":         u.ID,
			"name":       u.Name,
			"email":      u.Email,
			"created_at": u.CreatedAt.Format(time.RFC3339),
		})
	}

	return dashboard.TableData{
		Table:    name,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Columns: []dashboard.TableColumn{
			{Name: "id", Type: "int", PrimaryKey: true},
			{Name: "name", Type: "string"},
			{Name: "email", Type: "string"},
			{Name: "created_at", Type: "datetime"},
		},
		Rows: rows,
	}, nil
}

func (s *UserStore) InsertRow(table string, values map[string]any) (map[string]any, error) {
	if table != "users" {
		return nil, fmt.Errorf("unknown table: %s", table)
	}
	name, _ := values["name"].(string)
	email, _ := values["email"].(string)
	if name == "" || email == "" {
		return nil, fmt.Errorf("name and email are required")
	}
	u := s.Create(name, email)
	return map[string]any{
		"id":         u.ID,
		"name":       u.Name,
		"email":      u.Email,
		"created_at": u.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *UserStore) UpdateRow(table string, pk map[string]any, values map[string]any) error {
	if table != "users" {
		return fmt.Errorf("unknown table: %s", table)
	}
	idStr, _ := pk["id"].(string)
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return dashboard.ErrRowNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return dashboard.ErrRowNotFound
	}
	if v, ok := values["name"].(string); ok && v != "" {
		u.Name = v
	}
	if v, ok := values["email"].(string); ok && v != "" {
		u.Email = v
	}
	return nil
}

func (s *UserStore) DeleteRow(table string, pk map[string]any) error {
	if table != "users" {
		return fmt.Errorf("unknown table: %s", table)
	}
	idStr, _ := pk["id"].(string)
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return dashboard.ErrRowNotFound
	}
	if !s.Delete(id) {
		return dashboard.ErrRowNotFound
	}
	return nil
}
```

- [ ] **Step 2: Wire the store into the dashboard in `main()`**

In `main()`, immediately after `cfg := dashboard.DefaultConfig()` (line 123):

```go
	cfg := dashboard.DefaultConfig()
	cfg.AllowWrites = true // demonstrates the editable Database Browser; leave false in production unless intended
```

Immediately after `coll := dashboard.Install(app, router, cfg)` (line 125):

```go
	coll := dashboard.Install(app, router, cfg)
	coll.SetDBInspector(store)
	coll.SetDBWriter(store)
```

- [ ] **Step 3: Build and run the example**

Run: `go build ./cmd/dashboard-example/...`
Expected: builds cleanly.

Run: `go run ./cmd/dashboard-example` (leave running in the background for Step 4)
Expected: prints `Breeze listening on :3000`.

- [ ] **Step 4: Manual verification checklist**

With the example running:
1. `curl -u admin:admin http://localhost:3000/dashboard/api/db/tables` → JSON containing `{"name":"users","rows":0}`.
2. `curl -X POST http://localhost:3000/api/users -H 'Content-Type: application/json' -d '{"name":"Alice","email":"alice@example.com"}'` → creates a user via the *application* API (seeds data to look at).
3. `curl -u admin:admin 'http://localhost:3000/dashboard/api/db/tables/users'` → JSON with `"writable":true` and one row for Alice.
4. `curl -u admin:admin -X POST http://localhost:3000/dashboard/api/db/tables/users/rows -H 'Content-Type: application/json' -d '{"values":{"name":"Bob","email":"bob@example.com"}}'` → `201` with the inserted row.
5. `curl -u admin:admin -X PUT 'http://localhost:3000/dashboard/api/db/tables/users/rows/id=1' -H 'Content-Type: application/json' -d '{"values":{"name":"Alice Smith"}}'` → `200`.
6. `curl -u admin:admin -X DELETE 'http://localhost:3000/dashboard/api/db/tables/users/rows/id=1'` → `204`.
7. Open `http://localhost:3000/dashboard/database` in a browser (login admin/admin), select "users", confirm: rows are editable inline (click a non-PK cell, edit, click away — value persists after switching pages), "New row" adds a row, the delete button removes a row after confirmation.
8. Stop the example process.

- [ ] **Step 5: Run the full test suite and vet**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok` (the example app has no `_test.go` file to run, but this confirms the change didn't break anything else in the module).

- [ ] **Step 6: Commit**

```bash
git add cmd/dashboard-example/main.go
git commit -m "feat(dashboard-example): wire UserStore as DBInspector/DBWriter"
```

---

### Task 10: Update documentation

**Files:**
- Modify: `dashboard/README.md` (Database Browser section, and the Configuration YAML example)
- Modify: `README.md` (root — Database Browser bullet in the feature list)

**Interfaces:**
- Consumes: nothing (docs only).
- Produces: nothing (docs only).

- [ ] **Step 1: Update `dashboard/README.md`**

Replace the "Database Browser" paragraph (item 7, under "Develop"):

```markdown
#### 7. Database Browser
Browse every table with pagination, search, and column metadata (type, nullable, primary key, index, defaults, foreign-key references). Read-only unless both `Config.AllowWrites` is `true` and the application has called `Collector.SetDBWriter` with a `DBWriter` implementation — in which case rows can be created, edited inline, and deleted directly from the browser. See [DBWriter](#dbwriter-optional-crud) below.
```

Add a new section after the "Configuration" section (after the closing ` ``` ` of the YAML example):

```markdown
---

## DBWriter (optional CRUD)

By default the Database Browser is read-only. To enable Create/Update/Delete:

```go
coll.SetDBInspector(store) // existing read-only interface
coll.SetDBWriter(store)    // NEW — enables writes
cfg.AllowWrites = true     // NEW — must also be explicitly enabled
```

`DBWriter` is a separate interface from `DBInspector` so existing read-only
integrations are unaffected:

```go
type DBWriter interface {
    InsertRow(table string, values map[string]any) (map[string]any, error)
    UpdateRow(table string, pk map[string]any, values map[string]any) error
    DeleteRow(table string, pk map[string]any) error
}
```

Both `Config.AllowWrites` and a configured `DBWriter` are required — either
one alone leaves the browser read-only. See `cmd/dashboard-example/main.go`
for a full `UserStore` implementation of both interfaces.
```

- [ ] **Step 2: Update the root `README.md`**

In the "Built-in Developer Dashboard" bullet list (around line 195), change:

```markdown
- 🗄 Database Browser (read-only, paginated)
```

to:

```markdown
- 🗄 Database Browser (paginated; optional inline Create/Update/Delete via `DBWriter`)
```

- [ ] **Step 3: Verify the module still builds**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all `ok` (docs-only change, but confirms nothing else was left half-edited).

- [ ] **Step 4: Commit**

```bash
git add dashboard/README.md README.md
git commit -m "docs(dashboard): document DBWriter and the editable Database Browser"
```

---

## Final verification

- [ ] Run the full suite one more time from repo root: `go vet ./... && go test ./... -v 2>&1 | tail -80`
- [ ] Run `gofmt -l .` — expect empty output (no unformatted files).
- [ ] Confirm branch `feat/editable-database-browser` has 10 focused commits (one per task above, in order), each with a Conventional Commits message.
- [ ] Re-read `docs/superpowers/specs/2026-07-12-editable-database-browser-design.md` end to end and confirm every section (Architecture, Data flow, Frontend, Error handling, Testing, Backward compatibility) is implemented — see the Spec Coverage table below.

**Spec coverage:**

| Spec section | Implemented in |
|---|---|
| `DBWriter` interface, separate from `DBInspector` | Task 1 |
| `Collector.SetDBWriter`/`DBWriter()` | Task 1 |
| `Config.AllowWrites` double opt-in | Task 1 |
| Three new REST routes | Tasks 5, 6, 7 |
| Table-name validation against `Tables()` | Task 5 (`writableGuard`, reused by 6-7) |
| Cache invalidation on write | Task 2, exercised by Tasks 5-7's tests |
| `TableData.Writable` | Task 3 |
| Composite-key `:pk` encoding/parsing | Task 4 |
| Frontend inline edit/delete/new-row, gated on `writable` | Task 8 |
| Error handling (400/403/404, `ErrRowNotFound`) | Tasks 5, 6, 7 |
| Audit log via existing `RecordLog("app", ...)` | Tasks 5, 6, 7 |
| Backward compatibility (no `DBInspector`/`TableInfo`/`TableColumn` changes) | Global Constraints; verified by Task 1 not touching those types |
