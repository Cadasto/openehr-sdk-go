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

// TestParseTokenResponseOpenEHRClaims asserts that the openEHR-native EHR and
// episode context claims are decoded into typed fields. REQ-064
func TestParseTokenResponseOpenEHRClaims(t *testing.T) {
	body := []byte(`{"access_token":"a","ehrId":"ehr-1","episodeId":"ep-9"}`)
	tr, err := smart.ParseTokenResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if tr.EHRID != "ehr-1" {
		t.Fatalf("EHRID = %q, want %q", tr.EHRID, "ehr-1")
	}
	if tr.EpisodeID != "ep-9" {
		t.Fatalf("EpisodeID = %q, want %q", tr.EpisodeID, "ep-9")
	}
}

// TestParseTokenResponseSMARTCompatExtras asserts that the SMART-compat extras
// (intent, smart_style_url, need_patient_banner, tenant) are decoded into typed
// fields. REQ-064
func TestParseTokenResponseSMARTCompatExtras(t *testing.T) {
	body := []byte(`{
		"access_token":"a",
		"intent":"patient-search",
		"smart_style_url":"https://example.com/style.json",
		"need_patient_banner":true,
		"tenant":"tenant-42"
	}`)
	tr, err := smart.ParseTokenResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Intent != "patient-search" {
		t.Fatalf("Intent = %q, want %q", tr.Intent, "patient-search")
	}
	if tr.SMARTStyleURL != "https://example.com/style.json" {
		t.Fatalf("SMARTStyleURL = %q, want %q", tr.SMARTStyleURL, "https://example.com/style.json")
	}
	if !tr.NeedPatientBanner {
		t.Fatalf("NeedPatientBanner = false, want true")
	}
	if tr.Tenant != "tenant-42" {
		t.Fatalf("Tenant = %q, want %q", tr.Tenant, "tenant-42")
	}
}
