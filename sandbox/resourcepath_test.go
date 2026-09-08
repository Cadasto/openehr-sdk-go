package sandbox

import "testing"

// TestResourcePath pins the base-path rule: the sandbox cuts a request
// path through the first "/openehr/v1" segment wherever a deployment
// puts it, instead of matching a closed list of known prefixes. A
// deployment whose REST base carries some other prefix — "/api",
// "/cdr", a reverse-proxy mount — used to get a 404 that read as the
// consumer's own bug.
func TestResourcePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"canonical base", "/openehr/v1/ehr", "/ehr"},
		{"ehrbase base", "/ehrbase/rest/openehr/v1/ehr", "/ehr"},
		{"rest base", "/rest/openehr/v1/ehr", "/ehr"},
		{"ferroehr base", "/ferroehr/rest/openehr/v1/ehr", "/ehr"},
		{"unknown api prefix", "/api/openehr/v1/ehr", "/ehr"},
		{"deep proxy mount", "/a/b/c/openehr/v1/ehr/x", "/ehr/x"},
		{"base alone is the root", "/openehr/v1", "/"},
		{"no base segment at all", "/ehr", "/ehr"},
		{"empty path", "", ""},
		// Segment boundaries: neither of these contains the base.
		{"longer version segment", "/openehr/v11/ehr", "/openehr/v11/ehr"},
		{"base is a suffix of a longer segment", "/xopenehr/v1/ehr", "/xopenehr/v1/ehr"},
		// The first true base wins, even when the tail repeats it.
		{"repeated base", "/openehr/v1/openehr/v1/ehr", "/openehr/v1/ehr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resourcePath(tc.in); got != tc.want {
				t.Errorf("resourcePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestStatusLine pins the Status field net/http documents: "the status
// line", e.g. "200 OK" — not the bare reason phrase http.StatusText
// returns.
func TestStatusLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code int
		want string
	}{
		{200, "200 OK"},
		{201, "201 Created"},
		{404, "404 Not Found"},
		{409, "409 Conflict"},
	}
	for _, tc := range cases {
		if got := statusLine(tc.code); got != tc.want {
			t.Errorf("statusLine(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}
