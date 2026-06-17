package smart

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TokenResponse is the SMART token-endpoint payload (REQ-061). The
// application-level smart/ package maps this into LaunchContext
// (REQ-064) after optional ID-token validation.
type TokenResponse struct {
	AccessToken  string
	TokenType    string
	ExpiresIn    int64
	RefreshToken string
	Scope        string
	IDToken      string
	// FHIR-compat launch-context claims.
	Patient   string
	Encounter string
	FHIRUser  string
	// openEHR-native launch-context claims (REQ-064).
	// EHRID is the EHR identifier conveyed via the "ehrId" token claim,
	// requested via the "launch/patient" scope in the openEHR SMART spec
	// (https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html).
	EHRID string
	// EpisodeID is the episode identifier conveyed via the "episodeId" token
	// claim, requested via the "launch/episode" scope (experimental).
	EpisodeID string
	// SMART-compat extras surfaced by reference SMART clients.
	Intent            string
	SMARTStyleURL     string
	NeedPatientBanner bool
	Tenant            string
	Raw               map[string]any
}

// ParseTokenResponse decodes a token-endpoint JSON body into
// TokenResponse. Used by tests and by the exchange path in Source.
func ParseTokenResponse(body []byte) (TokenResponse, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return TokenResponse{}, fmt.Errorf("token response: %w", err)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return TokenResponse{}, fmt.Errorf("token response: %w", err)
	}
	out := TokenResponse{
		AccessToken:       tr.AccessToken,
		TokenType:         tr.TokenType,
		RefreshToken:      tr.RefreshToken,
		Scope:             tr.Scope,
		IDToken:           tr.IDToken,
		Patient:           tr.Patient,
		Encounter:         tr.Encounter,
		FHIRUser:          tr.FHIRUser,
		EHRID:             tr.EHRID,
		EpisodeID:         tr.EpisodeID,
		Intent:            tr.Intent,
		SMARTStyleURL:     tr.SMARTStyleURL,
		NeedPatientBanner: tr.NeedPatientBanner,
		Tenant:            tr.Tenant,
		Raw:               rawJSONToAny(raw),
	}
	if tr.ExpiresIn != "" {
		sec, err := strconv.ParseInt(string(tr.ExpiresIn), 10, 64)
		if err == nil {
			out.ExpiresIn = sec
		}
	}
	return out, nil
}

func rawJSONToAny(raw map[string]json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		var anyVal any
		if err := json.Unmarshal(v, &anyVal); err == nil {
			out[k] = anyVal
		} else {
			out[k] = string(v)
		}
	}
	return out
}

func tokenFromResponse(tr TokenResponse, issuer string) auth.Token {
	tok := auth.Token{
		Value:  tr.AccessToken,
		Type:   tr.TokenType,
		Scope:  tr.Scope,
		Issuer: issuer,
	}
	if tok.Type == "" {
		tok.Type = auth.TokenTypeBearer
	}
	if tr.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok
}

type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    json.Number `json:"expires_in"`
	RefreshToken string      `json:"refresh_token"`
	Scope        string      `json:"scope"`
	IDToken      string      `json:"id_token"`
	// FHIR-compat launch-context claims.
	Patient   string `json:"patient"`
	Encounter string `json:"encounter"`
	FHIRUser  string `json:"fhirUser"`
	// openEHR-native launch-context claims (REQ-064).
	EHRID     string `json:"ehrId"`
	EpisodeID string `json:"episodeId"`
	// SMART-compat extras.
	Intent            string `json:"intent"`
	SMARTStyleURL     string `json:"smart_style_url"`
	NeedPatientBanner bool   `json:"need_patient_banner"`
	Tenant            string `json:"tenant"`
}
