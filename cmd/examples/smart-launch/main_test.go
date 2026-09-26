package main

import "testing"

// TestRun exercises the full standalone PKCE launch end-to-end against the
// in-process stub server. Passing here proves the flow works
// offline without any real SMART authorization server.
func TestRun(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}
