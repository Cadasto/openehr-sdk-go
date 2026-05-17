package definition

import "testing"

func TestExtractLastPathSegment(t *testing.T) {
	tests := []struct {
		name string
		loc  string
		want string
	}{
		{"empty", "", ""},
		{"relative", "/definition/template/adl1.4/my-template", "my-template"},
		{"absolute with query", "https://cdr.example/openehr/v1/definition/template/adl1.4/tmpl?version=1", "tmpl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractLastPathSegment(tc.loc); got != tc.want {
				t.Errorf("extractLastPathSegment(%q) = %q, want %q", tc.loc, got, tc.want)
			}
		})
	}
}
