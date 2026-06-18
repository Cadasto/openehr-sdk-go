package main

import "testing"

// TestRunFlow exercises the full standalone PKCE launch end-to-end against the
// in-process stub server (REQ-061, F-H).  Passing here proves the flow works
// offline without any real SMART authorization server.
func TestRunFlow(t *testing.T) {
	if err := runFlow(); err != nil {
		t.Fatalf("runFlow: %v", err)
	}
}
