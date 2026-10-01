package probeclass

import srv "net/http/httptest"

// Probe904Aliased reaches httptest directly, under an import alias.
func Probe904Aliased() *srv.Server {
	return nil
}
