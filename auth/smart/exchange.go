package smart

import (
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TokenResponse is the SMART token-endpoint payload. The
// application-level smart/ package maps this into LaunchContext.
type TokenResponse struct {
	AccessToken  string
	TokenType    string
	ExpiresIn    int64
	RefreshToken string
	Scope        string
	IDToken      string
	// IDTokenClaims holds the verified claims of the session's ID token.
	// Only a [Source] sets it, after verifying the ID token the token
	// endpoint returned to [Source.ExchangeAuthorizationCode],
	// [Source.CompleteAuthorization] or a refresh; a refresh without an
	// ID token carries the session's earlier claims. It is nil when the
	// session has no verified ID token, and on a value built by
	// [ParseTokenResponse], which verifies nothing.
	//
	// smart.LaunchContextFromTokenResponse trusts these claims as they are:
	// it does not verify them again or check their expiry. Treat them as
	// read-only; the value is shared with [Source.LastTokenResponse].
	IDTokenClaims *IDTokenClaims
	// FHIR-compat launch-context claims.
	Patient   string
	Encounter string
	FHIRUser  string
	// openEHR-native launch-context claims.
	// EHRID is the EHR identifier conveyed via the "ehrId" token claim,
	// requested via the "launch/patient" scope in the openEHR SMART spec
	// (https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html).
	EHRID string
	// EpisodeID is the episode identifier conveyed via the "episodeId" token
	// claim, requested via the "launch/episode" scope (experimental).
	EpisodeID string
	// SMART-compat extras surfaced by reference SMART clients.
	// Intent is the optional "intent" launch-context parameter (SMART v2).
	Intent string
	// SMARTStyleURL is the optional "smart_style_url" launch-context parameter (SMART v2).
	SMARTStyleURL string
	// NeedPatientBanner is the optional "need_patient_banner" launch-context parameter
	// (SMART v2). nil means the server did not express a preference, and the caller should
	// apply the SMART default of showing the patient banner. Non-nil points to the
	// server's explicit value.
	NeedPatientBanner *bool
	// Tenant is the optional "tenant" launch-context parameter (SMART v2).
	Tenant string
	Raw    map[string]any
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

// sessionMembers are the token-response members a refresh response may
// leave out while the session keeps them: the SMART launch-context
// parameters and the granted scope. keep copies the member's typed field;
// fhirContext has none and lives in Raw only.
var sessionMembers = []struct {
	key  string
	keep func(dst *TokenResponse, src TokenResponse)
}{
	{"patient", func(d *TokenResponse, s TokenResponse) { d.Patient = s.Patient }},
	{"encounter", func(d *TokenResponse, s TokenResponse) { d.Encounter = s.Encounter }},
	{"ehrId", func(d *TokenResponse, s TokenResponse) { d.EHRID = s.EHRID }},
	{"episodeId", func(d *TokenResponse, s TokenResponse) { d.EpisodeID = s.EpisodeID }},
	{"fhirContext", nil},
	{"intent", func(d *TokenResponse, s TokenResponse) { d.Intent = s.Intent }},
	{"need_patient_banner", func(d *TokenResponse, s TokenResponse) { d.NeedPatientBanner = s.NeedPatientBanner }},
	{"smart_style_url", func(d *TokenResponse, s TokenResponse) { d.SMARTStyleURL = s.SMARTStyleURL }},
	{"tenant", func(d *TokenResponse, s TokenResponse) { d.Tenant = s.Tenant }},
	{"scope", func(d *TokenResponse, s TokenResponse) { d.Scope = s.Scope }},
}

// keepSessionMembers returns next, a refresh response, with each
// launch-context parameter and the scope that prev, the session's last
// token response, carried and next leaves out taken from prev: SMART App
// Launch lets a refresh response omit the launch context, and RFC 6749 §6
// reads an omitted scope as the original grant.
//
// A member counts as left out only when its key is absent from next's
// body, so a member next carries, even as an empty string or null, stays as
// next has it. A kept member goes into the typed field and into Raw, which
// is how fhirContext is kept; Raw's other members are next's own. The
// result has its own Raw map, so neither prev nor next is changed.
func keepSessionMembers(prev, next TokenResponse) TokenResponse {
	raw := make(map[string]any, len(next.Raw)+len(sessionMembers))
	maps.Copy(raw, next.Raw)
	for _, m := range sessionMembers {
		v, had := prev.Raw[m.key]
		if _, has := next.Raw[m.key]; has || !had {
			continue
		}
		raw[m.key] = v
		if m.keep != nil {
			m.keep(&next, prev)
		}
	}
	next.Raw = raw
	return next
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
		Type:   authScheme(tr.TokenType),
		Scope:  tr.Scope,
		Issuer: issuer,
	}
	if tr.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok
}

// authScheme maps a token endpoint's token_type to the Authorization scheme.
// The value is case insensitive (RFC 6749 §5.1), so any spelling of bearer,
// or no value at all, becomes "Bearer"; any other scheme passes through
// unchanged.
func authScheme(tokenType string) string {
	if tokenType == "" || strings.EqualFold(tokenType, auth.TokenTypeBearer) {
		return auth.TokenTypeBearer
	}
	return tokenType
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
	NeedPatientBanner *bool  `json:"need_patient_banner"`
	Tenant            string `json:"tenant"`
}
