// Package urlredact masks the password in a URL's userinfo, so that a URL
// the caller configured or the SDK discovered can go into an error, a panic
// or a log line without the password.
//
// [String] masks a URL string, and [Error] masks an error url.Parse returned
// for one: both the URL its *url.Error quotes and what its cause quotes from
// the password. The masked text still names the scheme, the user name and
// the host.
package urlredact

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// mask is the text that stands in for a password or for a part of one.
const mask = "xxxxx"

// String returns raw with the password in its userinfo masked, the way
// url.URL.Redacted masks it, so that the URL can go into an error or a log
// line. A URL without a password comes back unchanged.
//
// When url.Parse accepts raw, a "/", "?" or "#" ends the authority, so what
// follows it is path, query or fragment, not password, and String does not
// mask it: "https://svc:1234/secret@x.example/token" has the host
// "svc:1234" and no userinfo, and comes back unchanged. url.URL.Redacted
// rebuilds the URL it masks, so the rest of the text can come back
// re-escaped.
//
// A string url.Parse refuses has no authority it can be trusted to find, and
// its password may hold a "/", "?" or "#" that made url.Parse refuse it. For
// such a string everything from the first ":" after "//" to the last "@" is
// masked instead, and the rest of the string is kept as it was.
func String(raw string) string {
	u, err := url.Parse(raw)
	if err == nil {
		if _, ok := u.User.Password(); ok {
			return u.Redacted()
		}
		return raw
	}
	start, end, ok := passwordSpan(raw)
	if !ok {
		return raw
	}
	return raw[:start] + mask + raw[end:]
}

// Error masks the password of raw in err and returns err. err is an error
// url.Parse returned for raw, directly or through
// http.NewRequestWithContext, and raw is the string url.Parse was given.
//
// The URL that err's *url.Error quotes becomes String(raw). url.Parse cuts
// that URL at the first "#", so a "#" in the password would otherwise leave
// the part before it in clear text.
//
// url.Parse's cause can quote the password too. A "/", "?" or "#" in the
// password ends the authority early, and the cause quotes the part before
// it as a port. A bad escape in the password is quoted as well. Error masks
// each value the cause quotes that can come from the password: a value that
// overlaps the text String masks for a refused string, or a value that raw
// does not hold as written. errors.Is, errors.As and errors.AsType still
// reach the original cause, and Op is unchanged.
//
// url.Parse makes a new *url.Error on every call, so changing that one in
// place affects no other caller.
func Error(err error, raw string) error {
	ue, ok := errors.AsType[*url.Error](err)
	if !ok || ue == nil {
		return err
	}
	ue.URL = String(raw)
	start, end, ok := passwordSpan(raw)
	if !ok || ue.Err == nil {
		return err
	}
	text := ue.Err.Error()
	if masked := maskQuoted(text, raw, start, end); masked != text {
		ue.Err = &maskedCause{text: masked, err: ue.Err}
	}
	return err
}

// maskedCause is url.Parse's cause with the text Error masked.
type maskedCause struct {
	text string
	err  error
}

func (c *maskedCause) Error() string { return c.text }

func (c *maskedCause) Unwrap() error { return c.err }

// passwordSpan returns where the password of raw lies when url.Parse
// refuses it: from the first ":" after "//" to the last "@". ok is false
// when raw has no such span.
func passwordSpan(raw string) (start, end int, ok bool) {
	_, rest, found := strings.Cut(raw, "//")
	if !found {
		return 0, 0, false
	}
	at := strings.LastIndexByte(rest, '@')
	if at < 0 {
		return 0, 0, false
	}
	colon := strings.IndexByte(rest[:at], ':')
	if colon < 0 {
		return 0, 0, false
	}
	offset := len(raw) - len(rest)
	return offset + colon + 1, offset + at, true
}

// maskQuoted returns text with each double-quoted value in it masked when
// fromPassword says the value can come from raw[start:end].
func maskQuoted(text, raw string, start, end int) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(text, '"')
		if i < 0 {
			break
		}
		quoted, err := strconv.QuotedPrefix(text[i:])
		if err != nil {
			b.WriteString(text[:i+1])
			text = text[i+1:]
			continue
		}
		b.WriteString(text[:i])
		// QuotedPrefix accepted quoted, so Unquote does too; were it to
		// refuse, the value is masked rather than kept.
		if v, err := strconv.Unquote(quoted); err != nil || fromPassword(v, raw, start, end) {
			b.WriteString(strconv.Quote(mask))
		} else {
			b.WriteString(quoted)
		}
		text = text[i+len(quoted):]
	}
	b.WriteString(text)
	return b.String()
}

// fromPassword reports whether v, a value url.Parse's cause quotes, can come
// from raw[start:end]: some place raw holds v overlaps that span, or raw does
// not hold v as written at all, as when url.Parse quotes text it unescaped.
func fromPassword(v, raw string, start, end int) bool {
	if v == "" {
		return false
	}
	held := false
	for i := 0; ; {
		j := strings.Index(raw[i:], v)
		if j < 0 {
			break
		}
		at := i + j
		if at < end && at+len(v) > start {
			return true
		}
		held = true
		i = at + 1
	}
	return !held
}
