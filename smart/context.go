package smart

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"strconv"
)

type launchContextKey struct{}

// LaunchContext is the application-level SMART launch surface,
// populated from a token-endpoint response via
// [LaunchContextFromTokenResponse].
//
// Printed with fmt, a LaunchContext shows the tokens its Raw and its
// Principal's Raw hold as "[redacted]": see [LaunchContext.String]. Logged
// with log/slog, a *LaunchContext shows them the same way: see
// [LaunchContext.LogValue].
type LaunchContext struct {
	// FHIR-compat launch-context claims.
	Patient   string
	Encounter string
	// User is the verified ID token's fhirUser claim, else its sub. It is
	// empty without a verified ID token: a fhirUser member in the
	// token-endpoint body is not an identity claim and never sets it.
	User   string
	Scopes []string
	// IDToken holds the verified ID-token claims, nil without an ID token.
	// For a token response from the auth/smart Source it is that response's
	// IDTokenClaims: the claims the Source verified, shared rather than
	// copied, so read it and never modify it. smart trusts these claims as
	// they are, without verifying the token again or checking exp again, so
	// a context rebuilt after the token expired still carries them. Only the
	// Source sets a TokenResponse's IDTokenClaims; never fill them in yourself.
	IDToken *IDTokenClaims
	// Issuer is the value given with WithIssuer, else the verified ID
	// token's iss claim, and empty with neither. When the token response
	// already carries verified claims, a WithIssuer value is not compared
	// with their iss and takes its place here.
	Issuer    string
	Principal *PrincipalIdentity
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
	// launch context's own copy of the server's explicit value.
	NeedPatientBanner *bool
	// Tenant is the optional "tenant" launch-context parameter (SMART v2).
	Tenant string
	// Raw is a defensive shallow copy of the token-response raw map;
	// nested values are not deep-copied. Mutating it does not affect the
	// originating TokenResponse. Raw holds the token-endpoint members
	// verbatim, the access, refresh and ID tokens included, so it must not
	// be printed or logged as it is; print or log the *LaunchContext itself,
	// whose String and LogValue mask them.
	Raw map[string]any
}

// String describes lc with Raw and Principal's Raw shown as copies whose
// access_token, refresh_token and id_token members keep their keys but have
// their values replaced by "[redacted]". Principal is shown by the text of
// [PrincipalIdentity.String], IDToken by its claims and the other fields as
// they are; lc and its Raw are not changed. A LaunchContext value copied out
// of a *LaunchContext prints the same way.
func (lc LaunchContext) String() string {
	principal := "<nil>"
	if lc.Principal != nil {
		principal = lc.Principal.String()
	}
	banner := "<nil>"
	if lc.NeedPatientBanner != nil {
		banner = strconv.FormatBool(*lc.NeedPatientBanner)
	}
	return fmt.Sprintf("smart.LaunchContext{Patient: %q, Encounter: %q, User: %q, Scopes: %q, IDToken: %+v, Issuer: %q, Principal: %s, "+
		"EHRID: %q, EpisodeID: %q, Intent: %q, SMARTStyleURL: %q, NeedPatientBanner: %s, Tenant: %q, Raw: %v}",
		lc.Patient, lc.Encounter, lc.User, lc.Scopes, lc.IDToken, lc.Issuer, principal,
		lc.EHRID, lc.EpisodeID, lc.Intent, lc.SMARTStyleURL, banner, lc.Tenant, maskedTokens(lc.Raw))
}

// GoString returns the text of [LaunchContext.String], so that printing lc in
// Go syntax shows no token either.
func (lc LaunchContext) GoString() string { return lc.String() }

// Format writes the text of [LaunchContext.String] for every fmt verb it is
// called for, whatever the flags and width. fmt answers %T and %p without
// calling it, and %p of a LaunchContext value prints its fields, so never
// print a LaunchContext with %p.
func (lc LaunchContext) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, lc.String())
}

// LogValue lets log/slog log lc without its tokens. It logs the fields of lc
// as a group, keyed by field name, with Raw and Principal's Raw as copies
// whose access_token, refresh_token and id_token members keep their keys but
// have their values replaced by "[redacted]". lc itself is not changed. A nil
// lc logs as nil, as slog logs any nil pointer.
//
// The masking applies to a *LaunchContext, such as the one
// [LaunchContextFromTokenResponse] or [LaunchContextFromContext] hands out. A
// LaunchContext value copied out of it (*lc) and logged as it is does not go
// through LogValue: slog's text handler prints it with the text of
// [LaunchContext.String], which masks the same members, but its JSON handler
// encodes its fields, tokens included. Log the pointer instead.
func (lc *LaunchContext) LogValue() slog.Value {
	if lc == nil {
		return slog.AnyValue(nil)
	}
	principal := slog.Any("Principal", nil)
	if lc.Principal != nil {
		principal = slog.Attr{Key: "Principal", Value: lc.Principal.LogValue()}
	}
	banner := slog.Any("NeedPatientBanner", nil)
	if lc.NeedPatientBanner != nil {
		banner = slog.Bool("NeedPatientBanner", *lc.NeedPatientBanner)
	}
	return slog.GroupValue(
		slog.String("Patient", lc.Patient),
		slog.String("Encounter", lc.Encounter),
		slog.String("User", lc.User),
		slog.Any("Scopes", lc.Scopes),
		slog.Any("IDToken", lc.IDToken),
		slog.String("Issuer", lc.Issuer),
		principal,
		slog.String("EHRID", lc.EHRID),
		slog.String("EpisodeID", lc.EpisodeID),
		slog.String("Intent", lc.Intent),
		slog.String("SMARTStyleURL", lc.SMARTStyleURL),
		banner,
		slog.String("Tenant", lc.Tenant),
		slog.Any("Raw", maskedTokens(lc.Raw)),
	)
}

// tokenMembers are the token-endpoint members that carry a credential
// (RFC 6749 §5.1; OpenID Connect Core 1.0 §3.1.3.3).
var tokenMembers = [...]string{"access_token", "refresh_token", "id_token"}

// tokenMask stands in for the value of a token member when a value is
// printed or logged.
const tokenMask = "[redacted]"

// maskedTokens returns a shallow copy of raw whose token members, when
// present, hold tokenMask. A nil raw gives nil.
func maskedTokens(raw map[string]any) map[string]any {
	masked := maps.Clone(raw)
	for _, k := range tokenMembers {
		if _, ok := masked[k]; ok {
			masked[k] = tokenMask
		}
	}
	return masked
}

// WithLaunchContext attaches lc to ctx for downstream handlers.
func WithLaunchContext(ctx context.Context, lc *LaunchContext) context.Context {
	if lc == nil {
		return ctx
	}
	return context.WithValue(ctx, launchContextKey{}, lc)
}

// LaunchContextFromContext returns the launch context attached via
// [WithLaunchContext], or (nil, false) when absent.
func LaunchContextFromContext(ctx context.Context) (*LaunchContext, bool) {
	lc, ok := ctx.Value(launchContextKey{}).(*LaunchContext)
	return lc, ok && lc != nil
}
