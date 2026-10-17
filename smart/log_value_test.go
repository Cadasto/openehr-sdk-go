package smart_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

// The secrets a token-endpoint body carries in the tests below.
const (
	accessSecret  = "AT-SECRET-4d1c"
	refreshSecret = "RT-SECRET-9b07"
	idTokenSecret = "IDT-SECRET-77e2"
)

// tokenBodyLaunchContext builds a LaunchContext from a token-endpoint body
// that carries an access and a refresh token and the principal claims, so
// the principal is read from the body and its Raw holds the tokens too.
func tokenBodyLaunchContext(t *testing.T) *smart.LaunchContext {
	t.Helper()
	tr, err := authsmart.ParseTokenResponse([]byte(`{"access_token":"` + accessSecret + `","refresh_token":"` + refreshSecret +
		`","token_type":"Bearer","scope":"launch openid","patient":"p1","need_patient_banner":false,"principal_uid":"u1","principal_type":"PERSON"}`))
	if err != nil {
		t.Fatalf("ParseTokenResponse: %v", err)
	}
	lc, err := smart.LaunchContextFromTokenResponse(t.Context(), tr)
	if err != nil {
		t.Fatalf("LaunchContextFromTokenResponse: %v", err)
	}
	if lc.Principal == nil || lc.Raw["refresh_token"] != refreshSecret || lc.Principal.Raw["access_token"] != accessSecret {
		t.Fatalf("fixture: LaunchContext = %+v, want a principal and both tokens on Raw and on Principal.Raw", lc)
	}
	// The body cannot carry a verified id_token without a key set; Raw holds
	// one verbatim when it does.
	lc.Raw["id_token"] = idTokenSecret
	return lc
}

// logLine writes one record with the attribute "v" through h and returns
// what h wrote.
func logLine(newHandler func(*bytes.Buffer) slog.Handler, v any) string {
	var buf bytes.Buffer
	slog.New(newHandler(&buf)).Info("launch", "v", v)
	return buf.String()
}

func jsonHandler(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) }
func textHandler(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) }

// TestREQ064_LaunchContextLogValueMasksTokens logs a launch context, as a
// value and as a pointer, through slog's JSON and text handlers: the tokens
// on Raw and on Principal.Raw are masked, the members stay, and the typed
// fields are logged.
func TestREQ064_LaunchContextLogValueMasksTokens(t *testing.T) {
	lc := tokenBodyLaunchContext(t)
	handlers := []struct {
		name string
		new  func(*bytes.Buffer) slog.Handler
	}{{"json", jsonHandler}, {"text", textHandler}}
	values := []struct {
		name string
		v    any
	}{{"pointer", lc}, {"value", *lc}}
	for _, h := range handlers {
		for _, v := range values {
			t.Run(h.name+"/"+v.name, func(t *testing.T) {
				out := logLine(h.new, v.v)
				for _, secret := range []string{accessSecret, refreshSecret, idTokenSecret} {
					if strings.Contains(out, secret) {
						t.Errorf("log of a %s LaunchContext holds %q:\n%s", v.name, secret, out)
					}
				}
				for _, want := range []string{"access_token", "refresh_token", "id_token", "p1", "u1", "PERSON"} {
					if !strings.Contains(out, want) {
						t.Errorf("log of a %s LaunchContext lacks %q:\n%s", v.name, want, out)
					}
				}
				if strings.Contains(out, "LogValue panicked") {
					t.Errorf("log of a %s LaunchContext: LogValue panicked:\n%s", v.name, out)
				}
			})
		}
	}
	if lc.Raw["access_token"] != accessSecret || lc.Principal.Raw["refresh_token"] != refreshSecret {
		t.Errorf("logging changed the LaunchContext: Raw[access_token] = %v, Principal.Raw[refresh_token] = %v", lc.Raw["access_token"], lc.Principal.Raw["refresh_token"])
	}
}

// TestREQ064_LaunchContextLogValueShape reads the JSON log of a launch
// context back: every token member keeps its key with the same marker in
// place of its value, the other members and the typed fields keep their
// values, and a nil principal and a nil ID token log as null.
func TestREQ064_LaunchContextLogValueShape(t *testing.T) {
	lc := tokenBodyLaunchContext(t)
	var rec struct {
		V struct {
			Patient           string
			Scopes            []string
			NeedPatientBanner *bool
			IDToken           any
			Principal         struct {
				UID  string
				Type string
				Raw  map[string]any
			}
			Raw map[string]any
		}
	}
	if err := json.Unmarshal([]byte(logLine(jsonHandler, lc)), &rec); err != nil {
		t.Fatalf("JSON log line does not parse: %v", err)
	}
	got := rec.V
	marker, ok := got.Raw["access_token"].(string)
	if !ok || marker == "" {
		t.Fatalf("Raw[access_token] logged as %#v, want a marker string", got.Raw["access_token"])
	}
	for _, raw := range []map[string]any{got.Raw, got.Principal.Raw} {
		for _, k := range []string{"access_token", "refresh_token"} {
			if raw[k] != marker {
				t.Errorf("logged %s = %#v, want the marker %q", k, raw[k], marker)
			}
		}
		if raw["patient"] != "p1" || raw["token_type"] != "Bearer" {
			t.Errorf("logged Raw = %v, want patient p1 and token_type Bearer kept", raw)
		}
	}
	if got.Raw["id_token"] != marker {
		t.Errorf("logged Raw[id_token] = %#v, want the marker %q", got.Raw["id_token"], marker)
	}
	if got.Patient != "p1" || len(got.Scopes) != 2 || got.NeedPatientBanner == nil || *got.NeedPatientBanner {
		t.Errorf("logged typed fields = %+v, want Patient p1, 2 scopes, NeedPatientBanner false", got)
	}
	if got.Principal.UID != "u1" || got.Principal.Type != "PERSON" {
		t.Errorf("logged Principal = %+v, want UID u1, Type PERSON", got.Principal)
	}
	if got.IDToken != nil {
		t.Errorf("logged IDToken = %#v, want null without an ID token", got.IDToken)
	}

	bare := smart.LaunchContext{Patient: "p2"}
	var bareRec struct{ V map[string]any }
	if err := json.Unmarshal([]byte(logLine(jsonHandler, bare)), &bareRec); err != nil {
		t.Fatalf("JSON log line does not parse: %v", err)
	}
	if p, ok := bareRec.V["Principal"]; !ok || p != nil {
		t.Errorf("logged Principal of a context without one = %#v (present %v), want null", p, ok)
	}
	if bareRec.V["Patient"] != "p2" {
		t.Errorf("logged Patient = %#v, want p2", bareRec.V["Patient"])
	}
}

// TestREQ067_PrincipalIdentityLogValueMasksTokens logs a principal read
// from a token-endpoint body on its own, as a value and as a pointer: its
// Raw holds the tokens, and the log masks them.
func TestREQ067_PrincipalIdentityLogValueMasksTokens(t *testing.T) {
	p := tokenBodyLaunchContext(t).Principal
	for _, v := range []struct {
		name string
		v    any
	}{{"pointer", p}, {"value", *p}} {
		for _, h := range []func(*bytes.Buffer) slog.Handler{jsonHandler, textHandler} {
			out := logLine(h, v.v)
			for _, secret := range []string{accessSecret, refreshSecret} {
				if strings.Contains(out, secret) {
					t.Errorf("log of a %s PrincipalIdentity holds %q:\n%s", v.name, secret, out)
				}
			}
			for _, want := range []string{"access_token", "refresh_token", "u1", "PERSON"} {
				if !strings.Contains(out, want) {
					t.Errorf("log of a %s PrincipalIdentity lacks %q:\n%s", v.name, want, out)
				}
			}
		}
	}
	if p.Raw["access_token"] != accessSecret {
		t.Errorf("logging changed Principal.Raw[access_token] to %v", p.Raw["access_token"])
	}
}
