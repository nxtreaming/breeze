package dashboard

import (
	"encoding/json"
	"testing"

	"github.com/nelthaarion/breeze"
)

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
