package terminology_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// REQ-034 § openEHR terminology vocabulary — the pins over the *real*
// generated tables (openehr_gen.go), as opposed to terminology_test.go's
// hand-built groups, which exercise the lookup types.
//
// None of these tests calls t.Parallel(): the sibling in-package test
// TestRegistryAccessorsUseTheTables swaps the package registry slices for
// its own and restores them, so every test that reads the real registry
// stays on the serial run.

// pinPath is the vendored terminology, relative to this package directory —
// `go test` runs each package with its own directory as the working one.
const pinPath = "../../resources/terminology/openehr_terminology.xml"

func TestPinnedRelease(t *testing.T) {
	if terminology.Version != "3.0.0" {
		t.Errorf("Version = %q, want 3.0.0 (the pinned openEHR TERM release)", terminology.Version)
	}
	if terminology.ID != "openehr" {
		t.Errorf("ID = %q, want openehr", terminology.ID)
	}
	pin, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatalf("read the pinned terminology: %v", err)
	}
	sum := sha256.Sum256(pin)
	if got, want := terminology.SourceSHA256, hex.EncodeToString(sum[:]); got != want {
		t.Errorf("SourceSHA256 = %q, but %s hashes to %q — run 'make termgen'", got, pinPath, want)
	}
}

func TestTablesMatchTheOpenEHRTerminology(t *testing.T) {
	groups := slices.Collect(terminology.Groups())
	codeSets := slices.Collect(terminology.CodeSets())

	// Counts pinned from TERM Release-3.0.0.
	if len(groups) != 17 {
		t.Errorf("Groups() yields %d groups, want 17", len(groups))
	}
	if len(codeSets) != 3 {
		t.Errorf("CodeSets() yields %d code sets, want 3", len(codeSets))
	}
	concepts := 0
	for _, g := range groups {
		concepts += g.Len()
	}
	if concepts != 249 {
		t.Errorf("the groups hold %d concepts in total, want 249", concepts)
	}
	codes := 0
	for _, s := range codeSets {
		codes += s.Len()
	}
	if codes != 19 {
		t.Errorf("the code sets hold %d codes in total, want 19", codes)
	}

	// Spot pins — the codes SDK surfaces default, validate and decode with.
	wantRubric(t, terminology.AuditChangeType, "523", "deleted")
	wantRubric(t, terminology.AuditChangeType, "253", "unknown")
	wantRubric(t, terminology.AuditChangeType, "252", "synthesis")
	wantLen(t, terminology.AuditChangeType, 9)

	wantRubric(t, terminology.VersionLifecycleState, "532", "complete")
	wantRubric(t, terminology.VersionLifecycleState, "800", "inactive")
	wantLen(t, terminology.VersionLifecycleState, 5)

	// The upstream quirk the pin flags itself (SPECPR-51): code 532 is
	// "complete" in version lifecycle state and "completed" here. A rubric
	// belongs to a code *within a group*, which is why lookups are per group.
	wantRubric(t, terminology.InstructionStates, "532", "completed")

	wantLen(t, terminology.ParticipationMode, 32)
	wantRubric(t, terminology.ParticipationMode, "193", "not specified")
	wantRubric(t, terminology.ParticipationMode, "224", "interpreted video communication")
	if got, ok := terminology.ParticipationMode.Code("face-to-face communication"); !ok || got != "216" {
		t.Errorf("ParticipationMode.Code(face-to-face communication) = %q, %v; want 216, true", got, ok)
	}

	wantRubric(t, terminology.Setting, "238", "other care")
	if !terminology.Setting.Has("227") {
		t.Error("Setting.Has(227) = false, want true (emergency care)")
	}
	if terminology.Setting.Has("226") {
		t.Error("Setting.Has(226) = true, want false — 226 is not a setting in the pin")
	}

	wantRubric(t, terminology.CompositionCategory, "433", "event")
	wantRubric(t, terminology.EventMathFunction, "146", "mean")
	wantRubric(t, terminology.EventMathFunction, "640", "actual")

	for _, code := range []string{"N", "H", "HH", "HHH", "L", "LL", "LLL"} {
		if !terminology.NormalStatuses.Has(code) {
			t.Errorf("NormalStatuses.Has(%q) = false, want true", code)
		}
	}
	if terminology.NormalStatuses.Len() != 7 {
		t.Errorf("NormalStatuses.Len() = %d, want 7", terminology.NormalStatuses.Len())
	}
	if terminology.NormalStatuses.Has("X") {
		t.Error("NormalStatuses.Has(X) = true, want false")
	}

	// The registries resolve to the same values the exported variables name.
	if g, ok := terminology.GroupByID("setting"); !ok || g != terminology.Setting {
		t.Errorf("GroupByID(setting) = %v, %v; want the Setting group", g, ok)
	}
	if s, ok := terminology.CodeSetByID("normal_statuses"); !ok || s != terminology.NormalStatuses {
		t.Errorf("CodeSetByID(normal_statuses) = %v, %v; want the NormalStatuses code set", s, ok)
	}
	if g, ok := terminology.GroupByID("nope"); ok {
		t.Errorf("GroupByID(nope) = %v, true; want absence", g)
	}
	if s, ok := terminology.CodeSetByID("nope"); ok {
		t.Errorf("CodeSetByID(nope) = %v, true; want absence", s)
	}
}

// TestEveryGroupIsInvertible walks all 249 concepts: within a group, the
// code-to-rubric and rubric-to-code lookups must be inverses of each other,
// and Len must agree with what All yields. That is the property the parser's
// per-group uniqueness refusals exist to guarantee.
func TestEveryGroupIsInvertible(t *testing.T) {
	for g := range terminology.Groups() {
		n := 0
		for c := range g.All() {
			n++
			if got, ok := g.Rubric(c.Code); !ok || got != c.Rubric {
				t.Errorf("%s.Rubric(%q) = %q, %v; want %q, true", g.ID(), c.Code, got, ok, c.Rubric)
			}
			if got, ok := g.Code(c.Rubric); !ok || got != c.Code {
				t.Errorf("%s.Code(%q) = %q, %v; want %q, true", g.ID(), c.Rubric, got, ok, c.Code)
			}
			if !g.Has(c.Code) {
				t.Errorf("%s.Has(%q) = false, want true", g.ID(), c.Code)
			}
		}
		if n != g.Len() {
			t.Errorf("%s: All yields %d concepts but Len is %d", g.ID(), n, g.Len())
		}
	}
	for s := range terminology.CodeSets() {
		n := 0
		for code := range s.All() {
			n++
			if !s.Has(code) {
				t.Errorf("%s.Has(%q) = false, want true", s.ID(), code)
			}
		}
		if n != s.Len() {
			t.Errorf("%s: All yields %d codes but Len is %d", s.ID(), n, s.Len())
		}
	}
}

// TestTablesAreInSourceOrder pins the pin's own document order — the order
// the generated registry slices carry, and the order All promises.
func TestTablesAreInSourceOrder(t *testing.T) {
	groups := slices.Collect(terminology.Groups())
	if len(groups) == 0 {
		t.Fatal("Groups() yields nothing — the generated tables are missing")
	}
	if got := groups[0]; got != terminology.AttestationReason {
		t.Errorf("first group is %q, want attestation_reason", got.ID())
	}
	if got := groups[len(groups)-1]; got != terminology.ExtractUpdateTriggerEventType {
		t.Errorf("last group is %q, want extract_update_trigger_event_type", got.ID())
	}
	codeSets := slices.Collect(terminology.CodeSets())
	wantIDs := []string{"compression_algorithms", "integrity_check_algorithms", "normal_statuses"}
	gotIDs := make([]string, 0, len(codeSets))
	for _, s := range codeSets {
		gotIDs = append(gotIDs, s.ID())
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("CodeSets() order = %q, want %q (the pin's document order)", gotIDs, wantIDs)
	}
}

func wantRubric(t *testing.T, g *terminology.Group, code, want string) {
	t.Helper()
	got, ok := g.Rubric(code)
	if !ok || got != want {
		t.Errorf("%s.Rubric(%q) = %q, %v; want %q, true", g.ID(), code, got, ok, want)
	}
}

func wantLen(t *testing.T, g *terminology.Group, want int) {
	t.Helper()
	if got := g.Len(); got != want {
		t.Errorf("%s.Len() = %d, want %d", g.ID(), got, want)
	}
}
