package webtemplate

import (
	"strings"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// sanitizeID normalises a display name into an EHRbase lower-snake web id
// (REQ-106, ADR-0014). Rules reverse-engineered from the reference fixture:
//
//   - lowercase (Unicode-aware; diacritics are KEPT, e.g. "Körper" → "körper");
//   - keep Unicode letters, digits, '.', '-', '_';
//   - replace every other rune with '_', collapsing consecutive '_';
//   - trim leading/trailing '_' (a trailing '.' is kept, e.g. "Anzeichens.");
//   - if the result begins with a digit, prefix 'a' (identifiers may not
//     start with a digit — "24 hour average" → "a24_hour_average").
func sanitizeID(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-':
			b.WriteRune(r)
			prevUnderscore = false
		case r == '_':
			if !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		default:
			if b.Len() > 0 && !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out != "" {
		if r := []rune(out)[0]; unicode.IsDigit(r) {
			out = "a" + out
		}
	}
	return out
}

// termText returns the node's display text in the given language, or "".
func termText(n *templatecompile.CompiledNode, lang string) string {
	if t, ok := n.Term(n.NodeID(), lang); ok {
		return t.Items["text"]
	}
	return ""
}

// idOf resolves a node's web id: its display name if any, else the RM
// attribute name that reached it, else the RM type — all sanitised. The
// fallback applies after sanitisation, so a punctuation-only display
// name (which sanitises to "") still yields a usable FLAT-path segment.
func idOf(n *templatecompile.CompiledNode, attrName string, cfg *config) string {
	return firstNonEmptyID(termText(n, cfg.defaultLanguage), attrName, n.RMTypeName())
}

// firstNonEmptyID returns the first candidate whose sanitised form is
// non-empty, or "".
func firstNonEmptyID(candidates ...string) string {
	for _, c := range candidates {
		if id := sanitizeID(c); id != "" {
			return id
		}
	}
	return ""
}
