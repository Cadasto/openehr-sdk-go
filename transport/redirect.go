package transport

import (
	"fmt"
	"net/http"
)

// maxRedirects is net/http's default redirect limit, applied when the
// injected client has no CheckRedirect of its own.
const maxRedirects = 10

// httpClientFor returns the client doOnce sends httpReq with. A request
// carrying an Authorization header gets a fresh shallow copy of the injected
// client whose redirect policy refuses an https to non-https hop; any other
// request gets the injected client as it is.
//
// REQ-092, REQ-021: the copy is taken per request, never at New, so a change
// the caller makes to the injected client later (its Transport,
// CheckRedirect, Timeout or Jar) reaches the next request. The injected
// client itself is never written to.
func (c *Client) httpClientFor(httpReq *http.Request) *http.Client {
	if len(httpReq.Header.Values("Authorization")) == 0 {
		return c.cfg.httpClient
	}
	return refuseDowngrade(c.cfg.httpClient)
}

// refuseDowngrade returns a shallow copy of hc whose redirect policy refuses a
// redirect from an https URL to a URL that is not https, with
// ErrInsecureRedirect, and otherwise applies hc's own CheckRedirect, or
// net/http's default limit when hc has none. It mirrors the discovery
// resolver's refuseDowngrade and internal/noredirect's copy idiom.
//
// net/http copies an Authorization header to a redirect target on the same
// host or a subdomain whatever its scheme, so without this check a 3xx from
// https://host to http://host would carry the token in clear text. The hop
// is judged from the request just before this one (via's last entry), so an
// https hop anywhere in the chain is protected.
func refuseDowngrade(hc *http.Client) *http.Client {
	cp := *hc
	callerPolicy := hc.CheckRedirect
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return ErrInsecureRedirect
		}
		if callerPolicy != nil {
			return callerPolicy(req, via)
		}
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return nil
	}
	return &cp
}
