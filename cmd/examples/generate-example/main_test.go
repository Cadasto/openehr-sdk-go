package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// TestREQ107_ParsePolicy is the REQ-107 check that --policy reads the five
// names required, minimal, medium, large and complete in any case, minimal
// as Required and complete as Large, and that any other text, the empty
// string included, is an error that wraps instance.ErrUnsupportedPolicy
// and names the flag rather than a silent default. The flag's default is
// medium. The maintainer changed the rule the earlier table pinned, under
// which minimal and Large were errors.
func TestREQ107_ParsePolicy(t *testing.T) {
	tests := []struct {
		name    string
		want    instance.Policy
		wantErr bool
	}{
		{name: defaultPolicy, want: instance.Medium},
		{name: "required", want: instance.Required},
		{name: "minimal", want: instance.Required},
		{name: "medium", want: instance.Medium},
		{name: "large", want: instance.Large},
		{name: "complete", want: instance.Large},
		{name: "Large", want: instance.Large},
		{name: "MINIMAL", want: instance.Required},
		{name: "Complete", want: instance.Large},
		{name: "example", wantErr: true},
		{name: " medium", wantErr: true},
		{name: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.name), func(t *testing.T) {
			got, err := parsePolicy(tc.name)
			if tc.wantErr {
				if !errors.Is(err, instance.ErrUnsupportedPolicy) {
					t.Errorf("parsePolicy(%q) = %v, %v; want an error wrapping instance.ErrUnsupportedPolicy", tc.name, got, err)
				} else if !strings.Contains(err.Error(), "--policy") {
					t.Errorf("parsePolicy(%q) error = %q, want it to name the flag --policy", tc.name, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("parsePolicy(%q) = %v, %v; want %v, nil", tc.name, got, err, tc.want)
			}
		})
	}
}
