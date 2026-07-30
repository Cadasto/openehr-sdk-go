package webtemplate_test

// REQ-116 (Phase 0) / PROBE-075 — pins the two failure modes of the open
// sibling-naming gap against the vendored oracles, so the documented blocked
// state is guarded rather than observed. Both assertions are expected to
// change when REQ-116 lands: corona must then build, and both fixtures join
// the PROBE-075 parity matrix.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Corona_Anamnese fails loudly: five SECTION.adhoc.v1 siblings reuse one
// archetype, every one derives the same web id from the shared concept term,
// and Build refuses to emit ambiguous duplicates.
func TestBuild_CoronaAnamneseBlockedOnIDCollision(t *testing.T) {
	c := compileFixture(t, fixtures.WebTemplateOpt("Corona_Anamnese"))
	if _, err := webtemplate.Build(c); !errors.Is(err, webtemplate.ErrIDCollision) {
		t.Fatalf("Build(Corona_Anamnese) err = %v, want ErrIDCollision (REQ-116 open gap)", err)
	}
}

// GECCO_Diagnose diverges silently: it builds without error, but 30 aqlPaths
// in the reference golden carry name predicates this builder never emits, so
// its output would fail reference parity. Only extending PROBE-075 to this
// fixture (REQ-116 plan Phase 3) can catch that class — which is why a
// successful Build here proves nothing beyond "no id collision".
func TestBuild_GeccoDiagnoseBuildsWithoutParity(t *testing.T) {
	c := compileFixture(t, fixtures.WebTemplateOpt("GECCO_Diagnose"))
	if _, err := webtemplate.Build(c); err != nil {
		t.Fatalf("Build(GECCO_Diagnose) err = %v, want nil (collides on nothing; diverges on aqlPath only)", err)
	}
}
