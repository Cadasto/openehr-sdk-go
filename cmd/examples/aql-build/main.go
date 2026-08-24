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
//   - the REQ-162 opt-in RM-semantics gate (Builder.VerifyContainment), which
//     answers a question Build deliberately does not
//
// Run:
//
//	go run ./cmd/examples/aql-build
package main

import (
	"fmt"
	"log"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
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
	algebraB := aql.NewBuilder().
		Select(aql.Col("c")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
			aql.Archetype("OBSERVATION", "o", archetype).NotContains(aql.Class("CLUSTER", "cl")),
			aql.Class("EVALUATION", "ev"),
		))).
		OrderBy("c/context/start_time/value", aql.Descending).
		LimitInline(20).
		OffsetInline(40)
	algebraQ, err := algebraB.Build()
	if err != nil {
		log.Fatalf("containment algebra: %v", err)
	}

	// REQ-162: the opt-in RM-semantics gate. Build answers the SHAPE question
	// only — is this representable, canonical AQL — so a query whose classes can
	// never contain one another still builds and still emits. Asking the RM
	// question is the caller's choice, and this is how it is asked; nil means
	// the REQ-160 default containment relation (pass a
	// contain.Default().WithOverlay(…) copy for a deployment whose dialect
	// admits more).
	//
	// The query below is grammatically perfect and semantically dead: no
	// containment route connects OBSERVATION to EVALUATION, and the archetype
	// predicate names an OBSERVATION archetype on an EVALUATION. Build accepts
	// it — a caller is entitled to send it anyway, probing a dialect or
	// reproducing a bug report — and the verification says why it can never
	// match.
	deadB := aql.NewBuilder().
		Select(aql.Col("ev")).
		From("OBSERVATION", "o").
		Contains(aql.Archetype("EVALUATION", "ev", archetype))
	deadQ, err := deadB.Build()
	if err != nil {
		log.Fatalf("RM-impossible query: %v", err) // it builds: REQ-162 leaves Build unchanged
	}

	fmt.Println("struct-builder :", structQ)
	fmt.Println("verb-functions :", verbQ)
	fmt.Println("byte-identical :", structQ.String() == verbQ.String())
	fmt.Println()
	fmt.Println("containment algebra + in-text paging (REQ-117):")
	fmt.Println(" ", algebraQ)
	fmt.Println("  envelope paging unused — Fetch/Offset stay zero:", algebraQ.Fetch, algebraQ.Offset)
	fmt.Println()
	fmt.Println("containment verification (REQ-162) — opt-in; Build never runs it:")
	verify("containment algebra", algebraQ, algebraB.VerifyContainment(nil))
	verify("RM-impossible query", deadQ, deadB.VerifyContainment(nil))
}

// verify prints one VerifyContainment result. A finding carries a value-free
// Code and a value-bearing Detail and NO severity — a builder tree has no source
// text to point into, and each code's severity is fixed once, in the REQ-161
// catalogue — so the code is what a caller dispatches on, and what is printed.
func verify(label string, q aql.Query, findings []contain.Finding) {
	fmt.Printf("  == %s ==\n  %s\n", label, q)
	if len(findings) == 0 {
		fmt.Print("  result : no findings — every containment step is admissible under the pinned RM\n\n")
		return
	}
	for _, f := range findings {
		fmt.Printf("  %s\n    %s\n", f.Code, f.Detail)
	}
	fmt.Println()
}
