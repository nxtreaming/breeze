package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func generateTestResourceFile(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	fields, err := parseFields([]string{"name:string", "age:int"})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeResourceHandlerFile("User", "Users", fields, false); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("handlers", "user.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

func TestResourceHandlerImportsJSON(t *testing.T) {
	src := generateTestResourceFile(t)
	if !strings.Contains(src, `"encoding/json"`) {
		t.Error("generated handler is missing the encoding/json import")
	}
}

func TestResourceStoreMarkedAsDemo(t *testing.T) {
	src := generateTestResourceFile(t)
	if !strings.Contains(src, "In-memory store for scaffolding only") {
		t.Error("generated in-memory store is not marked as scaffolding/demo code")
	}
}

func TestParseFieldsEmptyNameNoPanic(t *testing.T) {
	for _, c := range []string{"", ":", ":string", "name:"} {
		if _, err := parseFields([]string{c}); err == nil {
			t.Errorf("parseFields(%q) expected error, got nil", c)
		}
	}
}

func TestResourceRoutesUseScalarPackage(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := generateResource("example.com/x", "User", []string{"name:string"}); err != nil {
		t.Fatal(err)
	}
	routes, err := os.ReadFile(registryFileName)
	if err != nil {
		t.Fatal(err)
	}
	src := string(routes)
	if !strings.Contains(src, `"github.com/nelthaarion/breeze/scalar"`) || !strings.Contains(src, "scalar.RouteDoc") {
		t.Errorf("routes must use the scalar package (middleware.Doc* takes scalar.RouteDoc), got:\n%s", src)
	}
	if strings.Contains(src, "swagger.") {
		t.Errorf("routes still reference the old swagger package:\n%s", src)
	}
}
