package smart

import "context"

type launchContextKey struct{}

// LaunchContext is the application-level SMART launch surface
// (REQ-064, REQ-067). Populated from a token-endpoint response via
// [LaunchContextFromTokenResponse].
type LaunchContext struct {
	// FHIR-compat launch-context claims.
	Patient   string
	Encounter string
	User      string
	Scopes    []string
	IDToken   *IDTokenClaims
	Issuer    string
	Principal *PrincipalIdentity
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
	// Raw is a defensive shallow copy of the token-response raw map;
	// nested values are not deep-copied. Mutating it does not affect the
	// originating TokenResponse.
	Raw map[string]any
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
