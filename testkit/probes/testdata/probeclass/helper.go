// Package probeclass is a fixture for the probe classifier in
// testkit/probes. Each exported Probe function reaches, or does not reach,
// a backend package in one known way. The go tool never builds it, because
// it sits under a testdata directory.
package probeclass

import (
	"net/http"
	"net/http/httptest"
)

// Probe901ThroughHelper names no backend package itself; it reaches
// httptest only through the same-package helper startServer.
func Probe901ThroughHelper() string {
	return startServer()
}

func startServer() string {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	return s.URL
}
