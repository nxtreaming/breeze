package dashboard

import "strings"

// maskHeaders returns a copy of headers with sensitive values replaced by
// "••••••". The comparison is case-insensitive against cfg.MaskedHeaders.
//
// Used by the dashboard middleware before persisting a RequestRecord so
// secrets never enter the rolling buffer.
func maskHeaders(cfg Config, headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	masked := make(map[string]string, len(headers))
	sensitive := make(map[string]struct{}, len(cfg.MaskedHeaders))
	for _, h := range cfg.MaskedHeaders {
		sensitive[strings.ToLower(h)] = struct{}{}
	}
	for k, v := range headers {
		if _, ok := sensitive[strings.ToLower(k)]; ok {
			masked[k] = "••••••"
		} else {
			masked[k] = v
		}
	}
	return masked
}

// maskLine scans s for tokens that look like secrets (key=value, key:value,
// "key":"value") and replaces their value with "••••••". Used by the log
// recorder so application logs that accidentally print tokens are still
// surfaced but redacted.
func maskLine(cfg Config, s string) string {
	if s == "" {
		return s
	}
	sensitive := make(map[string]struct{}, len(cfg.MaskedHeaders))
	for _, h := range cfg.MaskedHeaders {
		sensitive[strings.ToLower(h)] = struct{}{}
	}
	parts := strings.Fields(s)
	for i, p := range parts {
		key, sep, ok := sensitiveKeyIn(p, sensitive)
		if !ok {
			continue
		}
		head, _ := splitAfterValueStart(p, key, sep)
		parts[i] = head + "••••••" + trailingPunct(p, sep)
	}
	return strings.Join(parts, " ")
}

// sensitiveKeyIn finds a sensitive key at the start of the token (ignoring
// leading JSON punctuation like `{`, `[`, `,`, or a leading quote) and
// reports the separator that followed it ('=' or ':').
func sensitiveKeyIn(token string, sensitive map[string]struct{}) (key string, sep byte, ok bool) {
	lower := strings.ToLower(token)
	start := strings.IndexFunc(lower, func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
	})
	if start < 0 {
		return "", 0, false
	}
	rest := lower[start:]
	for k := range sensitive {
		if !strings.HasPrefix(rest, k) {
			continue
		}
		after := rest[len(k):]
		after = strings.TrimPrefix(after, `"`)
		if strings.HasPrefix(after, "=") {
			return k, '=', true
		}
		if strings.HasPrefix(after, ":") {
			return k, ':', true
		}
	}
	return "", 0, false
}

// splitAfterValueStart returns the token content up through the separator
// (and an opening quote for JSON-style values), so the caller can splice in
// the redacted placeholder.
func splitAfterValueStart(token, key string, sep byte) (head, tail string) {
	lower := strings.ToLower(token)
	idx := strings.Index(lower, key)
	head = token[:idx+len(key)]
	rest := token[idx+len(key):]
	if strings.HasPrefix(rest, `"`) {
		head += `"`
		rest = rest[1:]
	}
	head += string(sep)
	rest = rest[1:]
	if strings.HasPrefix(rest, `"`) {
		head += `"`
		rest = rest[1:]
	}
	return head, rest
}

// trailingPunct returns any trailing quote/brace/comma characters from the
// original token so JSON structure around the redacted value is preserved.
func trailingPunct(token string, sep byte) string {
	if sep != ':' {
		return ""
	}
	i := len(token)
	for i > 0 {
		c := token[i-1]
		if c == '"' || c == '}' || c == ']' || c == ',' {
			i--
			continue
		}
		break
	}
	return token[i:]
}
