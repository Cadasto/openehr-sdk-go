// Package probeclassinit is a fixture for the probe classifier: its init
// starts a test server when the package loads.
package probeclassinit

import "net/http/httptest"

func init() {
	httptest.NewServer(nil).Close()
}

// Probe910Plain names nothing, but the package's init runs when it loads.
func Probe910Plain() string {
	return "plain"
}
