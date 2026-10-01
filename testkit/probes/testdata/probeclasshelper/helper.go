// Package probeclasshelper is a fixture for the probe classifier: helpers
// that the probes of probeclassimport call. Some of them start a test
// server and some do not.
package probeclasshelper

import (
	"net/http"
	"net/http/httptest"

	"github.com/cadasto/openehr-sdk-go/testkit/probes/testdata/probeclassdeep"
)

// StartServer starts a test server and returns its URL.
func StartServer() string {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	return s.URL
}

// Greeting opens no connection.
func Greeting() string {
	return "hello"
}

// Server is a test server that starts only when asked to.
type Server struct {
	name string
}

// NewServer returns a Server that has not started.
func NewServer() *Server {
	return &Server{name: "fixture"}
}

// Name opens no connection.
func (s *Server) Name() string {
	return s.name
}

// Start starts a test server and returns its URL.
func (s *Server) Start() string {
	srv := httptest.NewServer(nil)
	defer srv.Close()
	return srv.URL
}

// Relayed starts a test server through probeclassdeep.
func Relayed() string {
	return probeclassdeep.Start()
}
