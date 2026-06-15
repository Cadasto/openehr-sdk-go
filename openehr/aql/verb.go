package aql

// Verb-functions are the second builder style of REQ-055. Each is a top-level
// entry point that returns a *[Builder]; the remaining clauses chain as
// methods. Because both styles share the same internal emitter, they produce
// byte-identical AQL for the same logical query (PROBE-020) — and clause call
// order never affects the output, since the emitter canonicalises ordering.
//
//	q, err := aql.Select(aql.Col("o")).
//	    FromEHR("e", aql.Param("ehr_id")).
//	    Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2")).
//	    Build()

// Select starts a verb-style query with the given projection list.
func Select(cols ...SelectField) *Builder { return NewBuilder().Select(cols...) }

// From starts a verb-style query from an RM class, e.g. From("COMPOSITION", "c").
func From(rmType, alias string) *Builder { return NewBuilder().From(rmType, alias) }

// FromEHR starts a verb-style query from an EHR scoped by ehr_id.
func FromEHR(alias string, id Value) *Builder { return NewBuilder().FromEHR(alias, id) }

// Where starts a verb-style query with the given predicate.
func Where(e WhereExpr) *Builder { return NewBuilder().Where(e) }
