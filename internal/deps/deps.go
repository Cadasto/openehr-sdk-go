// Package deps anchors runtime dependencies adopted for the SMART/auth
// conformance work (ADR 0009) so `go mod tidy` retains them before the
// auth phases import them directly. Remove once auth code imports these.
//
// TODO: remove this file once the auth packages (auth/, auth/smart/, smart/)
// import github.com/coreos/go-oidc/v3/oidc and golang.org/x/oauth2 directly.
package deps

import (
	_ "github.com/coreos/go-oidc/v3/oidc"
	_ "golang.org/x/oauth2"
)
