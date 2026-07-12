package breeze

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// spaHarness wraps the injected SPA runtime with just enough DOM stubbing to
// execute it under node, so the client-side template evaluator can be tested
// for real instead of by string inspection.
const spaHarness = `
'use strict';

function makeDataTag(json, locale) {
	return {
		textContent: json,
		getAttribute: function (n) { return n === 'data-locale' ? locale : null; },
	};
}

var targetEl = {
	innerHTML: '',
	querySelectorAll: function () { return []; },
};

var document = {
	_els: {
		'__breeze_tmpl__': makeDataTag(process.env.BREEZE_TMPL, ''),
		'__breeze_i18n__': makeDataTag(process.env.BREEZE_I18N, process.env.BREEZE_LOCALE),
		'target': targetEl,
	},
	getElementById: function (id) { return this._els[id] || null; },
	querySelector: function (sel) { return this._els[sel.replace('#', '')] || null; },
	addEventListener: function () {},
	body: { classList: { add: function () {}, remove: function () {} } },
};

var window = {
	addEventListener: function () {},
	dispatchEvent: function () {},
	location: { pathname: '/', search: '', origin: 'http://x', protocol: 'http:', host: 'x' },
	scrollY: 0,
	scrollTo: function () {},
};

var history = {
	state: null,
	replaceState: function (s) { this.state = s; },
	pushState: function () {},
};

var CustomEvent = function (name, opts) { this.detail = opts && opts.detail; };
var requestAnimationFrame = function (fn) { fn(); };

// ── runtime under test is appended below ──
`

// runSPAHarness executes the SPA runtime under node with the given embedded
// template sources / i18n dictionary and returns whatever testJS prints.
func runSPAHarness(t *testing.T, tmplJSON, i18nJSON, locale, testJS string) string {
	t.Helper()

	runtime := breezeRuntime()
	start := strings.Index(runtime, ">")
	end := strings.LastIndex(runtime, "</script>")
	if start < 0 || end < 0 {
		t.Fatal("cannot extract script body from breezeRuntime()")
	}
	body := runtime[start+1 : end]

	script := spaHarness + body + "\n" + testJS
	path := filepath.Join(t.TempDir(), "harness.js")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("node", path)
	cmd.Env = append(os.Environ(),
		"BREEZE_TMPL="+tmplJSON,
		"BREEZE_I18N="+i18nJSON,
		"BREEZE_LOCALE="+locale,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSPARuntime_ClientSideTranslation(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	tmpl := `{"badge": "<b>{{t \"home.title\"}}</b> {{t \"home.greeting\" \"name\" .User}} {{t \"cart.items\" \"count\" .Count}}"}`
	dict := `{"home.title": "Velkommen", "home.greeting": "Hej, %{name}!", "cart.items.one": "1 vare", "cart.items.other": "%{count} varer"}`

	test := `
window.breeze.render('badge', { User: 'Alice', Count: 3 }, '#target').then(function () {
	console.log(targetEl.innerHTML);
});`

	got := runSPAHarness(t, tmpl, dict, "da", test)
	want := "<b>Velkommen</b> Hej, Alice! 3 varer"
	if got != want {
		t.Errorf("client render = %q, want %q", got, want)
	}
}

func TestSPARuntime_ClientSidePluralForms(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	tmpl := `{"badge": "{{t \"cart.items\" \"count\" .Count}}"}`
	dict := `{"cart.items.zero": "No items", "cart.items.one": "1 item", "cart.items.other": "%{count} items"}`

	test := `
Promise.resolve()
	.then(function () { return window.breeze.render('badge', { Count: 0 }, '#target'); })
	.then(function (html) { console.log(html); return window.breeze.render('badge', { Count: 1 }, '#target'); })
	.then(function (html) { console.log(html); return window.breeze.render('badge', { Count: 5 }, '#target'); })
	.then(function (html) { console.log(html); });`

	got := runSPAHarness(t, tmpl, dict, "en", test)
	want := "No items\n1 item\n5 items"
	if got != want {
		t.Errorf("plural renders = %q, want %q", got, want)
	}
}

func TestSPARuntime_ClientSideMissingKeyEchoes(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	tmpl := `{"badge": "{{t \"nope.missing\"}}"}`

	test := `
window.breeze.render('badge', {}, '#target').then(function () {
	console.log(targetEl.innerHTML);
});`

	got := runSPAHarness(t, tmpl, `{}`, "en", test)
	if got != "nope.missing" {
		t.Errorf("missing key = %q, want the key echoed", got)
	}
}
