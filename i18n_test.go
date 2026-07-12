package breeze

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLocaleFiles creates a temp locales dir with an English and a Danish
// locale file. The Danish file uses the top-level locale wrapper
// ({"da": {...}}) to verify both formats load.
func writeLocaleFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	en := `{
		"home": {
			"title": "Welcome",
			"greeting": "Hello, %{name}!"
		},
		"cart": {
			"items": {
				"zero": "No items",
				"one": "1 item",
				"other": "%{count} items"
			}
		},
		"only_english": "English only"
	}`

	da := `{
		"da": {
			"home": {
				"title": "Velkommen",
				"greeting": "Hej, %{name}!"
			},
			"cart": {
				"items": {
					"one": "1 vare",
					"other": "%{count} varer"
				}
			}
		}
	}`

	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(en), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "da.json"), []byte(da), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newTestI18n(t *testing.T, cfg I18nConfig) *I18n {
	t.Helper()
	if cfg.Dir == "" {
		cfg.Dir = writeLocaleFiles(t)
	}
	if cfg.DefaultLocale == "" {
		cfg.DefaultLocale = "en"
	}
	i18n, err := NewI18n(cfg)
	if err != nil {
		t.Fatalf("NewI18n: %v", err)
	}
	return i18n
}

func TestI18n_LoadAndLookup(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{})
	if got := i18n.T("en", "home.title"); got != "Welcome" {
		t.Errorf(`T("en", "home.title") = %q, want "Welcome"`, got)
	}
}

func TestI18n_WrappedLocaleFile(t *testing.T) {
	// da.json wraps its tree in a top-level "da" key;
	// it must load identically to the unwrapped en.json.
	i18n := newTestI18n(t, I18nConfig{})
	if got := i18n.T("da", "home.title"); got != "Velkommen" {
		t.Errorf(`T("da", "home.title") = %q, want "Velkommen"`, got)
	}
}

func TestI18n_Locales(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{})
	locales := i18n.Locales()
	if len(locales) != 2 {
		t.Fatalf("Locales() = %v, want 2 locales", locales)
	}
	seen := map[string]bool{}
	for _, l := range locales {
		seen[l] = true
	}
	if !seen["en"] || !seen["da"] {
		t.Errorf("Locales() = %v, want [da en]", locales)
	}
}

func TestI18n_Interpolation(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{})
	if got := i18n.T("en", "home.greeting", "name", "Alice"); got != "Hello, Alice!" {
		t.Errorf(`greeting = %q, want "Hello, Alice!"`, got)
	}
}

func TestI18n_InterpolationUnknownPlaceholderKept(t *testing.T) {
	// Placeholders with no matching argument are left as-is so mistakes
	// are visible instead of silently vanishing.
	i18n := newTestI18n(t, I18nConfig{})
	if got := i18n.T("en", "home.greeting"); got != "Hello, %{name}!" {
		t.Errorf(`greeting without args = %q, want "Hello, %%{name}!"`, got)
	}
}

func TestI18n_Pluralization(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{})

	cases := []struct {
		count any
		want  string
	}{
		{0, "No items"}, // "zero" key present in en
		{1, "1 item"},
		{3, "3 items"},
	}
	for _, c := range cases {
		if got := i18n.T("en", "cart.items", "count", c.count); got != c.want {
			t.Errorf(`T("en", "cart.items", "count", %v) = %q, want %q`, c.count, got, c.want)
		}
	}

	// Danish has no "zero" key — count 0 falls through to "other".
	if got := i18n.T("da", "cart.items", "count", 0); got != "0 varer" {
		t.Errorf(`da zero fallback = %q, want "0 varer"`, got)
	}
}

func TestI18n_FallbackToDefaultLocale(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{Fallback: true})
	// "only_english" is missing in da — must fall back to the en value.
	if got := i18n.T("da", "only_english"); got != "English only" {
		t.Errorf(`fallback = %q, want "English only"`, got)
	}
}

func TestI18n_UnknownLocaleFallsBack(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{Fallback: true})
	if got := i18n.T("fr", "home.title"); got != "Welcome" {
		t.Errorf(`unknown locale = %q, want "Welcome"`, got)
	}
}

func TestI18n_MissingKeyDevMode(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{DevMode: true})
	want := "translation missing: en.nope.nothing"
	if got := i18n.T("en", "nope.nothing"); got != want {
		t.Errorf(`dev missing = %q, want %q`, got, want)
	}
}

func TestI18n_MissingKeyProdHumanizes(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{Fallback: true})
	// Missing everywhere → humanized last key segment.
	if got := i18n.T("en", "checkout.promo_code"); got != "Promo code" {
		t.Errorf(`humanized = %q, want "Promo code"`, got)
	}
}

func TestI18n_NegotiateLocale(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{})

	cases := []struct {
		header string
		want   string
	}{
		{"da", "da"},                     // exact match
		{"da-DK", "da"},                  // region tag falls back to primary
		{"fr, da;q=0.8, en;q=0.5", "da"}, // q-value ordering, skip unknown
		{"en;q=0.5, da;q=0.9", "da"},     // order in header irrelevant
		{"fr, de", ""},                   // nothing available
		{"*", "en"},                      // wildcard → default locale
		{"", ""},                         // empty header
		{";;;garbage;;q=x", ""},          // malformed input must not panic
		{"EN", "en"},                     // case-insensitive
	}
	for _, c := range cases {
		if got := i18n.NegotiateLocale(c.header); got != c.want {
			t.Errorf("NegotiateLocale(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestI18n_MissingDirErrors(t *testing.T) {
	_, err := NewI18n(I18nConfig{Dir: "/nonexistent/locales", DefaultLocale: "en"})
	if err == nil {
		t.Error("NewI18n with missing dir: want error, got nil")
	}
}

func TestI18n_InvalidJSONErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewI18n(I18nConfig{Dir: dir, DefaultLocale: "en"})
	if err == nil {
		t.Error("NewI18n with invalid JSON: want error, got nil")
	}
}

func TestI18n_ContextHelpers(t *testing.T) {
	i18n := newTestI18n(t, I18nConfig{})

	ctx := NewContext(GET, "/")
	ctx.SetLocale("da")
	ctx.SetI18n(i18n)

	if got := ctx.Locale(); got != "da" {
		t.Errorf("ctx.Locale() = %q, want %q", got, "da")
	}
	if got := ctx.T("home.title"); got != "Velkommen" {
		t.Errorf(`ctx.T("home.title") = %q, want "Velkommen"`, got)
	}

	// Without a bundle, ctx.T degrades to the key itself rather than panicking.
	bare := NewContext(GET, "/")
	if got := bare.T("home.title"); got != "home.title" {
		t.Errorf(`bare ctx.T = %q, want "home.title"`, got)
	}
}

func TestI18n_Reload(t *testing.T) {
	dir := writeLocaleFiles(t)
	i18n := newTestI18n(t, I18nConfig{Dir: dir})

	updated := `{"home": {"title": "Hi there"}}`
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := i18n.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := i18n.T("en", "home.title"); got != "Hi there" {
		t.Errorf(`after reload = %q, want "Hi there"`, got)
	}
}
