// Example: build an AQL query two ways — the struct-builder and the
// verb-functions — and show that both emit byte-identical, canonical AQL
// (REQ-055, PROBE-020). Pure building block (REQ-013): no transport, no auth,
// no client. The executor lives at openehr/client/query.
//
// Surfaces shown:
//   - aql.NewBuilder() struct style with Select / FromEHR / Contains / Where
//   - aql.Select(...) verb style producing the same wire string
//   - aql.Param for safe placeholders (never interpolate caller data)
//   - WHERE composition with aql.And / aql.Gt / comparison helpers
//   - the REQ-117 containment algebra (aql.Class / Contains / NotContains /
//     ContainsOr) and opt-in in-text paging (LimitInline / OffsetInline)
//
// Run:
//
//	go run ./cmd/examples/aql-build
package main

import (
	"fmt"
	"log"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

func main() {
	const archetype = "openEHR-EHR-OBSERVATION.body_temperature.v2"

	const magnitude = "o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude"

	// Struct-builder style. FromEHR scopes the query to one EHR via a WHERE
	// condition; consecutive Contains express nested containment.
	structQ, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", "o", archetype)).
		Where(aql.Gt(magnitude, aql.Real(37.5))).
		Build()
	if err != nil {
		log.Fatalf("struct-builder: %v", err)
	}

	// Verb-functions style — same construction, different entry point; the
	// emitter fixes clause order, so SELECT/FROM/WHERE land identically.
	verbQ, err := aql.Select(aql.Col("o")).
		Where(aql.Gt(magnitude, aql.Real(37.5))).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", "o", archetype)).
		Build()
	if err != nil {
		log.Fatalf("verb-functions: %v", err)
	}

	// REQ-117 containment algebra: a COMPOSITION containing EITHER a
	// body-temperature OBSERVATION that does NOT itself contain a CLUSTER, OR
	// any EVALUATION. Every combinator returns a NEW Containment, so operands
	// compose as values and one can be reused across expressions. A junction
	// is parenthesised only where the grouping is load-bearing, and NOT
	// attaches to a CONTAINS connector (NotContains) — never to a junction
	// operand, because the grammar admits NOT only as `NOT? CONTAINS`.
	//
	// Paging goes in-text here (LimitInline / OffsetInline) instead of the
	// request envelope, so the bound survives stored-query registration. The
	// two channels are mutually exclusive: setting Limit/Offset as well would
	// make Build return an error wrapping aql.ErrInvalidQuery.
	algebraQ, err := aql.NewBuilder().
		Select(aql.Col("c")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
			aql.Archetype("OBSERVATION", "o", archetype).NotContains(aql.Class("CLUSTER", "cl")),
			aql.Class("EVALUATION", "ev"),
		))).
		OrderBy("c/context/start_time/value", aql.Descending).
		LimitInline(20).
		OffsetInline(40).
		Build()
	if err != nil {
		log.Fatalf("containment algebra: %v", err)
	}

	fmt.Println("struct-builder :", structQ)
	fmt.Println("verb-functions :", verbQ)
	fmt.Println("byte-identical :", structQ.String() == verbQ.String())
	fmt.Println()
	fmt.Println("containment algebra + in-text paging (REQ-117):")
	fmt.Println(" ", algebraQ)
	fmt.Println("  envelope paging unused — Fetch/Offset stay zero:", algebraQ.Fetch, algebraQ.Offset)
}
