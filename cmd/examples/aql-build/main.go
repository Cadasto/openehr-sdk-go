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

	fmt.Println("struct-builder :", structQ)
	fmt.Println("verb-functions :", verbQ)
	fmt.Println("byte-identical :", structQ.String() == verbQ.String())
}
