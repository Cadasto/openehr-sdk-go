package smart

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TokenResponse is the SMART token-endpoint payload. The
// application-level smart/ package maps this into LaunchContext.
type TokenResponse struct {
	AccessToken string
	TokenType   string
	// ExpiresIn is the response's expires_in, the access token's lifetime
	// in whole seconds, or 0 when the response has none. An integer is kept
	// as sent. Any other number, such as 3600.5 or 3.6e3, is rounded down,
	// and counts as 100 years when larger; one too large for a float64
	// counts as none.
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
	// it does not verify them again or check their expiry, so pass them on
	// unchanged. Each TokenResponse a Source hands out has its own copy of
	// them, so changing one changes neither the source nor another copy.
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
	if secs, ok := expiresInSeconds(string(tr.ExpiresIn)); ok {
		out.ExpiresIn = secs
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

// cloneTokenResponse returns a copy of tr that shares no map, slice or
// pointer with tr: Raw is copied all the way down, and IDTokenClaims, with
// its Audience and Extra, and NeedPatientBanner point to copies of their
// own. The source keeps and hands out only such copies, so a caller who
// changes a token response it was given changes neither the source nor
// another caller's copy, and does not race with a refresh reading the
// source's own.
func cloneTokenResponse(tr TokenResponse) TokenResponse {
	tr.Raw = cloneJSONObject(tr.Raw)
	if tr.NeedPatientBanner != nil {
		tr.NeedPatientBanner = new(*tr.NeedPatientBanner)
	}
	if tr.IDTokenClaims != nil {
		claims := *tr.IDTokenClaims
		claims.Audience = slices.Clone(claims.Audience)
		claims.Extra = cloneJSONObject(claims.Extra)
		tr.IDTokenClaims = &claims
	}
	return tr
}

// cloneJSONObject returns a copy of m, an object decoded from JSON, with a
// copy of each object and array nested in it, so it shares no map or slice
// with m. A nil m gives nil.
func cloneJSONObject(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneJSONValue(v)
	}
	return out
}

// cloneJSONValue returns a copy of v, a value decoded from JSON, that
// shares no map or slice with v. A string, number, bool or null holds
// nothing to share and is returned as it is.
func cloneJSONValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return cloneJSONObject(v)
	case []any:
		if v == nil {
			return v
		}
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = cloneJSONValue(e)
		}
		return out
	default:
		return v
	}
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

// maxExpiresIn is the longest lifetime, in seconds, that an expires_in
// gives an access token: 100 years. A larger value counts as this one, so
// that turning it into a time.Duration cannot overflow into an expiry in the
// past, which would have every call refresh the token.
const maxExpiresIn = 100 * 365 * 24 * 60 * 60

// expiresInSeconds reads s, the expires_in member of a token response, as
// whole seconds. RFC 6749 §5.1 makes it "the lifetime in seconds of the
// access token", a JSON number, so every form of one counts. An integer is
// read as it is. Any other number, such as 3600.0, 3.6e3, 3600.5 or an
// integer beyond the int64 range, counts as its whole seconds rounded down,
// and as maxExpiresIn when it is larger. ok is false when s is not a number,
// or is one a float64 cannot hold (it reads as NaN or ±Inf).
func expiresInSeconds(s string) (secs int64, ok bool) {
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil {
		return secs, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	// Bounding f first keeps the conversion inside the int64 range, where
	// it is defined. A negative value stays negative, so it gives no
	// expiry, as a negative integer does.
	return int64(math.Floor(min(max(f, -maxExpiresIn), maxExpiresIn))), true
}

func tokenFromResponse(tr TokenResponse, issuer string) auth.Token {
	tok := auth.Token{
		Value:  tr.AccessToken,
		Type:   authScheme(tr.TokenType),
		Scope:  tr.Scope,
		Issuer: issuer,
	}
	if tr.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(min(tr.ExpiresIn, maxExpiresIn)) * time.Second)
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
