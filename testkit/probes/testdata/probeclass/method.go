package probeclass

import "net/http/httptest"

type server struct {
	url string
}

func (s *server) start() {
	s.url = httptest.NewServer(nil).URL
}

// Probe906Method is a method, not a package-level probe function, so the
// classifier must not list it.
func (s *server) Probe906Method() string {
	return s.url
}

// Probe903ThroughMethod names no backend package itself; it reaches
// httptest only through a method of the same-package type server.
func Probe903ThroughMethod() string {
	var s server
	s.start()
	return s.url
}
