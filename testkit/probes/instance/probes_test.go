package instanceprobes_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	instanceprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/instance"
)

func optPath(t *testing.T, name string) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "openehr", "template", "testdata")
	return filepath.Join(root, name)
}

func compileFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(optPath(t, name))
	if err != nil {
		t.Fatalf("ParseFile %s: %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", name, err)
	}
	return c
}

func testComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

func TestProbe027VitalSignsPasses(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Territory: "NL",
		Composer:  testComposer(),
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("Probe027 status=%q detail=%q", r.Status, r.Detail)
	}
}

func TestProbe027NilCompiledFails(t *testing.T) {
	_, err := instanceprobes.Probe027GeneratedValidates(context.Background(), nil, instance.Options{})
	if err == nil {
		t.Fatal("expected error for nil compiled template, got nil")
	}
}

func TestProbe027MissingTerritoryFails(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	r, err := instanceprobes.Probe027GeneratedValidates(context.Background(), c, instance.Options{
		Composer: testComposer(),
	})
	if err != nil {
		t.Fatalf("Probe027: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("missing Territory should fail, got status=%q detail=%q", r.Status, r.Detail)
	}
}
