package middleware

import (
	"strings"

	"github.com/nelthaarion/breeze"
)

// LocaleCookieName is the cookie used to persist an explicit language choice.
const LocaleCookieName = "breeze_locale"

// LocaleMiddleware resolves the request locale and attaches it (and the
// bundle) to the context so templates and ctx.T translate correctly.
//
// Resolution order:
//  1. ?lang= query parameter — also persists the choice as a cookie, so a
//     plain <a href="?lang=da"> works as a language switcher
//  2. breeze_locale cookie
//  3. Accept-Language header (q-value aware, primary-subtag fallback)
//  4. the bundle's default locale
//
// A locale from steps 1–2 is only honored when it is actually loaded, so a
// forged cookie or query value cannot select a nonexistent language.
//
// The middleware sets Content-Language (and Set-Cookie when persisting)
// after ctx.Next(): handler body methods (JSON/HTML/WriteString) replace
// ctx.Res entirely, so headers set before the handler would be lost.
func LocaleMiddleware(i18n *breeze.I18n) breeze.HandlerFunc {
	return func(ctx *breeze.Context) {
		locale := ""
		persist := false

		if lang := ctx.Query("lang"); lang != "" && i18n.HasLocale(lang) {
			locale = lang
			persist = true
		}

		if locale == "" {
			if c := cookieValue(ctx.Req.Header["cookie"], LocaleCookieName); c != "" && i18n.HasLocale(c) {
				locale = c
			}
		}

		if locale == "" {
			locale = i18n.NegotiateLocale(ctx.Req.Header["accept-language"])
		}

		if locale == "" {
			locale = i18n.DefaultLocale()
		}

		ctx.SetLocale(locale)
		ctx.SetI18n(i18n)

		ctx.Next()

		ctx.SetHeader("Content-Language", locale)
		if persist {
			ctx.SetHeader("Set-Cookie",
				LocaleCookieName+"="+locale+"; Path=/; Max-Age=31536000; SameSite=Lax")
		}
	}
}

// cookieValue extracts a single cookie value from a raw Cookie header.
func cookieValue(header, name string) string {
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+"=") {
			return part[len(name)+1:]
		}
	}
	return ""
}
