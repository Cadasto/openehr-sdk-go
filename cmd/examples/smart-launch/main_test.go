package main

import "testing"

// TestRun exercises the full standalone PKCE launch end-to-end against the
// in-process stub server. Passing here proves the flow works
// offline without any real SMART authorization server, from the authorize
// URL to CompleteAuthorization on the redirect's query. The stub refuses an
// authorization request without aud, so passing also proves the source
// built from the catalog sends one.
func TestRun(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}
