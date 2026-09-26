// This example builds AQL queries with the openehr/aql package and prints the
// query text each one produces. AQL (Archetype Query Language) is the openEHR
// query language. The builder only produces text and never talks to a server,
// so the program runs offline with no fixture and no clinical data repository
// (CDR); executing a built query is the job of openehr/client/query.
//
// It shows the two builder styles (a chained Builder and free-standing verb
// functions) producing byte-identical text, the containment algebra for nested
// CONTAINS clauses together with in-text LIMIT / OFFSET paging, and the opt-in
// check that asks whether the classes in a query can contain one another
// under the openEHR Reference Model (RM).
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

// bodyTemperature is the archetype the sample queries look for. An archetype
// id names a reusable clinical model; this one is the OBSERVATION for a body
// temperature reading.
const bodyTemperature = "openEHR-EHR-OBSERVATION.body_temperature.v2"

// magnitudePath is the RM path, starting at the alias "o", of the measured
// temperature inside that archetype.
const magnitudePath = "o/data[at0001]/events[at0006]/data/items[at0004]/value/magnitude"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run tells the story in three sections: the same query built two ways, a
// query with nested containment and in-text paging, and the RM check a caller
// can opt into after building.
func run() error {
	if err := buildTwoWays(); err != nil {
		return err
	}
	fmt.Println()

	// Section 2. Both the builder and the query it builds are kept: the
	// verification in section 3 runs on the builder's tree, and the query
	// text is printed next to the findings.
	algebra := containmentAlgebra()
	algebraQuery, err := algebra.Build()
	if err != nil {
		return fmt.Errorf("build containment-algebra query: %w", err)
	}
	fmt.Println("containment algebra + in-text paging:")
	fmt.Println(" ", algebraQuery)
	// LimitInline / OffsetInline put the bounds in the text, so the envelope
	// fields of the built Query stay at their zero values.
	fmt.Println("  envelope paging unused — Fetch/Offset stay zero:", algebraQuery.Fetch, algebraQuery.Offset)
	fmt.Println()

	// Section 3. Build only answers the shape question (is this well-formed
	// AQL?), so the RM-impossible query below builds and emits like any
	// other. Whether its classes can contain one another is a separate
	// question, asked with VerifyContainment and only when the caller wants
	// it: a caller may send such a query on purpose, to probe a server or
	// to reproduce a bug report.
	dead := rmImpossibleQuery()
	deadQuery, err := dead.Build()
	if err != nil {
		return fmt.Errorf("build RM-impossible query: %w", err)
	}
	fmt.Println("containment verification (opt-in; Build never runs it):")
	// VerifyContainment walks the builder's own tree, so it needs no built
	// Query. The nil argument selects the default containment relation of
	// the pinned RM; a deployment whose CDR allows extra routes passes
	// contain.Default().WithOverlay(...) instead.
	printFindings("containment algebra", algebraQuery, algebra.VerifyContainment(nil))
	printFindings("RM-impossible query", deadQuery, dead.VerifyContainment(nil))
	return nil
}

// buildTwoWays builds one logical query with each builder style and shows
// that the emitted AQL is the same string.
func buildTwoWays() error {
	// Struct-builder style: start from NewBuilder and chain the clauses.
	// FromEHR scopes the query to one EHR by adding a WHERE condition on
	// e/ehr_id/value. Each Contains nests one level deeper: the COMPOSITION
	// sits inside the EHR and the OBSERVATION inside the COMPOSITION.
	// aql.Param("ehr_id") emits a $ehr_id placeholder that is bound at
	// execution time; never paste caller data into the query text.
	structQuery, err := aql.NewBuilder().
		Select(aql.Col("o")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", "o", bodyTemperature)).
		Where(aql.Gt(magnitudePath, aql.Real(37.5))).
		Build()
	if err != nil {
		return fmt.Errorf("build struct-style query: %w", err)
	}

	// Verb-function style: the same clauses, entered through package-level
	// functions. The clauses are given in a different order on purpose; the
	// emitter fixes the clause order, so the text still comes out identical.
	verbQuery, err := aql.Select(aql.Col("o")).
		Where(aql.Gt(magnitudePath, aql.Real(37.5))).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Archetype("COMPOSITION", "c", "")).
		Contains(aql.Archetype("OBSERVATION", "o", bodyTemperature)).
		Build()
	if err != nil {
		return fmt.Errorf("build verb-style query: %w", err)
	}

	fmt.Println("struct-builder :", structQuery)
	fmt.Println("verb-functions :", verbQuery)
	fmt.Println("byte-identical :", structQuery.String() == verbQuery.String())
	return nil
}

// containmentAlgebra prepares a query whose CONTAINS clause is a small
// expression tree: a COMPOSITION that contains either a body-temperature
// OBSERVATION with no CLUSTER inside it, or any EVALUATION.
//
// Every combinator (Contains, NotContains, ContainsOr) returns a new
// Containment value, so operands can be built separately and reused. The
// emitter adds parentheses only where the grouping matters, and NOT always
// attaches to a CONTAINS connector (NotContains) because the grammar allows
// "NOT CONTAINS" and never a NOT in front of a group.
//
// LimitInline and OffsetInline write LIMIT and OFFSET into the query text.
// The default channel is the request envelope (Limit / Offset, which land in
// Query.Fetch / Query.Offset); the in-text form is for a query that will be
// registered as a stored query, where only the text survives. Setting both
// channels makes Build return an error wrapping aql.ErrInvalidQuery.
func containmentAlgebra() *aql.Builder {
	return aql.NewBuilder().
		Select(aql.Col("c")).
		FromEHR("e", aql.Param("ehr_id")).
		Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
			aql.Archetype("OBSERVATION", "o", bodyTemperature).NotContains(aql.Class("CLUSTER", "cl")),
			aql.Class("EVALUATION", "ev"),
		))).
		OrderBy("c/context/start_time/value", aql.Descending).
		LimitInline(20).
		OffsetInline(40)
}

// rmImpossibleQuery prepares a query that is well-formed AQL and can never
// return a row: under the RM no containment route connects OBSERVATION to
// EVALUATION, and the archetype id names an OBSERVATION archetype on an
// EVALUATION class.
func rmImpossibleQuery() *aql.Builder {
	return aql.NewBuilder().
		Select(aql.Col("ev")).
		From("OBSERVATION", "o").
		Contains(aql.Archetype("EVALUATION", "ev", bodyTemperature))
}

// printFindings prints the query and one VerifyContainment result for it. A
// Finding carries a stable Code, which is what a program dispatches on, and
// a free-text Detail. It carries no severity and no position: a builder tree
// has no source text to point into, and each code's severity is fixed in the
// lint catalogue (the VerifyContainment doc lists them).
func printFindings(label string, query aql.Query, findings []contain.Finding) {
	fmt.Printf("  == %s ==\n  %s\n", label, query)
	if len(findings) == 0 {
		fmt.Print("  result : no findings — every containment step is admissible under the pinned RM\n\n")
		return
	}
	for _, finding := range findings {
		fmt.Printf("  %s\n    %s\n", finding.Code, finding.Detail)
	}
	fmt.Println()
}
