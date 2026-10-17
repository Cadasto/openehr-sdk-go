package smart_test

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// The credentials the source in these tests holds, each a value the tests
// look for in printed text.
const (
	printSecret  = "CLIENT-SECRET-XYZ"
	printAccess  = "ACCESS-XYZ"
	printRefresh = "REFRESH-XYZ"
)

// printVerbs are the fmt verbs, with flags and widths, that must all print
// the same text for a *smart.Source.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%X", "%t", "%20v", "%-20s", "%+q", "%#x"}

// printedSource returns a confidential-client source that holds the access
// token printAccess, the refresh token printRefresh and a token response
// carrying both, from a refresh it has run.
func printedSource(t *testing.T) *smart.Source {
	t.Helper()
	srv := &scriptedServer{refresh: func(r *http.Request, _ int) (*http.Response, error) {
		return scriptedAnswer(r, http.StatusOK,
			`{"access_token":"`+printAccess+`","refresh_token":"`+printRefresh+`","token_type":"Bearer","expires_in":3600}`), nil
	}}
	src := scriptedSource(t, srv, smart.WithClientSecret(printSecret))
	src.SetTokens(auth.Token{Value: "at-old", Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(-time.Minute)}, "rt-old")
	if tok, err := src.Token(t.Context()); err != nil || tok.Value != printAccess {
		t.Fatalf("Token() = (%q, %v), want (%s, nil)", tok.Value, err, printAccess)
	}
	return src
}

// leakedCredentials returns the credentials of printedSource that out shows.
func leakedCredentials(out string) []string {
	var leaked []string
	for _, c := range []string{printSecret, printAccess, printRefresh} {
		if strings.Contains(out, c) {
			leaked = append(leaked, c)
		}
	}
	return leaked
}

// TestSourcePrintsNoCredentials checks that printing a *smart.Source, with
// any fmt verb, through String or GoString, or nested in another value,
// shows no client secret and no token, and that every fmt verb prints the
// same text as String: the client ID and the issuer. A nil *smart.Source
// prints too, without a panic.
func TestSourcePrintsNoCredentials(t *testing.T) {
	src := printedSource(t)
	stringer, isStringer := any(src).(fmt.Stringer)
	if !isStringer {
		t.Errorf("*smart.Source does not implement fmt.Stringer")
	}
	goStringer, isGoStringer := any(src).(fmt.GoStringer)
	if !isGoStringer {
		t.Errorf("*smart.Source does not implement fmt.GoStringer")
	}
	if _, ok := any(src).(fmt.Formatter); !ok {
		t.Errorf("*smart.Source does not implement fmt.Formatter")
	}
	var want string
	if isStringer {
		want = stringer.String()
		for _, part := range []string{"client-id", "https://idp.test"} {
			if !strings.Contains(want, part) {
				t.Errorf("String() = %q, want it to name %q", want, part)
			}
		}
		if leaked := leakedCredentials(want); len(leaked) > 0 {
			t.Errorf("String() = %q, shows %v", want, leaked)
		}
	}
	if isGoStringer {
		if got := goStringer.GoString(); got != want || len(leakedCredentials(got)) > 0 {
			t.Errorf("GoString() = %q, want %q, the text of String()", got, want)
		}
	}

	for _, verb := range printVerbs {
		out := fmt.Sprintf(verb, src)
		if leaked := leakedCredentials(out); len(leaked) > 0 {
			t.Errorf("fmt.Sprintf(%q, src) shows %v: %s", verb, leaked, out)
		} else if isStringer && out != want {
			t.Errorf("fmt.Sprintf(%q, src) = %q, want %q, the text of String()", verb, out, want)
		}
	}
	nested := map[string]string{
		"Sprint":         fmt.Sprint(src),
		"Sprintln":       fmt.Sprintln(src),
		"slice %v":       fmt.Sprintf("%v", []any{src}),
		"struct %+v":     fmt.Sprintf("%+v", struct{ Source *smart.Source }{src}),
		"struct %#v":     fmt.Sprintf("%#v", struct{ Source *smart.Source }{src}),
		"map value %v":   fmt.Sprintf("%v", map[string]*smart.Source{"k": src}),
		"pointer to %+v": fmt.Sprintf("%+v", &src),
	}
	for name, out := range nested {
		if leaked := leakedCredentials(out); len(leaked) > 0 {
			t.Errorf("%s of src shows %v: %s", name, leaked, out)
		}
	}

	t.Run("nil", func(t *testing.T) {
		var nilSrc *smart.Source
		var nilText string
		if s, ok := any(nilSrc).(fmt.Stringer); ok {
			nilText = s.String()
			if g, ok := any(nilSrc).(fmt.GoStringer); ok && g.GoString() != nilText {
				t.Errorf("GoString() of a nil *smart.Source = %q, want %q, the text of String()", g.GoString(), nilText)
			}
		} else {
			t.Errorf("*smart.Source does not implement fmt.Stringer")
		}
		for _, verb := range printVerbs {
			if out := fmt.Sprintf(verb, nilSrc); out != nilText {
				t.Errorf("fmt.Sprintf(%q, a nil *smart.Source) = %q, want %q, the text of String()", verb, out, nilText)
			}
		}
	})
}

// TestREQ026_PrintingSourceDuringRefreshIsRaceFree pins REQ-026 (idiom.md §
// Concurrency): printing a *smart.Source with any fmt verb while other
// goroutines refresh its tokens reads nothing a refresh writes, so the race
// detector reports nothing and the text shows no credential.
func TestREQ026_PrintingSourceDuringRefreshIsRaceFree(t *testing.T) {
	srv := &scriptedServer{refresh: func(r *http.Request, _ int) (*http.Response, error) {
		return scriptedAnswer(r, http.StatusOK,
			`{"access_token":"`+printAccess+`","refresh_token":"`+printRefresh+`","token_type":"Bearer","expires_in":1}`), nil
	}}
	src := scriptedSource(t, srv, smart.WithClientSecret(printSecret))
	src.SetTokens(auth.Token{Value: "at-old", Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(-time.Minute)}, "rt-old")
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 50 {
			_ = src.Reauth(t.Context())
		}
	})
	for range 50 {
		for _, verb := range printVerbs {
			if out := fmt.Sprintf(verb, src); len(leakedCredentials(out)) > 0 {
				t.Errorf("fmt.Sprintf(%q, src) during a refresh shows a credential: %s", verb, out)
			}
		}
	}
	wg.Wait()
}
