package main

import (
	"fmt"
	"go/token"
	"strings"
)

// field is a single name:type pair parsed from `breeze generate resource`
// arguments, e.g. "email:string".
type field struct {
	Name string // Go-exported field name, e.g. "Email"
	JSON string // json tag, e.g. "email"
	Type string // Go type, e.g. "string"
}

var supportedFieldTypes = map[string]bool{
	"string":    true,
	"int":       true,
	"int64":     true,
	"float64":   true,
	"bool":      true,
	"time.Time": true,
}

// parseFields parses a list of "name:type" arguments into fields, validating
// that each name is a legal Go identifier and each type is supported.
func parseFields(args []string) ([]field, error) {
	fields := make([]field, 0, len(args))
	seen := make(map[string]bool, len(args))

	for _, arg := range args {
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid field %q — expected name:type (e.g. email:string)", arg)
		}
		name, typ := parts[0], parts[1]

		if !token.IsIdentifier(name) {
			return nil, fmt.Errorf("invalid field name %q — must be a valid Go identifier", name)
		}
		if !supportedFieldTypes[typ] {
			return nil, fmt.Errorf("unsupported field type %q for %q — supported types: string, int, int64, float64, bool, time.Time", typ, name)
		}

		exported := strings.ToUpper(name[:1]) + name[1:]
		if seen[exported] {
			return nil, fmt.Errorf("duplicate field %q", name)
		}
		seen[exported] = true

		fields = append(fields, field{Name: exported, JSON: name, Type: typ})
	}

	return fields, nil
}

// usesTime reports whether any field requires the time package to be
// imported by generated code.
func usesTime(fields []field) bool {
	for _, f := range fields {
		if f.Type == "time.Time" {
			return true
		}
	}
	return false
}
