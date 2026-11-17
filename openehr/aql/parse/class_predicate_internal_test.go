package parse

// class_predicate_internal_test.go: REQ-119 § Emit-side structural parity,
// the one Archetype-and-Predicate pair a class carries. The VERSION and $param
// conditions of namedArchetypePair are also refused by later guards of
// checkClassOperands, so Emit alone cannot show either condition failing; this
// pins them on the helper that defines the pair.

import "testing"

func TestREQ119_NamedArchetypePairConditions(t *testing.T) {
	t.Parallel()
	const hrid = "openEHR-EHR-OBSERVATION.blood_pressure.v1"
	named := hrid + ", 'x'"
	cases := []struct {
		name  string
		class ClassExpr
		want  bool
	}{
		{"the named form", ClassExpr{Archetype: hrid, Predicate: named}, true},
		{"trivia inside", ClassExpr{Archetype: hrid, Predicate: " " + hrid + " -- c\n, 'x' "}, true},
		{"VERSION class", ClassExpr{Version: true, Archetype: hrid, Predicate: named}, false},
		{"$param archetype", ClassExpr{ParamArchetype: true, Archetype: hrid, Predicate: named}, false},
		{"another archetype", ClassExpr{Archetype: "openEHR-EHR-OBSERVATION.lab_result.v1", Predicate: named}, false},
		{"no archetype", ClassExpr{Predicate: named}, false},
		{"not the named form", ClassExpr{Archetype: hrid, Predicate: "name/value='x'"}, false},
		{"no archetype, not the named form", ClassExpr{Predicate: "name/value='x'"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := namedArchetypePair(tc.class); got != tc.want {
				t.Errorf("namedArchetypePair(Archetype %q, Predicate %q, Version %v, ParamArchetype %v) = %v; want %v",
					tc.class.Archetype, tc.class.Predicate, tc.class.Version, tc.class.ParamArchetype, got, tc.want)
			}
		})
	}
}

// TestREQ119_NamedArchetypeTextReadsTokensNotText pins the lexical reading:
// three tokens of the named form and nothing after them, any skipped trivia
// between them, and a lexer error refused.
func TestREQ119_NamedArchetypeTextReadsTokensNotText(t *testing.T) {
	t.Parallel()
	const hrid = "openEHR-EHR-OBSERVATION.blood_pressure.v1"
	cases := map[string]string{
		hrid + ", 'x'":                       hrid,
		hrid + `,"x"`:                        hrid,
		hrid + ", $p":                        hrid,
		hrid + ", SNOMED-CT::1|a, b|":        hrid,
		hrid + ", at0004":                    hrid,
		hrid + ", id4":                       hrid,
		"\n" + hrid + "\t-- c\n,\n'x'\n":     hrid,
		hrid:                                 "",
		hrid + ",":                           "",
		hrid + " 'x'":                        "",
		hrid + ", 'x', 'y'":                  "",
		hrid + ", 'x' and at0001":            "",
		hrid + ", 'x']":                      "",
		hrid + ", 'x":                        "",
		hrid + ", 'x' ~":                     "",
		hrid + ", name/value":                "",
		"at0001, 'x'":                        "",
		"$p, 'x'":                            "",
		"":                                   "",
		"'x', " + hrid:                       "",
		hrid + ", " + hrid:                   "",
		"name/value = 'x'":                   "",
		hrid + ", 'x' -- trailing comment":   hrid,
		hrid + ", 'x' -- trailing comment\n": hrid,
	}
	for text, want := range cases {
		if got := namedArchetypeText(text); got != want {
			t.Errorf("namedArchetypeText(%q) = %q; want %q", text, got, want)
		}
	}
}
