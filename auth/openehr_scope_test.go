package auth_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// validScopes are scopes Token must write, with the token it must return.
// The patterns sit on every edge of the RFC 6749 §3.3 scope-token set and
// carry the separators ParseOpenEHRScope splits on, so the same rows pin the
// round trip in both directions (REQ-165).
var validScopes = []struct {
	name  string
	scope auth.OpenEHRScope
	want  string
}{
	{
		name:  "patient composition wildcard",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "composition", Pattern: "*", Permissions: "rs"},
		want:  "patient/composition-*.rs",
	},
	{
		name:  "user aql dotted query name",
		scope: auth.OpenEHRScope{Compartment: "user", Resource: "aql", Pattern: "org.openehr::bloodpressure.v1", Permissions: "s"},
		want:  "user/aql-org.openehr::bloodpressure.v1.s",
	},
	{
		name:  "system template dashed id",
		scope: auth.OpenEHRScope{Compartment: "system", Resource: "template", Pattern: "My-Template.v2", Permissions: "cruds"},
		want:  "system/template-My-Template.v2.cruds",
	},
	{
		name:  "wildcard inside a pattern is kept as written",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "template", Pattern: "openEHR-EHR-COMPOSITION.*.v1", Permissions: "r"},
		want:  "patient/template-openEHR-EHR-COMPOSITION.*.v1.r",
	},
	{
		name:  "pattern with a slash",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "template", Pattern: "a/b", Permissions: "r"},
		want:  "patient/template-a/b.r",
	},
	{
		name:  "pattern starting with a dash",
		scope: auth.OpenEHRScope{Compartment: "user", Resource: "composition", Pattern: "-x", Permissions: "u"},
		want:  "user/composition--x.u",
	},
	{
		name:  "pattern ending with a dot",
		scope: auth.OpenEHRScope{Compartment: "user", Resource: "composition", Pattern: "x.", Permissions: "d"},
		want:  "user/composition-x..d",
	},
	{
		name:  "pattern that looks like permissions",
		scope: auth.OpenEHRScope{Compartment: "system", Resource: "aql", Pattern: "q.rs", Permissions: "c"},
		want:  "system/aql-q.rs.c",
	},
	{
		name:  "lowest scope-token byte 0x21",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "aql", Pattern: "!", Permissions: "s"},
		want:  "patient/aql-!.s",
	},
	{
		name:  "byte 0x23 after the excluded double quote",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "aql", Pattern: "#", Permissions: "s"},
		want:  "patient/aql-#.s",
	},
	{
		name:  "byte 0x5B before the excluded backslash",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "aql", Pattern: "[", Permissions: "s"},
		want:  "patient/aql-[.s",
	},
	{
		name:  "byte 0x5D after the excluded backslash",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "aql", Pattern: "]", Permissions: "s"},
		want:  "patient/aql-].s",
	},
	{
		name:  "highest scope-token byte 0x7E",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "aql", Pattern: "~", Permissions: "s"},
		want:  "patient/aql-~.s",
	},
	{
		name:  "non-adjacent permissions in order",
		scope: auth.OpenEHRScope{Compartment: "patient", Resource: "composition", Pattern: "x", Permissions: "cds"},
		want:  "patient/composition-x.cds",
	},
}

// withPart returns a valid scope with one part replaced by the caller.
func withPart(mod func(*auth.OpenEHRScope)) auth.OpenEHRScope {
	s := auth.OpenEHRScope{Compartment: "patient", Resource: "composition", Pattern: "x", Permissions: "rs"}
	mod(&s)
	return s
}

// TestREQ165_TokenWritesValidScopes pins the token Token returns for every
// part combination the openEHR resource-scope grammar allows (REQ-165).
func TestREQ165_TokenWritesValidScopes(t *testing.T) {
	t.Parallel()
	for _, tc := range validScopes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.scope.Token()
			if err != nil {
				t.Fatalf("%#v.Token() error = %v, want nil", tc.scope, err)
			}
			if got != tc.want {
				t.Errorf("%#v.Token() = %q, want %q", tc.scope, got, tc.want)
			}
		})
	}
}

// TestREQ165_TokenRefusesInvalidParts pins every refusal rule of Token: each
// row breaks one rule, and the error must match ErrInvalidScope, name the
// broken rule, and come with no token. Deleting any one check lets its rows
// through or changes the rule they report (REQ-165).
func TestREQ165_TokenRefusesInvalidParts(t *testing.T) {
	t.Parallel()
	const (
		compartment = "compartment is not"
		resource    = "resource is not"
		emptyPat    = "pattern is empty"
		badPat      = "pattern contains a character outside"
		emptyPerm   = "permissions are empty"
		outside     = "permissions use a letter outside"
		repeat      = "permissions repeat a letter"
		order       = "permissions are out of order"
	)
	cases := []struct {
		name  string
		scope auth.OpenEHRScope
		rule  string
	}{
		{"empty compartment", withPart(func(s *auth.OpenEHRScope) { s.Compartment = "" }), compartment},
		{"capitalised compartment", withPart(func(s *auth.OpenEHRScope) { s.Compartment = "Patient" }), compartment},
		{"padded compartment", withPart(func(s *auth.OpenEHRScope) { s.Compartment = " patient" }), compartment},
		{"launch compartment", withPart(func(s *auth.OpenEHRScope) { s.Compartment = "launch" }), compartment},
		{"empty resource", withPart(func(s *auth.OpenEHRScope) { s.Resource = "" }), resource},
		{"capitalised resource", withPart(func(s *auth.OpenEHRScope) { s.Resource = "Composition" }), resource},
		{"FHIR resource", withPart(func(s *auth.OpenEHRScope) { s.Resource = "Observation" }), resource},
		{"ehr resource", withPart(func(s *auth.OpenEHRScope) { s.Resource = "ehr" }), resource},
		{"resource carrying a dash", withPart(func(s *auth.OpenEHRScope) { s.Resource = "template-x" }), resource},
		{"empty pattern", withPart(func(s *auth.OpenEHRScope) { s.Pattern = "" }), emptyPat},
		{"space 0x20", withPart(func(s *auth.OpenEHRScope) { s.Pattern = "Vital signs" }), badPat},
		{"double quote 0x22", withPart(func(s *auth.OpenEHRScope) { s.Pattern = `a"b` }), badPat},
		{"backslash 0x5C", withPart(func(s *auth.OpenEHRScope) { s.Pattern = `a\b` }), badPat},
		{"delete 0x7F", withPart(func(s *auth.OpenEHRScope) { s.Pattern = "a\x7f" }), badPat},
		{"tab", withPart(func(s *auth.OpenEHRScope) { s.Pattern = "a\tb" }), badPat},
		{"NUL", withPart(func(s *auth.OpenEHRScope) { s.Pattern = "\x00" }), badPat},
		{"non-ASCII", withPart(func(s *auth.OpenEHRScope) { s.Pattern = "bloeddruk-é" }), badPat},
		{"empty permissions", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "" }), emptyPerm},
		{"letter outside cruds", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "x" }), outside},
		{"capital letter", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "R" }), outside},
		{"wildcard permission", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "*" }), outside},
		{"SMART v1 word", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "read" }), outside},
		{"repeated letter", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "rr" }), repeat},
		{"out of order", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "sr" }), order},
		{"repeat after another letter", withPart(func(s *auth.OpenEHRScope) { s.Permissions = "rsr" }), order},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.scope.Token()
			if !errors.Is(err, auth.ErrInvalidScope) {
				t.Fatalf("%#v.Token() error = %v, want one matching ErrInvalidScope", tc.scope, err)
			}
			if !strings.Contains(err.Error(), tc.rule) {
				t.Errorf("%#v.Token() error = %q, want it to say %q", tc.scope, err, tc.rule)
			}
			if got != "" {
				t.Errorf("%#v.Token() token = %q, want \"\" alongside the error", tc.scope, got)
			}
		})
	}
}

// TestREQ165_TokenErrorQuotesNoCallerValue pins that a refusal names the
// broken part without repeating what the caller put in it, so a template id
// or query name never travels into logs through the error (REQ-165).
func TestREQ165_TokenErrorQuotesNoCallerValue(t *testing.T) {
	t.Parallel()
	const value = "zz caller value zz"
	cases := map[string]auth.OpenEHRScope{
		"compartment": withPart(func(s *auth.OpenEHRScope) { s.Compartment = value }),
		"resource":    withPart(func(s *auth.OpenEHRScope) { s.Resource = value }),
		"pattern":     withPart(func(s *auth.OpenEHRScope) { s.Pattern = value }),
		"permissions": withPart(func(s *auth.OpenEHRScope) { s.Permissions = value }),
	}
	for part, scope := range cases {
		t.Run(part, func(t *testing.T) {
			t.Parallel()
			_, err := scope.Token()
			if err == nil {
				t.Fatalf("%#v.Token() error = nil, want one naming the %s", scope, part)
			}
			if !strings.Contains(err.Error(), part) || strings.Contains(err.Error(), "zz") {
				t.Errorf("%#v.Token() error = %q, want it to name the %s without quoting its value", scope, err, part)
			}
		})
	}
}

// TestREQ165_ParseReadsScopeTokens pins how ParseOpenEHRScope splits a
// token: the compartment ends at the first "/", the permissions start after
// the last "." and the resource ends at the first "-" (REQ-165).
func TestREQ165_ParseReadsScopeTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		token string
		want  auth.OpenEHRScope
	}{
		{"patient/composition-*.rs", auth.OpenEHRScope{Compartment: "patient", Resource: "composition", Pattern: "*", Permissions: "rs"}},
		{"user/aql-org.openehr::bloodpressure.v1.s", auth.OpenEHRScope{Compartment: "user", Resource: "aql", Pattern: "org.openehr::bloodpressure.v1", Permissions: "s"}},
		{"system/template-My-Template.v2.cruds", auth.OpenEHRScope{Compartment: "system", Resource: "template", Pattern: "My-Template.v2", Permissions: "cruds"}},
		{"patient/template-a/b.r", auth.OpenEHRScope{Compartment: "patient", Resource: "template", Pattern: "a/b", Permissions: "r"}},
	}
	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			t.Parallel()
			got, ok := auth.ParseOpenEHRScope(tc.token)
			if !ok || got != tc.want {
				t.Errorf("ParseOpenEHRScope(%q) = %#v, %v; want %#v, true", tc.token, got, ok, tc.want)
			}
		})
	}
}

// TestREQ165_ParseRefusesOtherTokens pins that ParseOpenEHRScope answers
// false with the zero scope, and never panics, for anything that is not an
// openEHR resource scope (REQ-165).
func TestREQ165_ParseRefusesOtherTokens(t *testing.T) {
	t.Parallel()
	tokens := []string{
		"openid",
		"launch/patient",
		"offline_access",
		"patient/Observation.rs",
		"patient/*.read",
		"patient/composition-.rs",
		"patient/composition-x.sr",
		"patient/composition-x.rr",
		"patient/composition-x",
		"patient/composition-x.",
		"patient/composition",
		"patient/compositionx.r",
		"Patient/composition-x.r",
		"patient/Composition-x.r",
		"launch/composition-x.r",
		"patient//composition-x.r",
		"patient.rs/composition-x",
		"a.b/c",
		" patient/composition-x.r",
		"patient/composition-x.r ",
		"patient/composition-x.r\n",
		"patient/composition-a b.r",
		"patient/composition-x.rs openid",
		`patient/composition-"x".r`,
		`patient/composition-a\b.r`,
		"patient/composition-é.r",
		"",
		"/",
		".",
		"-",
		"/-.",
		"patient/-.r",
		"patient/.r",
	}
	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			t.Parallel()
			got, ok := auth.ParseOpenEHRScope(tok)
			if ok || got != (auth.OpenEHRScope{}) {
				t.Errorf("ParseOpenEHRScope(%q) = %#v, %v; want the zero scope, false", tok, got, ok)
			}
		})
	}
}

// TestREQ165_TokenAndParseRoundTrip checks the two functions against each
// other: every token Token writes parses back to the scope it came from, and
// a parsed scope writes back the token it was read from (REQ-165).
func TestREQ165_TokenAndParseRoundTrip(t *testing.T) {
	t.Parallel()
	for _, tc := range validScopes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tok, err := tc.scope.Token()
			if err != nil {
				t.Fatalf("%#v.Token() error = %v, want nil", tc.scope, err)
			}
			got, ok := auth.ParseOpenEHRScope(tok)
			if !ok || got != tc.scope {
				t.Fatalf("ParseOpenEHRScope(%q) = %#v, %v; want %#v, true", tok, got, ok, tc.scope)
			}
			back, err := got.Token()
			if err != nil || back != tok {
				t.Errorf("ParseOpenEHRScope(%q).Token() = %q, %v; want %q, nil", tok, back, err, tok)
			}
		})
	}
}

func ExampleOpenEHRScope_Token() {
	read := auth.OpenEHRScope{Compartment: "patient", Resource: "composition", Pattern: "*", Permissions: "rs"}
	tok, err := read.Token()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(auth.JoinScopes(auth.ScopeOpenID, auth.ScopeLaunchPatient, tok))

	// A template id with a space cannot be written as a scope token.
	_, err = auth.OpenEHRScope{Compartment: "user", Resource: "template", Pattern: "Vital signs", Permissions: "r"}.Token()
	fmt.Println(errors.Is(err, auth.ErrInvalidScope))
	// Output:
	// openid launch/patient patient/composition-*.rs
	// true
}

func ExampleParseOpenEHRScope() {
	granted := []string{"openid", "user/aql-org.openehr::bloodpressure.v1.s", "patient/Observation.rs"}
	for _, tok := range granted {
		s, ok := auth.ParseOpenEHRScope(tok)
		if !ok {
			fmt.Printf("%s: not an openEHR resource scope\n", tok)
			continue
		}
		fmt.Printf("%s: compartment %s, resource %s, pattern %s, permissions %s\n",
			tok, s.Compartment, s.Resource, s.Pattern, s.Permissions)
	}
	// Output:
	// openid: not an openEHR resource scope
	// user/aql-org.openehr::bloodpressure.v1.s: compartment user, resource aql, pattern org.openehr::bloodpressure.v1, permissions s
	// patient/Observation.rs: not an openEHR resource scope
}
