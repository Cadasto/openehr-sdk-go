package smart

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/internal/expiresin"
	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
)

// TokenResponse is the SMART token-endpoint payload. The
// application-level smart/ package maps this into LaunchContext.
//
// Printed with fmt or logged with log/slog, a TokenResponse shows its access,
// refresh and ID tokens as "[redacted]", in their fields and in Raw, and the
// URLs it holds with their passwords masked: see [TokenResponse.String] and
// [TokenResponse.LogValue]. Its JSON encoding is unchanged and carries the
// tokens.
type TokenResponse struct {
	AccessToken string
	TokenType   string
	// ExpiresIn is the response's expires_in, the access token's lifetime
	// in whole seconds, or 0 when the response has none: HasExpiresIn tells
	// the two apart. A number such as 3600.5 or 3.6e3 is rounded down. Every
	// value, an integer included, is kept between minus and plus 100 years,
	// so turning it into a time.Duration of that many seconds cannot
	// overflow. A number too large for a float64 counts as none.
	ExpiresIn int64
	// HasExpiresIn reports whether the response carried expires_in as a
	// number. A source gives the access token a lifetime of zero seconds
	// for an expires_in of zero or below (RFC 6749 §5.1), so the token is
	// renewed on the next call, and no expiry when the response has none.
	HasExpiresIn bool
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

// redacted is the text that stands for a credential when a value is printed
// or logged.
const redacted = "[redacted]"

// redact returns redacted for a credential s that is set and "" for an empty
// one.
func redact(s string) string {
	if s == "" {
		return ""
	}
	return redacted
}

// tokenMembers are the token-endpoint members that carry a credential
// (RFC 6749 §5.1; OpenID Connect Core 1.0 §3.1.3.3).
var tokenMembers = [...]string{"access_token", "refresh_token", "id_token"}

// urlMembers are the token-endpoint and ID-token members whose value is a
// URL (OpenID Connect Core 1.0 §2; SMART App Launch fhirUser and
// smart_style_url).
var urlMembers = [...]string{"iss", "fhirUser", "smart_style_url"}

// maskedRaw returns a shallow copy of raw whose token members hold redacted
// when they hold anything other than "" or null, and whose URL members that
// hold a string hold it with the password masked. The keys are kept. A nil
// raw gives nil, and raw itself is not changed.
func maskedRaw(raw map[string]any) map[string]any {
	masked := maps.Clone(raw)
	for _, k := range tokenMembers {
		if v, ok := masked[k]; ok && v != nil && v != "" {
			masked[k] = redacted
		}
	}
	for _, k := range urlMembers {
		if v, ok := masked[k].(string); ok {
			masked[k] = urlredact.String(v)
		}
	}
	return masked
}

// maskedClaims returns a copy of c whose Issuer and FHIRUser URLs have their
// passwords masked, or nil for a nil c. c is not changed.
func maskedClaims(c *IDTokenClaims) *IDTokenClaims {
	if c == nil {
		return nil
	}
	masked := *c
	masked.Issuer = urlredact.String(c.Issuer)
	masked.FHIRUser = urlredact.String(c.FHIRUser)
	return &masked
}

// String describes tr with its AccessToken, RefreshToken and IDToken, and
// the access_token, refresh_token and id_token members of a copy of Raw,
// replaced by "[redacted]", or left empty when they are empty. The password
// of each URL it holds is masked, as [net/url.URL.Redacted] masks it:
// FHIRUser, SMARTStyleURL, the Issuer and FHIRUser of IDTokenClaims, and the
// iss, fhirUser and smart_style_url members of the copy of Raw. The other
// fields are shown as they are; tr, its IDTokenClaims and its Raw are not
// changed.
func (tr TokenResponse) String() string {
	banner := "<nil>"
	if tr.NeedPatientBanner != nil {
		banner = strconv.FormatBool(*tr.NeedPatientBanner)
	}
	return fmt.Sprintf("smart.TokenResponse{AccessToken: %q, TokenType: %q, ExpiresIn: %d, HasExpiresIn: %t, RefreshToken: %q, Scope: %q, IDToken: %q, "+
		"IDTokenClaims: %+v, Patient: %q, Encounter: %q, FHIRUser: %q, EHRID: %q, EpisodeID: %q, Intent: %q, SMARTStyleURL: %q, "+
		"NeedPatientBanner: %s, Tenant: %q, Raw: %v}",
		redact(tr.AccessToken), tr.TokenType, tr.ExpiresIn, tr.HasExpiresIn, redact(tr.RefreshToken), tr.Scope, redact(tr.IDToken),
		maskedClaims(tr.IDTokenClaims), tr.Patient, tr.Encounter, urlredact.String(tr.FHIRUser), tr.EHRID, tr.EpisodeID, tr.Intent,
		urlredact.String(tr.SMARTStyleURL), banner, tr.Tenant, maskedRaw(tr.Raw))
}

// GoString returns the text of [TokenResponse.String], so that printing tr in
// Go syntax shows no token either.
func (tr TokenResponse) GoString() string { return tr.String() }

// Format writes the text of [TokenResponse.String] for every fmt verb it is
// called for, whatever the flags and width. fmt answers %T and %p without
// calling it, and %p of a TokenResponse value prints its fields, so never
// print a TokenResponse with %p.
func (tr TokenResponse) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, tr.String())
}

// LogValue lets log/slog log tr as a group of its fields, keyed by field
// name, with the tokens replaced and the URLs masked as
// [TokenResponse.String] replaces and masks them. tr, its IDTokenClaims and
// its Raw are not changed.
func (tr TokenResponse) LogValue() slog.Value {
	banner := slog.Any("NeedPatientBanner", nil)
	if tr.NeedPatientBanner != nil {
		banner = slog.Bool("NeedPatientBanner", *tr.NeedPatientBanner)
	}
	return slog.GroupValue(
		slog.String("AccessToken", redact(tr.AccessToken)),
		slog.String("TokenType", tr.TokenType),
		slog.Int64("ExpiresIn", tr.ExpiresIn),
		slog.Bool("HasExpiresIn", tr.HasExpiresIn),
		slog.String("RefreshToken", redact(tr.RefreshToken)),
		slog.String("Scope", tr.Scope),
		slog.String("IDToken", redact(tr.IDToken)),
		slog.Any("IDTokenClaims", maskedClaims(tr.IDTokenClaims)),
		slog.String("Patient", tr.Patient),
		slog.String("Encounter", tr.Encounter),
		slog.String("FHIRUser", urlredact.String(tr.FHIRUser)),
		slog.String("EHRID", tr.EHRID),
		slog.String("EpisodeID", tr.EpisodeID),
		slog.String("Intent", tr.Intent),
		slog.String("SMARTStyleURL", urlredact.String(tr.SMARTStyleURL)),
		banner,
		slog.String("Tenant", tr.Tenant),
		slog.Any("Raw", maskedRaw(tr.Raw)),
	)
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
	if secs, ok := expiresin.Seconds(string(tr.ExpiresIn)); ok {
		out.ExpiresIn = secs
		out.HasExpiresIn = true
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

func tokenFromResponse(tr TokenResponse, issuer string) auth.Token {
	tok := auth.Token{
		Value:  tr.AccessToken,
		Type:   authScheme(tr.TokenType),
		Scope:  tr.Scope,
		Issuer: issuer,
	}
	// RFC 6749 §5.1: expires_in is the token's lifetime, so one of zero or
	// below makes the token due at once, and only a response without one
	// leaves it without an expiry (REQ-063). A non-zero ExpiresIn counts as
	// present on a TokenResponse built without HasExpiresIn.
	if tr.HasExpiresIn || tr.ExpiresIn != 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(max(tr.ExpiresIn, 0)) * time.Second)
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
