package dashboard

import "testing"

func TestMaskLineQuotedJSON(t *testing.T) {
	cfg := Config{MaskedHeaders: []string{"authorization"}}

	got := maskLine(cfg, `{"authorization":"Bearer-xyz"}`)
	want := `{"authorization":"••••••"}`
	if got != want {
		t.Errorf("maskLine() = %q, want %q", got, want)
	}
}

func TestMaskLineKeyValue(t *testing.T) {
	cfg := Config{MaskedHeaders: []string{"token"}}

	got := maskLine(cfg, "token=abc123 other=fine")
	want := "token=•••••• other=fine"
	if got != want {
		t.Errorf("maskLine() = %q, want %q", got, want)
	}
}
