package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The program runs to the end on a template whose root has no content
// attribute, such as the demographic Address.v2.opt, whose root ADDRESS has
// name and details.
func TestRunTemplateWithoutContent(t *testing.T) {
	opt := fixtures.TemplateOptForName("Address.v2")
	if err := run([]string{opt}); err != nil {
		t.Fatalf("run(%s) = %v, want nil", opt, err)
	}
}

// The program looks /content up where the root has a child under it, and the
// root's first attribute with a child otherwise.
func TestPathToLookUp(t *testing.T) {
	cases := []struct {
		template string
		want     string
	}{
		{template: "vital_signs", want: "/content"},
		{template: "Address.v2", want: "/name"},
	}
	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			raw, err := os.ReadFile(fixtures.TemplateOptForName(tc.template))
			if err != nil {
				t.Fatal(err)
			}
			opt, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("ReadOperationalTemplate(%s): %v", tc.template, err)
			}
			compiled, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile(%s): %v", tc.template, err)
			}
			if got, ok := pathToLookUp(compiled.Root()); !ok || got != tc.want {
				t.Errorf("pathToLookUp(%s root) = %q, %t; want %q, true", tc.template, got, ok, tc.want)
			}
		})
	}
}
