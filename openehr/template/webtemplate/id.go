package webtemplate

import (
	"strings"
	"unicode"

	impl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
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

// termText returns the node's display text: the text of the term for its
// node id that the nearest enclosing archetype root defines, or "" when no
// root defines one. A compiled template carries one set of terms, in its
// default language, so the text is in that language.
func termText(n *templatecompile.CompiledNode) string {
	text, _ := impl.TermItem(n, n.NodeID(), "text")
	return text
}

// idOf resolves a node's web id: the template-level node name the OPT pins
// on it (REQ-116) if any, else the archetype concept term, else the RM
// attribute name that reached it, else the RM type — all sanitised. The
// fallback applies after sanitisation, so a punctuation-only display
// name (which sanitises to "") still yields a usable FLAT-path segment.
//
// The pinned name comes first because it is what the reference uses, and
// it is the only thing that distinguishes siblings filling one slot with
// the same archetype: they share a concept term, so deriving from the term
// gave every one of them the same id and hit [ErrIDCollision].
//
// text is the node's term text, as [termText] returns it; the caller passes
// it in because it also names the node.
func idOf(n *templatecompile.CompiledNode, attrName, text string) string {
	return firstNonEmptyID(n.NodeName(), text, attrName, n.RMTypeName())
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
