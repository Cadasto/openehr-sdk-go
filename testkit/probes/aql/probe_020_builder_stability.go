package aqlprobes

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// Probe020BuilderStability asserts that the struct-builder and the
// verb-functions emit byte-identical AQL for the reference query, and that the
// shared output equals the supplied golden string (REQ-055, PROBE-020).
//
// The reference query is "all OBSERVATIONs of archetype body_temperature for a
// given EHR". golden is the checked-in canonical form from
// openehr/aql/testdata/wire/observations_by_archetype.aql; the caller reads it
// (probes take no filesystem dependency).
func Probe020BuilderStability(golden string) (Result, error) {
	r := Result{Probe: "PROBE-020"}

	const (
		alias       = "o"
		archetypeID = "openEHR-EHR-OBSERVATION.body_temperature.v2"
	)

	structQ, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", alias, archetypeID)).
		Build()
	if err != nil {
		return r, fmt.Errorf("PROBE-020: struct-builder: %w", err)
	}

	verbQ, err := aql.Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", alias, archetypeID)).
		Build()
	if err != nil {
		return r, fmt.Errorf("PROBE-020: verb-functions: %w", err)
	}

	switch {
	case structQ.String() != verbQ.String():
		r.Status = "fail"
		r.Detail = fmt.Sprintf("struct vs verb diverge:\n struct: %q\n   verb: %q", structQ.String(), verbQ.String())
	case structQ.String() != golden:
		r.Status = "fail"
		r.Detail = fmt.Sprintf("output does not match golden:\n  built: %q\n golden: %q", structQ.String(), golden)
	default:
		r.Status = "pass"
	}
	return r, nil
}
