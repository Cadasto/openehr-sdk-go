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
	// ErrorDescription is the server's human-readable explanation. On a
	// WireError it is set only when the client is built with
	// WithRawErrorBodies(true), and empty otherwise: it is free text, so it
	// may name a patient or quote a request value, like the message of the
	// openEHR error envelope.
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

// permitsReauth reports whether a 401 carrying c may be answered with one
// Reauth and a retry (REQ-166, REQ-063): only when there is no challenge,
// it names no error, or it names invalid_token, the one RFC 6750 §3.1
// error a new token can fix. A nil receiver is the "no challenge" case.
func (c *BearerChallenge) permitsReauth() bool {
	return c == nil || c.Error == "" || c.Error == "invalid_token"
}

// maxChallengeLineLen is the longest WWW-Authenticate line the parser
// reads (REQ-166). Real Bearer challenges are far shorter; a longer line is
// treated as unparseable without being scanned (RFC 9110 §5.4).
const maxChallengeLineLen = 8 << 10

// parseBearerChallenge returns the first well-formed Bearer challenge in
// the WWW-Authenticate header lines, or nil when there is none (REQ-166).
// A line longer than maxChallengeLineLen is skipped unread; the other lines
// are still read.
func parseBearerChallenge(lines []string) *BearerChallenge {
	for _, line := range lines {
		if len(line) > maxChallengeLineLen {
			continue
		}
		if bc := bearerFromLine(line); bc != nil {
			return bc
		}
	}
	return nil
}

// bearerFromLine returns the first well-formed Bearer challenge on one
// WWW-Authenticate line, or nil when there is none.
//
// The line is a comma-separated list of challenges (RFC 9110 §11.6.1):
//
//	challenge  = auth-scheme [ 1*SP ( token68 / #auth-param ) ]
//	auth-param = token BWS "=" BWS ( token / quoted-string )
//
// Commas separate both challenges and the params inside one, so after each
// comma a lookahead decides: a token followed by "=" is another param of
// the current challenge, any other token starts the next challenge. Empty
// list elements are skipped (RFC 9110 §5.6.1.2).
//
// A line that breaks that grammar anywhere yields nil: once a quoted-string
// or a separator is out of place, the boundaries between challenges cannot
// be trusted, so neither can a Bearer challenge that looked complete before
// the fault. A Bearer challenge that the line grammar allows but RFC 6750
// does not, a token68 or a repeated parameter, is skipped in favour of a
// later one.
//
// Challenges are read one at a time. Only a Bearer challenge is built, and
// only until one is accepted; every other challenge is checked and passed
// over without copying anything, so a line full of them allocates nothing.
// The lookahead rereads at most the one token after a comma, so the work
// is linear in the line length.
func bearerFromLine(line string) *BearerChallenge {
	s := &headerScanner{s: line}
	var found *BearerChallenge
	s.skipSeparators()
	for !s.done() {
		scheme := s.token()
		if scheme == "" {
			return nil
		}
		var b bearerBuilder
		sink := &b
		if found != nil || !strings.EqualFold(scheme, "Bearer") {
			sink = nil
		}
		if !s.challengeBody(sink) {
			return nil
		}
		if sink != nil && !b.rejected {
			bc := b.bc
			found = &bc
		}
		// challengeBody stops at a comma or the end of the line.
		s.skipSeparators()
	}
	return found
}

// bearerBuilder collects the params of one Bearer challenge. Its methods
// accept a nil receiver, which stands for a challenge that is only being
// checked, not built.
type bearerBuilder struct {
	bc       BearerChallenge
	seen     uint8 // the named params already set, one bit each
	rejected bool
}

// rejectToken68 records a token68, which the Bearer scheme does not use.
func (b *bearerBuilder) rejectToken68() {
	if b != nil {
		b.rejected = true
	}
}

// add records one param. raw is the token, or the content of the
// quoted-string still escaped. A name seen before in any letter case
// rejects the challenge: RFC 9110 §11.2 allows each name once per
// challenge, and a repeated error would leave the reauth decision to
// whichever copy won.
func (b *bearerBuilder) add(name, raw string, quoted bool) {
	if b == nil || b.rejected {
		return
	}
	// Names are tokens, so ASCII: EqualFold and ToLower cannot fold a
	// non-ASCII letter onto a known name.
	var field *string
	var bit uint8
	switch {
	case strings.EqualFold(name, "realm"):
		field, bit = &b.bc.Realm, 1<<0
	case strings.EqualFold(name, "error"):
		field, bit = &b.bc.Error, 1<<1
	case strings.EqualFold(name, "error_description"):
		field, bit = &b.bc.ErrorDescription, 1<<2
	case strings.EqualFold(name, "error_uri"):
		field, bit = &b.bc.ErrorURI, 1<<3
	case strings.EqualFold(name, "scope"):
		field, bit = &b.bc.Scope, 1<<4
	default:
		key := strings.ToLower(name)
		if _, dup := b.bc.Params[key]; dup {
			b.rejected = true
			return
		}
		if b.bc.Params == nil {
			b.bc.Params = make(map[string]string)
		}
		b.bc.Params[key] = paramValue(raw, quoted)
		return
	}
	if b.seen&bit != 0 {
		b.rejected = true
		return
	}
	b.seen |= bit
	*field = paramValue(raw, quoted)
}

// paramValue returns a param's value: a token as it stands, a
// quoted-string's content with its quoted-pairs unescaped. raw has been
// checked, so a backslash in it is always followed by the escaped byte.
func paramValue(raw string, quoted bool) string {
	if !quoted || strings.IndexByte(raw, '\\') < 0 {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' {
			i++
		}
		b.WriteByte(raw[i])
	}
	return b.String()
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

// challengeBody reads what follows a scheme, up to the comma that ends the
// challenge or the end of the line. The params go to b, which is nil when
// the challenge is only being checked. It returns false when the line
// breaks the grammar.
func (h *headerScanner) challengeBody(b *bearerBuilder) bool {
	if !h.done() && h.peek() != ',' {
		// The scheme ends at a space, a comma or the end of the line.
		if !isOWS(h.peek()) {
			return false
		}
		h.skipOWS()
	}
	switch {
	case h.done():
		// A scheme with no parameters ends the line.
		return true
	case h.peek() == ',':
		// Empty list elements may open the auth-param list, as in
		// "Bearer , error=…" (RFC 9110 §5.6.1.2). Only an auth-param can
		// follow them there; anything else is the next challenge.
		comma := h.pos
		h.skipSeparators()
		if h.done() || !h.atAuthParam() {
			h.pos = comma
			return true
		}
		return h.authParams(b)
	case h.token68():
		b.rejectToken68()
		return true
	default:
		return h.authParams(b)
	}
}

// token68 consumes a token68 when one fills the rest of the list element,
// that is when only whitespace stands between it and the next comma or the
// end of the line. Otherwise it leaves the cursor alone, so "realm=x" and
// `realm="x"` fall through to the auth-param reading.
func (h *headerScanner) token68() bool {
	i := h.pos
	for i < len(h.s) && isToken68Char(h.s[i]) {
		i++
	}
	if i == h.pos {
		return false
	}
	for i < len(h.s) && h.s[i] == '=' {
		i++
	}
	for i < len(h.s) && isOWS(h.s[i]) {
		i++
	}
	if i < len(h.s) && h.s[i] != ',' {
		return false
	}
	h.pos = i
	return true
}

// authParams consumes the comma-separated auth-params of one challenge,
// handing each to b. It stops at the end of the line, or at a comma whose
// next element starts a new challenge, leaving the cursor on that comma.
// It returns false on a malformed param.
func (h *headerScanner) authParams(b *bearerBuilder) bool {
	for {
		name, raw, quoted, ok := h.authParam()
		if !ok {
			return false
		}
		b.add(name, raw, quoted)
		h.skipOWS()
		if h.done() {
			return true
		}
		if h.peek() != ',' {
			return false
		}
		comma := h.pos
		h.skipSeparators()
		if h.done() {
			return true
		}
		if !h.atAuthParam() {
			h.pos = comma
			return true
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

// authParam consumes one name=value pair. raw is the token, or the content
// of the quoted-string still escaped, and quoted says which; neither is
// copied.
func (h *headerScanner) authParam() (name, raw string, quoted, ok bool) {
	name = h.token()
	if name == "" {
		return "", "", false, false
	}
	h.skipOWS()
	if h.done() || h.peek() != '=' {
		return "", "", false, false
	}
	h.pos++
	h.skipOWS()
	if h.done() {
		return "", "", false, false
	}
	if h.peek() == '"' {
		raw, ok = h.quotedString()
		return name, raw, true, ok
	}
	raw = h.token()
	return name, raw, false, raw != ""
}

// quotedString consumes a quoted-string starting at the opening quote and
// returns its content between the quotes, still escaped. It fails on a
// control character or a line that ends before the closing quote.
func (h *headerScanner) quotedString() (string, bool) {
	h.pos++ // the opening quote
	start := h.pos
	for !h.done() {
		c := h.peek()
		switch {
		case c == '"':
			raw := h.s[start:h.pos]
			h.pos++
			return raw, true
		case c == '\\':
			h.pos++
			if h.done() || !isQuotedPairChar(h.peek()) {
				return "", false
			}
		case !isQdtext(c):
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
