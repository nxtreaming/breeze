package main

import "testing"

func TestParseFieldsValid(t *testing.T) {
	fields, err := parseFields([]string{"name:string", "age:int", "signedUpAt:time.Time"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []field{
		{Name: "Name", JSON: "name", Type: "string"},
		{Name: "Age", JSON: "age", Type: "int"},
		{Name: "SignedUpAt", JSON: "signedUpAt", Type: "time.Time"},
	}
	if len(fields) != len(want) {
		t.Fatalf("got %d fields, want %d", len(fields), len(want))
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("field %d = %+v, want %+v", i, fields[i], want[i])
		}
	}
}

func TestParseFieldsErrors(t *testing.T) {
	cases := []string{
		"noColon",
		"1invalid:string",
		"name:unsupportedtype",
		"name:string:extra",
		":string",
		"name:",
	}
	for _, c := range cases {
		if _, err := parseFields([]string{c}); err == nil {
			t.Errorf("parseFields(%q) expected error, got nil", c)
		}
	}
}

func TestParseFieldsDuplicate(t *testing.T) {
	if _, err := parseFields([]string{"name:string", "Name:int"}); err == nil {
		t.Error("expected error for duplicate field, got nil")
	}
}
