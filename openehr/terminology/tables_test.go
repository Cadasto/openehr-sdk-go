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

// pinPath and externalPinPath are the two vendored files of the pin, relative
// to this package directory — `go test` runs each package with its own
// directory as the working one.
const (
	pinPath         = "../../resources/terminology/openehr_terminology.xml"
	externalPinPath = "../../resources/terminology/openehr_external_terminologies.xml"
)

func TestPinnedRelease(t *testing.T) {
	if terminology.Version != "3.0.0" {
		t.Errorf("Version = %q, want 3.0.0 (the pinned openEHR TERM release)", terminology.Version)
	}
	if terminology.ID != "openehr" {
		t.Errorf("ID = %q, want openehr", terminology.ID)
	}
	for _, tc := range []struct{ name, got, path string }{
		{"SourceSHA256", terminology.SourceSHA256, pinPath},
		{"ExternalSourceSHA256", terminology.ExternalSourceSHA256, externalPinPath},
	} {
		pin, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read the pinned file: %v", err)
		}
		sum := sha256.Sum256(pin)
		if want := hex.EncodeToString(sum[:]); tc.got != want {
			t.Errorf("%s = %q, but %s hashes to %q — run 'make termgen'", tc.name, tc.got, tc.path, want)
		}
	}
}

// REQ-034: every code set of the pin, the four external ones and the three
// openEHR-issued ones, each with the issuer and the external id the pin gives
// it, and the counts of TERM Release-3.0.0.
func TestExternalCodeSetsCarryTheirIssuerAndExternalID(t *testing.T) {
	tests := []struct {
		set               *terminology.CodeSet
		id, issuer, extID string
		n                 int
	}{
		{terminology.Countries, "countries", "ISO", "ISO_3166-1", 250},
		{terminology.CharacterSets, "character_sets", "IANA", "IANA_character-sets", 14},
		{terminology.Languages, "languages", "ISO", "ISO_639-1", 253},
		{terminology.MediaTypes, "media_types", "IANA", "IANA_media-types", 107},
		{terminology.NormalStatuses, "normal_statuses", "openehr", "openehr_normal_statuses", 7},
		{terminology.CompressionAlgorithms, "compression_algorithms", "openehr", "openehr_compression_algorithms", 5},
		{terminology.IntegrityCheckAlgorithms, "integrity_check_algorithms", "openehr", "openehr_integrity_check_algorithms", 7},
	}
	for _, tc := range tests {
		if got := tc.set.ID(); got != tc.id {
			t.Errorf("ID() = %q, want %q", got, tc.id)
		}
		if got := tc.set.Issuer(); got != tc.issuer {
			t.Errorf("%s.Issuer() = %q, want %q", tc.id, got, tc.issuer)
		}
		if got := tc.set.ExternalID(); got != tc.extID {
			t.Errorf("%s.ExternalID() = %q, want %q", tc.id, got, tc.extID)
		}
		if got := tc.set.Len(); got != tc.n {
			t.Errorf("%s.Len() = %d, want %d", tc.id, got, tc.n)
		}
		if s, ok := terminology.CodeSetByID(tc.id); !ok || s != tc.set {
			t.Errorf("CodeSetByID(%q) = %v, %v; want the generated variable", tc.id, s, ok)
		}
	}
}

// REQ-034: membership of an ISO or IANA code set ignores letter case, as
// those registers do; membership of an openEHR code set is exact. A member is
// a code the pinned set lists, and nothing else: no alias the set leaves out,
// no code the live register added later, no non-ASCII look-alike.
func TestCodeSetMembershipOverThePin(t *testing.T) {
	tests := []struct {
		set  *terminology.CodeSet
		code string
		want bool
	}{
		{terminology.Languages, "en", true},
		{terminology.Languages, "EN", true},
		{terminology.Languages, "en-US", true},
		{terminology.Languages, "xx", false},
		{terminology.CharacterSets, "UTF-8", true},
		{terminology.CharacterSets, "utf-8", true},
		{terminology.CharacterSets, "iso_8859-1:1987", true},
		// ISO-8859-1 is an IANA alias of ISO_8859-1:1987 the pin does not list.
		{terminology.CharacterSets, "ISO-8859-1", false},
		{terminology.CharacterSets, "UTF-99", false},
		// U+FF18 FULLWIDTH DIGIT EIGHT.
		{terminology.CharacterSets, "utf-８", false},
		{terminology.Countries, "nl", true},
		{terminology.Countries, "NL", true},
		{terminology.Countries, "ZZ", false},
		// U+212A KELVIN SIGN, which Unicode folds to k: KE is Kenya.
		{terminology.Countries, "KE", false},
		{terminology.Countries, "ke", true},
		{terminology.MediaTypes, "text/plain", true},
		{terminology.MediaTypes, "TEXT/PLAIN", true},
		{terminology.MediaTypes, "video/jpeg", true},
		{terminology.NormalStatuses, "H", true},
		{terminology.NormalStatuses, "h", false},
		{terminology.IntegrityCheckAlgorithms, "SHA-256", true},
		{terminology.IntegrityCheckAlgorithms, "sha-256", false},
	}
	for _, tc := range tests {
		if got := tc.set.Has(tc.code); got != tc.want {
			t.Errorf("%s (issuer %q).Has(%q) = %v, want %v", tc.set.ID(), tc.set.Issuer(), tc.code, got, tc.want)
		}
	}
}

// REQ-034: a nil code set reports zero values and never panics, seen from
// outside the package.
func TestNilCodeSetIsInertFromOutside(t *testing.T) {
	var s *terminology.CodeSet
	if s.Has("en") || s.Len() != 0 || s.ID() != "" || s.Name() != "" || s.Issuer() != "" || s.ExternalID() != "" {
		t.Error("a nil *terminology.CodeSet must answer zero values")
	}
	if n := len(slices.Collect(s.All())); n != 0 {
		t.Errorf("nil All yields %d codes, want 0", n)
	}
}

func TestTablesMatchTheOpenEHRTerminology(t *testing.T) {
	groups := slices.Collect(terminology.Groups())
	codeSets := slices.Collect(terminology.CodeSets())

	// Counts pinned from TERM Release-3.0.0.
	if len(groups) != 17 {
		t.Errorf("Groups() yields %d groups, want 17", len(groups))
	}
	if len(codeSets) != 7 {
		t.Errorf("CodeSets() yields %d code sets, want 7 (3 openEHR-issued, 4 external)", len(codeSets))
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
	// 19 openEHR-issued codes, then 250 + 14 + 253 + 107 external ones.
	if codes != 643 {
		t.Errorf("the code sets hold %d codes in total, want 643", codes)
	}

	// Spot pins — the codes SDK surfaces default, validate and decode with,
	// including the members the group widening newly admits (252/666/816/817
	// for change type, 800/801 for lifecycle state). Each anchors a rubric to
	// a human-known literal independently of the pin, so a bad vendor sync
	// that transposed a rubric would fail here, not just against the XML.
	wantRubric(t, terminology.AuditChangeType, "523", "deleted")
	wantRubric(t, terminology.AuditChangeType, "253", "unknown")
	wantRubric(t, terminology.AuditChangeType, "252", "synthesis")
	wantRubric(t, terminology.AuditChangeType, "666", "attestation")
	wantRubric(t, terminology.AuditChangeType, "816", "restoration")
	wantRubric(t, terminology.AuditChangeType, "817", "format conversion")
	wantLen(t, terminology.AuditChangeType, 9)

	wantRubric(t, terminology.VersionLifecycleState, "532", "complete")
	wantRubric(t, terminology.VersionLifecycleState, "800", "inactive")
	wantRubric(t, terminology.VersionLifecycleState, "801", "abandoned")
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
	// openehr_terminology.xml's code sets first, then
	// openehr_external_terminologies.xml's, each in its document order.
	wantIDs := []string{
		"compression_algorithms", "integrity_check_algorithms", "normal_statuses",
		"countries", "character_sets", "languages", "media_types",
	}
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
