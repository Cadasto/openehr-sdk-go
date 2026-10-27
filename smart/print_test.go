package smart_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart"
)

// redactedMarker is the text the value of a token member prints and logs
// as.
const redactedMarker = "[redacted]"

// printVerbs are the fmt verbs, with flags and widths, that must all print
// the String text of a launch context or a principal.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%t", "%20v", "%-20s"}

// printing is one text a value printed or logged as. how names the way it
// was produced; sameAsString reports that the text must equal the String
// text.
type printing struct {
	how          string
	text         string
	sameAsString bool
}

// printingsOf prints v through String and GoString, with every verb of
// printVerbs, nested in a struct and in a slice, and through log/slog's text
// handler, and also through its JSON handler when withJSON is set. It returns
// the texts and the String text, and names in missing each of fmt.Stringer,
// fmt.GoStringer and fmt.Formatter that v does not implement.
func printingsOf(v any, withJSON bool) (out []printing, stringText string, missing []string) {
	if s, ok := v.(fmt.Stringer); ok {
		stringText = s.String()
		out = append(out, printing{how: "String()", text: stringText})
	} else {
		missing = append(missing, "fmt.Stringer")
	}
	if g, ok := v.(fmt.GoStringer); ok {
		out = append(out, printing{how: "GoString()", text: g.GoString(), sameAsString: true})
	} else {
		missing = append(missing, "fmt.GoStringer")
	}
	if _, ok := v.(fmt.Formatter); !ok {
		missing = append(missing, "fmt.Formatter")
	}
	for _, verb := range printVerbs {
		out = append(out, printing{how: fmt.Sprintf("fmt.Sprintf(%q)", verb), text: fmt.Sprintf(verb, v), sameAsString: true})
	}
	out = append(out,
		printing{how: "a struct field under %+v", text: fmt.Sprintf("%+v", struct{ V any }{v})},
		printing{how: "a slice element under %#v", text: fmt.Sprintf("%#v", []any{v})},
		printing{how: "slog TextHandler", text: logLine(textHandler, v)},
	)
	if withJSON {
		out = append(out, printing{how: "slog JSONHandler", text: logLine(jsonHandler, v)})
	}
	return out, stringText, missing
}

// printedHolder is a launch context or principal as a caller can hold it:
// the pointer the package hands out, or a value copied out of it. withJSON
// reports that log/slog's JSON handler logs it without its tokens too, which
// only the pointer's LogValue gives.
type printedHolder struct {
	name     string
	v        any
	withJSON bool
}

// checkPrinted prints each holder in every way printingsOf does and checks
// each text: it shows none of secrets, carries the marker and every value of
// shown, and, for a verb or GoString, equals the String text.
func checkPrinted(t *testing.T, holders []printedHolder, secrets, shown []string) {
	t.Helper()
	for _, h := range holders {
		out, stringText, missing := printingsOf(h.v, h.withJSON)
		for _, m := range missing {
			t.Errorf("%s (%T) does not implement %s", h.name, h.v, m)
		}
		for _, p := range out {
			t.Run(h.name+"/"+p.how, func(t *testing.T) {
				for _, s := range secrets {
					if strings.Contains(p.text, s) {
						t.Errorf("%s of a %s shows the token %q:\n%s", p.how, h.name, s, p.text)
					}
				}
				for _, want := range append([]string{redactedMarker}, shown...) {
					if !strings.Contains(p.text, want) {
						t.Errorf("%s of a %s = %q, want it to show %q", p.how, h.name, p.text, want)
					}
				}
				if p.sameAsString && stringText != "" && p.text != stringText {
					t.Errorf("%s of a %s = %q, want %q, the text of String()", p.how, h.name, p.text, stringText)
				}
			})
		}
	}
}

// TestREQ060_LaunchContextPrintsNoToken pins REQ-060 for the launch context:
// a *LaunchContext, and a LaunchContext value copied out of it, printed with
// any fmt verb or through log/slog's text handler, show the access_token,
// refresh_token and id_token members of Raw and of Principal's Raw as
// [redacted], keep their keys and show the other fields. The pointer logs
// the same way through the JSON handler. Printing does not change the launch
// context, a nil one prints as <nil>, and one without tokens, principal, ID
// token or banner preference prints those as empty or <nil>.
func TestREQ060_LaunchContextPrintsNoToken(t *testing.T) { // REQ-060, REQ-064
	lc := tokenBodyLaunchContext(t)
	checkPrinted(t,
		[]printedHolder{
			{name: "*LaunchContext", v: lc, withJSON: true},
			{name: "LaunchContext copied out of the pointer", v: *lc},
		},
		[]string{accessSecret, refreshSecret, idTokenSecret},
		[]string{"access_token", "refresh_token", "id_token", "p1", "u1", "PERSON", "openid"},
	)
	if lc.Raw["access_token"] != accessSecret || lc.Raw["id_token"] != idTokenSecret || lc.Principal.Raw["refresh_token"] != refreshSecret {
		t.Errorf("printing changed the LaunchContext: Raw = %v, Principal.Raw[refresh_token] = %v", lc.Raw, lc.Principal.Raw["refresh_token"])
	}

	t.Run("nil", func(t *testing.T) {
		if got := fmt.Sprintf("%+v", (*smart.LaunchContext)(nil)); got != "<nil>" {
			t.Errorf("fmt.Sprintf(%%+v, a nil *LaunchContext) = %q, want <nil>", got)
		}
	})

	t.Run("without tokens", func(t *testing.T) {
		got := fmt.Sprint(smart.LaunchContext{Patient: "p2"})
		if strings.Contains(got, redactedMarker) {
			t.Errorf("fmt.Sprint(a LaunchContext without tokens) = %q, want no %s", got, redactedMarker)
		}
		for _, want := range []string{`Patient: "p2"`, "IDToken: <nil>", "Principal: <nil>", "NeedPatientBanner: <nil>", "Raw: map[]"} {
			if !strings.Contains(got, want) {
				t.Errorf("fmt.Sprint(a LaunchContext without tokens) = %q, want it to show %s", got, want)
			}
		}
	})
}

// TestREQ060_PrincipalIdentityPrintsNoToken pins REQ-060 for the principal:
// a *PrincipalIdentity, and a PrincipalIdentity value copied out of it,
// printed with any fmt verb or through log/slog's text handler, show the
// access_token, refresh_token and id_token members of Raw as [redacted],
// keep their keys and show UID and Type. The pointer logs the same way
// through the JSON handler. Printing does not change the principal, and a nil
// one prints as <nil>.
func TestREQ060_PrincipalIdentityPrintsNoToken(t *testing.T) { // REQ-060, REQ-067
	p := tokenBodyLaunchContext(t).Principal
	p.Raw["id_token"] = idTokenSecret
	checkPrinted(t,
		[]printedHolder{
			{name: "*PrincipalIdentity", v: p, withJSON: true},
			{name: "PrincipalIdentity copied out of the pointer", v: *p},
		},
		[]string{accessSecret, refreshSecret, idTokenSecret},
		[]string{"access_token", "refresh_token", "id_token", "u1", "PERSON"},
	)
	if p.Raw["access_token"] != accessSecret || p.Raw["id_token"] != idTokenSecret {
		t.Errorf("printing changed the PrincipalIdentity: Raw = %v", p.Raw)
	}

	t.Run("nil", func(t *testing.T) {
		if got := fmt.Sprintf("%+v", (*smart.PrincipalIdentity)(nil)); got != "<nil>" {
			t.Errorf("fmt.Sprintf(%%+v, a nil *PrincipalIdentity) = %q, want <nil>", got)
		}
	})
}

// The URLs of urlLaunchContext, each with urlPassword as its password, and
// the text each prints as.
const (
	urlPassword    = "urlpass-s3cr3t"
	urlIssuer      = "https://iss:" + urlPassword + "@idp.example"
	maskedIssuer   = "https://iss:xxxxx@idp.example"
	urlFHIRUser    = "https://user:" + urlPassword + "@fhir.example/Practitioner/1"
	maskedFHIRUser = "https://user:xxxxx@fhir.example/Practitioner/1"
	urlStyle       = "https://style:" + urlPassword + "@app.example/smart-style.json"
	maskedStyle    = "https://style:xxxxx@app.example/smart-style.json"
)

// urlLaunchContext returns a launch context with urlPassword as the password
// of every URL it holds: Issuer, User and SMARTStyleURL, the issuer and
// fhirUser claims of IDToken, the iss, fhirUser and smart_style_url members
// of Raw, and the iss and fhirUser members of Principal's Raw. Raw and
// Principal's Raw also hold an access token.
func urlLaunchContext() *smart.LaunchContext {
	return &smart.LaunchContext{
		Patient:       "p1",
		User:          urlFHIRUser,
		IDToken:       &smart.IDTokenClaims{Subject: "sub-1", Audience: []string{"client-id"}, Issuer: urlIssuer, FHIRUser: urlFHIRUser},
		Issuer:        urlIssuer,
		SMARTStyleURL: urlStyle,
		Principal: &smart.PrincipalIdentity{UID: "u1", Type: smart.PrincipalTypePerson, Raw: map[string]any{
			"access_token": accessSecret, "iss": urlIssuer, "fhirUser": urlFHIRUser, "principal_uid": "u1",
		}},
		Raw: map[string]any{
			"access_token": accessSecret, "iss": urlIssuer, "fhirUser": urlFHIRUser, "smart_style_url": urlStyle, "patient": "p1",
		},
	}
}

// TestREQ060_LaunchContextPrintsNoURLPassword pins REQ-060 for the URLs of a
// launch context and its principal: printed with any fmt verb or through
// log/slog's text handler, and logged as a pointer through its JSON handler,
// they show the password of every URL they hold masked, in the typed fields,
// in the ID token's claims and in the iss, fhirUser and smart_style_url
// members of Raw. Printing changes neither of them.
func TestREQ060_LaunchContextPrintsNoURLPassword(t *testing.T) { // REQ-060
	lc := urlLaunchContext()
	checkPrinted(t,
		[]printedHolder{
			{name: "*LaunchContext", v: lc, withJSON: true},
			{name: "LaunchContext copied out of the pointer", v: *lc},
		},
		[]string{accessSecret, urlPassword},
		[]string{"p1", "sub-1", "u1", maskedIssuer, maskedFHIRUser, maskedStyle},
	)
	checkPrinted(t,
		[]printedHolder{
			{name: "*PrincipalIdentity", v: lc.Principal, withJSON: true},
			{name: "PrincipalIdentity copied out of the pointer", v: *lc.Principal},
		},
		[]string{accessSecret, urlPassword},
		[]string{"u1", "PERSON", maskedIssuer, maskedFHIRUser},
	)
	if !reflect.DeepEqual(lc, urlLaunchContext()) {
		t.Errorf("printing changed the LaunchContext: got %#v", *lc)
	}
}

// TestREQ025_FormatWithNilStateDoesNotPanic pins REQ-025 (idiom.md § No
// panics): Format called directly with a nil fmt.State returns without a
// panic, for the zero value of every type of smart that has a Format
// method.
func TestREQ025_FormatWithNilStateDoesNotPanic(t *testing.T) { // REQ-025
	t.Parallel()
	tests := []struct {
		name string
		v    fmt.Formatter
	}{
		{name: "smart.LaunchContext", v: smart.LaunchContext{}},
		{name: "smart.PrincipalIdentity", v: smart.PrincipalIdentity{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: Format(nil, 'v') panicked: %v", tc.name, r)
				}
			}()
			tc.v.Format(nil, 'v')
		})
	}
}
