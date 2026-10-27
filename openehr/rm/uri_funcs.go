package rm

// REQ-120 — URI reference components.
//
// Hand-written bodies for DV_URI's scheme, path, query and fragment_id,
// whose generated stubs are suppressed (manual_impl.go). DV_EHR_URI embeds
// DV_URI, so it answers them too. The validation floor's DV_EHR_URI
// Scheme_valid rule reads Scheme, so the floor and these functions agree on
// what a scheme is. See docs/specifications/rm-functions.md § REQ-120.

import "strings"

// Scheme returns the scheme of the URI reference in Value: the text before
// the first colon, when that text is a scheme as RFC 3986 section 3.1 writes
// one, a letter followed by letters, digits, "+", "-" or ".". The scheme
// keeps the letter case it is written in. Scheme returns "" for a nil
// receiver and for a reference with no scheme, such as a relative reference,
// a path with a colon after a slash, or text that starts with a digit.
func (d *DVURI) Scheme() string { return d.parts().scheme }

// Path returns the path of the URI reference in Value, as RFC 3986 section 3
// splits the reference: the text after the scheme and its colon, and after
// the authority (a "//" and the text up to the next "/"), up to the query or
// the fragment. So "ehr://sys/7d44/compositions" has the path
// "/7d44/compositions", and "mailto:a@example.org" has "a@example.org". Path
// returns "" for a nil receiver and for a reference with no path.
func (d *DVURI) Path() string { return d.parts().path }

// Query returns the query of the URI reference in Value: the text after the
// first "?" and before the fragment, without the "?". Query returns "" for a
// nil receiver and for a reference with no query or an empty one.
func (d *DVURI) Query() string { return d.parts().query }

// FragmentID returns the fragment of the URI reference in Value: the text
// after the first "#", without the "#". FragmentID returns "" for a nil
// receiver and for a reference with no fragment or an empty one.
func (d *DVURI) FragmentID() string { return d.parts().fragment }

// uriParts holds the components of a URI reference that DV_URI exposes. Each
// is the text as written, with no percent-decoding, so a plain-text URI,
// which the RM admits, splits as its encoded form would.
type uriParts struct {
	scheme, path, query, fragment string
}

// parts splits the reference in d.Value as RFC 3986 appendix B does, with the
// scheme held to the grammar of section 3.1. The scheme is cut first, at the
// first colon: a scheme holds no "/", "?" or "#", so text before a colon that
// follows one of them is no scheme. The fragment is then cut at the first
// "#", the query at the first "?" before it, and an authority, "//" and the
// text up to the next "/", is dropped from the front of what is left.
func (d *DVURI) parts() uriParts {
	if d == nil {
		return uriParts{}
	}
	var p uriParts
	rest := d.Value
	if scheme, after, found := strings.Cut(rest, ":"); found && isURIScheme(scheme) {
		p.scheme, rest = scheme, after
	}
	rest, p.fragment, _ = strings.Cut(rest, "#")
	rest, p.query, _ = strings.Cut(rest, "?")
	if hier, found := strings.CutPrefix(rest, "//"); found {
		rest = ""
		if i := strings.IndexByte(hier, '/'); i >= 0 {
			rest = hier[i:]
		}
	}
	p.path = rest
	return p
}

// isURIScheme reports whether s is a scheme as RFC 3986 section 3.1 writes
// one: `ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )`.
func isURIScheme(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		case i > 0 && ('0' <= c && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return true
}
