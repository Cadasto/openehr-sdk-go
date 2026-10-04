package smart_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// stubAnswer is what a stub endpoint answers.
type stubAnswer struct {
	status int // 0 answers 200
	body   string
}

// stubRequest is what a stub endpoint received.
type stubRequest struct {
	form       url.Values
	basic      bool
	user, pass string
}

// stubServer is a stub authorization server whose token and revocation
// endpoints answer with what the test sets and record every request they
// receive.
type stubServer struct {
	srv *httptest.Server

	mu         sync.Mutex
	token      stubAnswer
	tokenReqs  []stubRequest
	revoke     stubAnswer
	revokeReqs []stubRequest
}

func newStubServer(t *testing.T) *stubServer {
	t.Helper()
	s := &stubServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user, pass, basic := r.BasicAuth()
		got := stubRequest{form: r.PostForm, basic: basic, user: user, pass: pass}
		s.mu.Lock()
		var ans stubAnswer
		switch r.URL.Path {
		case "/token":
			s.tokenReqs = append(s.tokenReqs, got)
			ans = s.token
		case "/revoke":
			s.revokeReqs = append(s.revokeReqs, got)
			ans = s.revoke
		default:
			s.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if ans.status != 0 {
			w.WriteHeader(ans.status)
		}
		_, _ = w.Write([]byte(ans.body))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// answerToken sets what the token endpoint answers from now on.
func (s *stubServer) answerToken(status int, body string) {
	s.mu.Lock()
	s.token = stubAnswer{status: status, body: body}
	s.mu.Unlock()
}

// tokenRequests returns the requests the token endpoint has received.
func (s *stubServer) tokenRequests() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.tokenReqs...)
}

// answerRevoke sets what the revocation endpoint answers from now on.
func (s *stubServer) answerRevoke(status int, body string) {
	s.mu.Lock()
	s.revoke = stubAnswer{status: status, body: body}
	s.mu.Unlock()
}

// revokeRequests returns the requests the revocation endpoint has received.
func (s *stubServer) revokeRequests() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.revokeReqs...)
}

// endpoints returns the server's endpoints, including its revocation
// endpoint; it publishes no key set.
func (s *stubServer) endpoints() discovery.AuthEndpoints {
	return discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL(s.srv.URL + "/authorize"),
		TokenEndpoint:         discovery.MustParseURL(s.srv.URL + "/token"),
		RevocationEndpoint:    discovery.MustParseURL(s.srv.URL + "/revoke"),
	}
}

// source builds a public-client source on ep, bound to completeIssuer.
func (s *stubServer) source(t *testing.T, ep discovery.AuthEndpoints, opts ...smart.Option) *smart.Source {
	t.Helper()
	src, err := newSource("client-id", ep, append([]smart.Option{
		smart.WithHTTPClient(s.srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithIssuer(completeIssuer),
	}, opts...)...)
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	return src
}
