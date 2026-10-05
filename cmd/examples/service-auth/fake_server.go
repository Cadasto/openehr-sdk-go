package main

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// accessToken is the opaque token the fake issues. A client never looks
// inside an access token; it only sends it back.
const accessToken = "sandbox-opaque-access-token-0001"

// tokenBody is the token endpoint's success answer (RFC 6749 §4.4.3, §5.1).
const tokenBody = `{"access_token":"` + accessToken + `","token_type":"Bearer","expires_in":300}`

// capabilitiesBody answers the System API's OPTIONS /. Its shape follows the
// Options schema of the openEHR System API; the values are made up.
const capabilitiesBody = `{
  "solution": "Example CDR",
  "solution_version": "1.4.2",
  "vendor": "Example vendor",
  "restapi_specs_version": "1.1.0-development",
  "conformance_profile": "STANDARD",
  "endpoints": ["/ehr", "/query", "/definition"]
}`

// fakeServer plays the authorization server and the openEHR server. It is
// example scaffolding, not an SDK feature: the SDK only ever acts as the
// client of an authorization server. It is safe for concurrent use.
type fakeServer struct {
	clientID     string
	clientSecret string
	tokenHits    atomic.Int64
}

// newFakeServer returns a fake that accepts exactly these client credentials.
func newFakeServer(clientID, clientSecret string) *fakeServer {
	return &fakeServer{clientID: clientID, clientSecret: clientSecret}
}

// backend registers the fake's two routes on a sandbox backend, an in-process
// http.RoundTripper. The sandbox matches a route on the full URL path, or on
// the path after "/openehr/v1", so OPTIONS on the REST base arrives as "/".
func (f *fakeServer) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, tokenPath, f.token)
	b.HandleFunc(http.MethodOptions, "/", f.options)
	return b
}

// tokenRequests reports how many times the token endpoint was called.
func (f *fakeServer) tokenRequests() int64 { return f.tokenHits.Load() }

// token answers POST /oauth/token for the client credentials grant. A client
// that authenticated with HTTP Basic and failed gets 401 with a Basic
// challenge (RFC 6749 §5.2).
func (f *fakeServer) token(w http.ResponseWriter, r *http.Request) {
	f.tokenHits.Add(1)
	if !f.validClient(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="sandbox"`)
		writeJSON(w, http.StatusUnauthorized, `{"error":"invalid_client"}`)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":"invalid_request"}`)
		return
	}
	if r.PostForm.Get("grant_type") != "client_credentials" {
		writeJSON(w, http.StatusBadRequest, `{"error":"unsupported_grant_type"}`)
		return
	}
	// A token response is never cached by intermediaries (RFC 6749 §5.1).
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, tokenBody)
}

// validClient checks the HTTP Basic credentials. The client form-encodes the
// id and the secret before Basic encoding (RFC 6749 §2.3.1), so the fake
// decodes them before comparing.
func (f *fakeServer) validClient(r *http.Request) bool {
	rawID, rawSecret, ok := r.BasicAuth()
	if !ok {
		return false
	}
	id, errID := url.QueryUnescape(rawID)
	secret, errSecret := url.QueryUnescape(rawSecret)
	if errID != nil || errSecret != nil {
		return false
	}
	// Run both comparisons before combining them, so a wrong id does not skip
	// the secret comparison and answer sooner than a wrong secret.
	idOK := same(id, f.clientID)
	secretOK := same(secret, f.clientSecret)
	return idOK && secretOK
}

// options answers the System API's OPTIONS /, but only for a request that
// carries the issued token as a bearer token (RFC 6750 §2.1).
func (f *fakeServer) options(w http.ResponseWriter, r *http.Request) {
	if !same(r.Header.Get("Authorization"), "Bearer "+accessToken) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="sandbox"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Allow", http.MethodOptions)
	writeJSON(w, http.StatusOK, capabilitiesBody)
}

// same compares two secrets in constant time, so the time taken does not
// reveal how much of a guess was right.
func same(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// writeJSON sends status and a JSON body.
func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A failed write surfaces on the client side as a decode error.
	_, _ = w.Write([]byte(body))
}
