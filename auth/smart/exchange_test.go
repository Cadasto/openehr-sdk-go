package smart_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

func TestParseTokenResponse(t *testing.T) {
	body := []byte(`{
		"access_token":"at",
		"token_type":"Bearer",
		"expires_in":3600,
		"refresh_token":"rt",
		"scope":"openid patient/*.read",
		"id_token":"jwt",
		"patient":"p1",
		"encounter":"e1",
		"fhirUser":"Practitioner/x"
	}`)
	tr, err := smart.ParseTokenResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if tr.AccessToken != "at" || tr.Patient != "p1" || tr.FHIRUser != "Practitioner/x" || tr.ExpiresIn != 3600 {
		t.Fatalf("tr = %#v", tr)
	}
	if tr.Raw["patient"] != "p1" {
		t.Fatalf("raw = %#v", tr.Raw)
	}
}
