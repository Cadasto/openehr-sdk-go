package discovery

import (
	"errors"
	"net/url"
	"strings"
)

// redactURL returns raw with the password in its userinfo masked, the way
// url.URL.Redacted masks it, so that the URL can go into an error or a log
// line. A URL without a password comes back unchanged.
//
// A string url.Parse refuses has no authority it can be trusted to find, and
// its password may hold a "/", "?" or "#" that ends the authority early. For
// such a string everything from the first ":" after "//" to the last "@" is
// masked instead: the scheme, the user name and the host stay, the password
// does not.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err == nil {
		if _, ok := u.User.Password(); ok {
			return u.Redacted()
		}
		return raw
	}
	_, rest, ok := strings.Cut(raw, "//")
	if !ok {
		return raw
	}
	at := strings.LastIndexByte(rest, '@')
	if at < 0 {
		return raw
	}
	colon := strings.IndexByte(rest[:at], ':')
	if colon < 0 {
		return raw
	}
	offset := len(raw) - len(rest)
	return raw[:offset+colon+1] + "xxxxx" + raw[offset+at:]
}

// redactURLError masks the password in the URL that a *url.Error quotes, and
// returns err. It is for an error url.Parse has just returned, directly or
// through http.NewRequestWithContext: url.Parse makes a new *url.Error on
// every call, so changing that one in place affects no other caller.
func redactURLError(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok && ue != nil {
		ue.URL = redactURL(ue.URL)
	}
	return err
}
