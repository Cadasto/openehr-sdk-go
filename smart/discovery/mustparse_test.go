package discovery_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ070_MustParseURLPanicMasksPassword checks that MustParseURL still
// panics on a URL url.Parse refuses, and that the panic value names the
// URL's host but not the password in its userinfo, which both the URL it
// was given and url.Parse's error quote.
func TestREQ070_MustParseURLPanicMasksPassword(t *testing.T) { // REQ-070
	t.Parallel()
	const password = "s3cret"
	tests := []struct {
		name string
		raw  string
		host string // text the panic value must still carry
	}{
		{name: "bad escape in the host", raw: "https://svc:" + password + "@x%zz.example/token", host: "x%zz.example"},
		{name: "unclosed IPv6 host", raw: "https://svc:" + password + "@[::1", host: "[::1"},
		{name: "slash in the password", raw: "https://svc:s3/" + password + "@x.example/token", host: "x.example"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mustParseURLPanic(tc.raw)
			if got == "" {
				t.Fatalf("MustParseURL(%q) did not panic, want a panic", tc.raw)
			}
			if strings.Contains(got, password) {
				t.Errorf("MustParseURL(%q) panic = %q, want no password", tc.raw, got)
			}
			if !strings.Contains(got, tc.host) {
				t.Errorf("MustParseURL(%q) panic = %q, want it to name the host %q", tc.raw, got, tc.host)
			}
		})
	}
}

// mustParseURLPanic calls MustParseURL(raw) and returns the text of the
// value it panics with, or "" when it does not panic.
func mustParseURLPanic(raw string) (text string) {
	defer func() {
		if r := recover(); r != nil {
			text = fmt.Sprint(r)
		}
	}()
	discovery.MustParseURL(raw)
	return ""
}
