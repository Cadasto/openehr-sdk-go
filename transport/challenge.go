package transport

import "strings"

// BearerChallenge is the Bearer challenge a resource server sent in a
// WWW-Authenticate header with a 401 or 403 response (RFC 6750 §3). It is
// how the server says why it refused the token: Error "invalid_token"
// means the token expired, was revoked or is malformed, so a new token may
// succeed; Error "insufficient_scope" means the token lacks a permission,
// so only a new authorization with a wider scope can help.
//
// Parameter names are matched in any letter case. Values are kept exactly
// as the server sent them, with quoted strings unescaped. They come from
// the server unchecked: treat ErrorDescription as untrusted text before
// showing it to a person, and ErrorURI or a URL in Params as untrusted
// before following it.
type BearerChallenge struct {
	// Realm is the protection space the server names, if any.
	Realm string
	// Error is the error code, such as "invalid_token",
	// "insufficient_scope" or "invalid_request". It is empty when the
	// challenge names no error, which is what a server sends when the
	// request carried no token at all.
	Error string
	// ErrorDescription is the server's human-readable explanation.
	ErrorDescription string
	// ErrorURI is the address of a web page about the error.
	ErrorURI string
	// Scope is the scope the server says the request needs, as a
	// space-separated list.
	Scope string
	// Params holds every other parameter of the challenge, keyed by its
	// lower-case name: for example "resource_metadata", the address of the
	// server's protected-resource metadata (RFC 9728). It is nil when the
	// challenge has no other parameters.
	Params map[string]string
}

// parseBearerChallenge returns the first well-formed Bearer challenge in
// the WWW-Authenticate header lines, or nil when there is none (REQ-166).
//
// Each line is a comma-separated list of challenges (RFC 9110 §11.6.1).
// A line that breaks that grammar anywhere is dropped whole: once a
// quoted-string or a separator is out of place, the boundaries between
// challenges cannot be trusted, so neither can a Bearer challenge that
// looked complete before the fault. A Bearer challenge that the line
// grammar allows but RFC 6750 does not, a token68 or a repeated parameter,
// is skipped in favour of a later one.
func parseBearerChallenge(lines []string) *BearerChallenge {
	for _, line := range lines {
		challenges, ok := parseChallenges(line)
		if !ok {
			continue
		}
		for _, ch := range challenges {
			if !strings.EqualFold(ch.scheme, "Bearer") {
				continue
			}
			if bc, ok := ch.bearer(); ok {
				return bc
			}
		}
	}
	return nil
}

// authChallenge is one challenge of a WWW-Authenticate line: the scheme and
// either a token68 or a list of auth-params, in the order they appeared.
type authChallenge struct {
	scheme  string
	token68 string
	params  []authParam
}

type authParam struct {
	name, value string
}

// bearer maps ch onto a BearerChallenge. It refuses a token68, which the
// Bearer scheme does not use, and a parameter that appears twice in any
// letter case: RFC 9110 §11.2 allows each name once per challenge, and a
// repeated error would leave the reauth decision to whichever copy won.
func (ch authChallenge) bearer() (*BearerChallenge, bool) {
	if ch.token68 != "" {
		return nil, false
	}
	bc := &BearerChallenge{}
	seen := make(map[string]bool, len(ch.params))
	for _, p := range ch.params {
		// Names are tokens, so ASCII: ToLower cannot fold a non-ASCII
		// letter onto a known name.
		name := strings.ToLower(p.name)
		if seen[name] {
			return nil, false
		}
		seen[name] = true
		switch name {
		case "realm":
			bc.Realm = p.value
		case "error":
			bc.Error = p.value
		case "error_description":
			bc.ErrorDescription = p.value
		case "error_uri":
			bc.ErrorURI = p.value
		case "scope":
			bc.Scope = p.value
		default:
			if bc.Params == nil {
				bc.Params = make(map[string]string)
			}
			bc.Params[name] = p.value
		}
	}
	return bc, true
}

// parseChallenges splits one WWW-Authenticate line into its challenges per
// RFC 9110 §11.6.1:
//
//	challenge  = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
//	auth-param = token BWS "=" BWS ( token / quoted-string )
//
// Commas separate both challenges and the params inside one, so after each
// comma a lookahead decides: a token followed by "=" is another param of
// the current challenge, any other token starts the next challenge. Empty
// list elements are skipped (RFC 9110 §5.6.1). ok is false when the line
// breaks the grammar. The lookahead rereads at most the one token after a
// comma, so every byte is read a bounded number of times and the cost stays
// linear in the line length whatever the input.
func parseChallenges(line string) (challenges []authChallenge, ok bool) {
	s := &headerScanner{s: line}
	s.skipSeparators()
	for !s.done() {
		ch := authChallenge{scheme: s.token()}
		if ch.scheme == "" {
			return nil, false
		}
		if !s.done() && s.peek() != ',' {
			// The scheme ends at a space, a comma or the end of the line.
			if !isOWS(s.peek()) {
				return nil, false
			}
			s.skipOWS()
		}
		// At a comma or the end of the line the scheme has no parameters.
		if !s.done() && s.peek() != ',' {
			if tok, ok := s.token68(); ok {
				ch.token68 = tok
			} else if ch.params, ok = s.authParams(); !ok {
				return nil, false
			}
		}
		// Each branch above stops at a comma or the end of the line.
		challenges = append(challenges, ch)
		s.skipSeparators()
	}
	return challenges, true
}

// headerScanner walks a header line byte by byte.
type headerScanner struct {
	s   string
	pos int
}

func (h *headerScanner) done() bool { return h.pos >= len(h.s) }

// peek returns the byte at the cursor; the caller checks done first.
func (h *headerScanner) peek() byte { return h.s[h.pos] }

func (h *headerScanner) skipOWS() {
	for !h.done() && isOWS(h.peek()) {
		h.pos++
	}
}

// skipSeparators skips whitespace and commas, which also passes over empty
// list elements.
func (h *headerScanner) skipSeparators() {
	for !h.done() && (isOWS(h.peek()) || h.peek() == ',') {
		h.pos++
	}
}

// token consumes and returns the token at the cursor, or "" when the
// cursor is not on a token character.
func (h *headerScanner) token() string {
	start := h.pos
	for !h.done() && isTchar(h.peek()) {
		h.pos++
	}
	return h.s[start:h.pos]
}

// token68 consumes and returns a token68 when one fills the rest of the
// list element, that is when only whitespace stands between it and the
// next comma or the end of the line. Otherwise it leaves the cursor alone,
// so "realm=x" and `realm="x"` fall through to the auth-param reading.
func (h *headerScanner) token68() (string, bool) {
	i := h.pos
	for i < len(h.s) && isToken68Char(h.s[i]) {
		i++
	}
	if i == h.pos {
		return "", false
	}
	for i < len(h.s) && h.s[i] == '=' {
		i++
	}
	end := i
	for i < len(h.s) && isOWS(h.s[i]) {
		i++
	}
	if i < len(h.s) && h.s[i] != ',' {
		return "", false
	}
	tok := h.s[h.pos:end]
	h.pos = i
	return tok, true
}

// authParams consumes the comma-separated auth-params of one challenge. It
// stops at the end of the line, or at a comma whose next element starts a
// new challenge, leaving the cursor on that comma. ok is false on a
// malformed param.
func (h *headerScanner) authParams() (params []authParam, ok bool) {
	for {
		p, ok := h.authParam()
		if !ok {
			return nil, false
		}
		params = append(params, p)
		h.skipOWS()
		if h.done() {
			return params, true
		}
		if h.peek() != ',' {
			return nil, false
		}
		comma := h.pos
		h.skipSeparators()
		if h.done() {
			return params, true
		}
		if !h.atAuthParam() {
			h.pos = comma
			return params, true
		}
	}
}

// atAuthParam reports whether the cursor is on "token BWS =", without
// moving it.
func (h *headerScanner) atAuthParam() bool {
	i := h.pos
	for i < len(h.s) && isTchar(h.s[i]) {
		i++
	}
	if i == h.pos {
		return false
	}
	for i < len(h.s) && isOWS(h.s[i]) {
		i++
	}
	return i < len(h.s) && h.s[i] == '='
}

// authParam consumes one name=value pair; the value is a token or a
// quoted-string.
func (h *headerScanner) authParam() (authParam, bool) {
	name := h.token()
	if name == "" {
		return authParam{}, false
	}
	h.skipOWS()
	if h.done() || h.peek() != '=' {
		return authParam{}, false
	}
	h.pos++
	h.skipOWS()
	if h.done() {
		return authParam{}, false
	}
	if h.peek() == '"' {
		v, ok := h.quotedString()
		return authParam{name: name, value: v}, ok
	}
	v := h.token()
	return authParam{name: name, value: v}, v != ""
}

// quotedString consumes a quoted-string starting at the opening quote and
// returns its unescaped content. It fails on a control character or a line
// that ends before the closing quote.
func (h *headerScanner) quotedString() (string, bool) {
	h.pos++ // the opening quote
	var b strings.Builder
	for !h.done() {
		c := h.peek()
		switch {
		case c == '"':
			h.pos++
			return b.String(), true
		case c == '\\':
			h.pos++
			if h.done() || !isQuotedPairChar(h.peek()) {
				return "", false
			}
			b.WriteByte(h.peek())
		case isQdtext(c):
			b.WriteByte(c)
		default:
			return "", false
		}
		h.pos++
	}
	return "", false
}

func isOWS(c byte) bool { return c == ' ' || c == '\t' }

// isTchar reports whether c is a token character (RFC 9110 §5.6.2).
func isTchar(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0
}

// isToken68Char reports whether c may appear in a token68 before its
// trailing "=" padding (RFC 9110 §11.2).
func isToken68Char(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-._~+/", c) >= 0
}

// isQdtext reports whether c may appear unescaped in a quoted-string:
// HTAB, SP, visible characters other than '"' and '\', and obs-text
// (RFC 9110 §5.6.4).
func isQdtext(c byte) bool {
	return c == '\t' || c == ' ' || c == 0x21 || (0x23 <= c && c <= 0x5B) || (0x5D <= c && c <= 0x7E) || c >= 0x80
}

// isQuotedPairChar reports whether c may follow a backslash in a
// quoted-string: HTAB, SP, a visible character, or obs-text.
func isQuotedPairChar(c byte) bool {
	return c == '\t' || c == ' ' || (0x21 <= c && c <= 0x7E) || c >= 0x80
}
