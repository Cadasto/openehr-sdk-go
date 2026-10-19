package main

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// TestREQ107_ParsePolicy is the REQ-107 check that --policy maps each of
// its three names onto the policy of that name, and that any other text,
// including a name in another case and the empty string, is an error
// rather than a silent default.
func TestREQ107_ParsePolicy(t *testing.T) {
	tests := []struct {
		name    string
		want    instance.Policy
		wantErr bool
	}{
		{name: "required", want: instance.Required},
		{name: "medium", want: instance.Medium},
		{name: "large", want: instance.Large},
		{name: "minimal", wantErr: true},
		{name: "example", wantErr: true},
		{name: "Large", wantErr: true},
		{name: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.name), func(t *testing.T) {
			got, err := parsePolicy(tc.name)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parsePolicy(%q) = %v, nil; want an error", tc.name, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("parsePolicy(%q) = %v, %v; want %v, nil", tc.name, got, err, tc.want)
			}
		})
	}
}
