package smart_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// sessionLaunchContext is the launch context of the session the tests in
// this file start: a typed parameter, a flag and an array of objects, so a
// copy of the token response has a slice and a map nested in Raw to share.
var sessionLaunchContext = map[string]any{
	"patient":             "P1",
	"need_patient_banner": true,
	"fhirContext": []any{
		map[string]any{"reference": "DiagnosticReport/1"},
		map[string]any{"reference": "Observation/2"},
	},
}

// exchangeLaunchSession completes a code exchange on src whose response
// carries sessionLaunchContext and an ID token with an object claim, org,
// that IDTokenClaims keeps in Extra. It returns what the exchange returned.
func exchangeLaunchSession(t *testing.T, p *oidcProvider, src *smart.Source) smart.TokenResponse {
	t.Helper()
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	claims := idClaims(req.Nonce)
	claims["org"] = map[string]any{"unit": "cardiology"}
	p.setBody(launchBody(t, "at-1", sessionLaunchContext, map[string]any{"refresh_token": "rt-1", "id_token": p.sign(t, claims)}))
	_, tr, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want success", err)
	}
	return tr
}

// refreshLeavingOutTheLaunchContext refreshes src with a response that has
// no ID token and no launch context, so the session keeps the earlier ones.
func refreshLeavingOutTheLaunchContext(t *testing.T, p *oidcProvider, src *smart.Source) {
	t.Helper()
	src.SetTokens(staleAccess("at-1"), "rt-1")
	p.setBody(launchBody(t, "at-2", nil, map[string]any{"refresh_token": "rt-2"}))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() error = %v, want a refreshed token", err)
	}
}

// changeResponse changes everything a holder of tr can reach through it: a
// member of Raw, a key it adds, an object and an element of the array
// nested in Raw, the flag NeedPatientBanner points to, and the claims, with
// their audience and an object nested in their Extra.
func changeResponse(tr smart.TokenResponse) {
	if tr.Raw != nil {
		tr.Raw["patient"] = "changed"
		tr.Raw["added"] = true
		if ctx, ok := tr.Raw["fhirContext"].([]any); ok && len(ctx) == 2 {
			if ref, ok := ctx[0].(map[string]any); ok {
				ref["reference"] = "changed"
			}
			ctx[1] = "changed"
		}
	}
	if tr.NeedPatientBanner != nil {
		*tr.NeedPatientBanner = false
	}
	if c := tr.IDTokenClaims; c != nil {
		c.Subject = "changed"
		if len(c.Audience) > 0 {
			c.Audience[0] = "changed"
		}
		if org, ok := c.Extra["org"].(map[string]any); ok {
			org["unit"] = "changed"
		}
		if c.Extra != nil {
			c.Extra["added"] = true
		}
	}
}

// checkSessionResponse reports each value of tr, the session's last token
// response, that is not what exchangeLaunchSession gave the session.
func checkSessionResponse(t *testing.T, when string, tr smart.TokenResponse) {
	t.Helper()
	if tr.Patient != "P1" || tr.Raw["patient"] != "P1" {
		t.Errorf("%s: LastTokenResponse() Patient = %q, Raw[patient] = %v; want P1 for both", when, tr.Patient, tr.Raw["patient"])
	}
	if _, ok := tr.Raw["added"]; ok {
		t.Errorf("%s: LastTokenResponse().Raw has the key a holder added to its copy", when)
	}
	ctx, _ := tr.Raw["fhirContext"].([]any)
	if len(ctx) != 2 {
		t.Errorf("%s: LastTokenResponse().Raw[fhirContext] = %v, want two references", when, tr.Raw["fhirContext"])
	} else {
		first, _ := ctx[0].(map[string]any)
		second, _ := ctx[1].(map[string]any)
		if first["reference"] != "DiagnosticReport/1" || second["reference"] != "Observation/2" {
			t.Errorf("%s: LastTokenResponse().Raw[fhirContext] = %v, want DiagnosticReport/1 and Observation/2", when, ctx)
		}
	}
	if got := bannerValue(tr); got != true {
		t.Errorf("%s: LastTokenResponse().NeedPatientBanner points to %v, want true", when, got)
	}
	c := tr.IDTokenClaims
	if c == nil {
		t.Errorf("%s: LastTokenResponse().IDTokenClaims = nil, want the session's verified claims", when)
		return
	}
	if c.Subject != "user-1" || !slices.Equal(c.Audience, []string{"client-id"}) {
		t.Errorf("%s: LastTokenResponse() claims sub = %q, aud = %v; want user-1, [client-id]", when, c.Subject, c.Audience)
	}
	if org, _ := c.Extra["org"].(map[string]any); org["unit"] != "cardiology" {
		t.Errorf("%s: LastTokenResponse() claims Extra[org] = %v, want unit cardiology", when, c.Extra["org"])
	}
	if _, ok := c.Extra["added"]; ok {
		t.Errorf("%s: LastTokenResponse() claims Extra has the key a holder added to its copy", when)
	}
}

// TestREQ026_TokenResponsesHandedOutAreTheCallersOwn pins REQ-026 (idiom.md
// § Concurrency): the token response ExchangeAuthorizationCode returns and
// each one LastTokenResponse returns is the caller's own copy, down to the
// values nested in Raw, the ID-token claims and the banner flag. A caller
// changing any of them changes neither the source nor a later result.
func TestREQ026_TokenResponsesHandedOutAreTheCallersOwn(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	tr := exchangeLaunchSession(t, p, src)
	checkSessionResponse(t, "the code exchange's result", tr)

	changeResponse(tr)
	checkSessionResponse(t, "after the caller changed the code exchange's result", src.LastTokenResponse())

	changeResponse(src.LastTokenResponse())
	checkSessionResponse(t, "after the caller changed a LastTokenResponse result", src.LastTokenResponse())

	// After a refresh that leaves them out, the session keeps the launch
	// context and the claims, and a caller still gets its own copy of them.
	refreshLeavingOutTheLaunchContext(t, p, src)
	checkSessionResponse(t, "after a refresh", src.LastTokenResponse())
	changeResponse(src.LastTokenResponse())
	checkSessionResponse(t, "after the caller changed a LastTokenResponse result after a refresh", src.LastTokenResponse())
}

// TestREQ026_TokenChangeResponseIsTheHooksOwnAllTheWayDown pins REQ-026
// (idiom.md § Concurrency) with REQ-063: the token response the
// token-change hook gets is its own copy, down to the values nested in Raw,
// the ID-token claims and the banner flag, after a code exchange and after a
// refresh. A hook changing any of them does not change the source.
func TestREQ026_TokenChangeResponseIsTheHooksOwnAllTheWayDown(t *testing.T) {
	t.Parallel()
	p := newOIDCProvider(t)
	var calls int
	src := p.source(t, p.endpoints(), smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
		calls++
		changeResponse(c.Response)
	}))
	exchangeLaunchSession(t, p, src)
	checkSessionResponse(t, "after the hook changed the code exchange's change", src.LastTokenResponse())

	refreshLeavingOutTheLaunchContext(t, p, src)
	checkSessionResponse(t, "after the hook changed the refresh's change", src.LastTokenResponse())
	if calls != 2 {
		t.Errorf("token-change hook calls = %d, want 2: the code exchange and the refresh", calls)
	}
}

// TestREQ026_ChangingATokenResponseDoesNotRaceARefresh pins REQ-026
// (idiom.md § Concurrency): a caller reading and changing the token
// responses it gets from LastTokenResponse, and a token-change hook doing
// the same with its own, while refreshes run, share nothing with the
// source. Run under -race, it reports no data race.
func TestREQ026_ChangingATokenResponseDoesNotRaceARefresh(t *testing.T) {
	t.Parallel()
	srv := &scriptedServer{refresh: func(r *http.Request, n int) (*http.Response, error) {
		// Every other response leaves the launch context out, so the
		// session keeps the earlier one, nested values included.
		launch := `,"patient":"P1","need_patient_banner":true,"fhirContext":[{"reference":"DiagnosticReport/1"},{"reference":"Observation/2"}]`
		if n%2 == 0 {
			launch = ""
		}
		// A one-second token is inside the 30-second refresh window, so
		// every Token call refreshes.
		return scriptedAnswer(r, http.StatusOK,
			fmt.Sprintf(`{"access_token":"at-%d","token_type":"Bearer","expires_in":1,"refresh_token":"rt-%d"%s}`, n, n, launch)), nil
	}}
	src := scriptedSource(t, srv, smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
		_ = fmt.Sprint(c.Response)
		changeResponse(c.Response)
	}))
	src.SetTokens(staleAccess("at-0"), "rt-0")
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("first Token() error = %v, want a refreshed token", err)
	}

	const rounds = 100
	var wg sync.WaitGroup
	wg.Go(func() {
		for range rounds {
			if _, err := src.Token(t.Context()); err != nil {
				t.Errorf("Token() error = %v, want a refreshed token", err)
				return
			}
		}
	})
	wg.Go(func() {
		for range rounds {
			tr := src.LastTokenResponse()
			_ = fmt.Sprint(tr)
			changeResponse(tr)
		}
	})
	wg.Wait()

	if last := src.LastTokenResponse(); last.Patient != "P1" || last.Raw["patient"] != "P1" {
		t.Errorf("LastTokenResponse() Patient = %q, Raw[patient] = %v; want P1 for both: no holder's change reaches the source", last.Patient, last.Raw["patient"])
	}
}

// copiedReferenceFields are the fields of TokenResponse and IDTokenClaims
// that can share memory with a copy of the struct that holds them: a map,
// slice, pointer, interface, channel or function. The source's copy of a
// token response copies each one deeply, and changeResponse and
// checkSessionResponse check each one by name.
var copiedReferenceFields = []string{
	"TokenResponse.IDTokenClaims",
	"TokenResponse.NeedPatientBanner",
	"TokenResponse.Raw",
	"IDTokenClaims.Audience",
	"IDTokenClaims.Extra",
}

// TestREQ026_TokenResponseCopyCoversEveryReferenceField pins REQ-026
// (idiom.md § Concurrency) for fields added later: it walks TokenResponse
// and, through its IDTokenClaims pointer, IDTokenClaims, and fails on each
// field that can share memory with a copy and that copiedReferenceFields
// does not list. A map, slice or pointer added to either struct then fails
// this test by name, instead of being shared between the source and the
// copies it hands out. A listed field the walk no longer finds fails it
// too, so the list stays true.
func TestREQ026_TokenResponseCopyCoversEveryReferenceField(t *testing.T) { // REQ-026
	t.Parallel()
	listed := make(map[string]bool, len(copiedReferenceFields))
	for _, name := range copiedReferenceFields {
		listed[name] = false
	}
	walked := make(map[reflect.Type]bool)
	var walk func(owner string, typ reflect.Type)
	walk = func(owner string, typ reflect.Type) {
		if walked[typ] {
			return
		}
		walked[typ] = true
		for field := range typ.Fields() {
			name := owner + "." + field.Name
			switch ft := field.Type; {
			case ft == reflect.TypeFor[time.Time]():
				// A time.Time is a value. The *Location it holds is never
				// changed once made, so a copy of it shares nothing that
				// changes.
			case ft.Kind() == reflect.Struct:
				walk(name, ft)
			case sharesMemory(ft):
				if _, ok := listed[name]; !ok {
					t.Errorf("%s is a %s, which a copy of a token response would share with the source: copy it deeply in cloneTokenResponse, change and check it in changeResponse and checkSessionResponse, and list it in copiedReferenceFields", name, ft)
					continue
				}
				listed[name] = true
				if ft.Kind() == reflect.Pointer && ft.Elem().Kind() == reflect.Struct {
					walk(ft.Elem().Name(), ft.Elem())
				}
			}
		}
	}
	walk("TokenResponse", reflect.TypeFor[smart.TokenResponse]())
	for _, name := range copiedReferenceFields {
		if !listed[name] {
			t.Errorf("copiedReferenceFields lists %s, which the walk did not find: drop it from the list", name)
		}
	}
}

// sharesMemory reports whether a value of type typ can share memory with a
// copy of it: a map, slice, pointer, interface, channel or function, or an
// array or struct that holds one.
func sharesMemory(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Chan, reflect.Func:
		return true
	case reflect.Array:
		return sharesMemory(typ.Elem())
	case reflect.Struct:
		if typ == reflect.TypeFor[time.Time]() {
			return false
		}
		for field := range typ.Fields() {
			if sharesMemory(field.Type) {
				return true
			}
		}
	}
	return false
}
