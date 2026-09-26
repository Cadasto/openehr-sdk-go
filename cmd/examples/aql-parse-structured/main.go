// This example parses AQL text into the structured query tree that the
// openehr/aql/parse package returns, prints each clause of that tree, and
// emits the tree back to canonical AQL text. AQL (Archetype Query Language)
// is the openEHR query language. Parsing needs no server and no fixture, so
// the program runs offline.
//
// Run it with no argument to walk three built-in queries, or pass your own
// query as the argument:
//
//	go run ./cmd/examples/aql-parse-structured
//	go run ./cmd/examples/aql-parse-structured "SELECT c FROM EHR e CONTAINS COMPOSITION c WHERE c/uid/value = \$id"
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// basicQuery uses the clauses most queries have: a SELECT projection, a
// CONTAINS chain under the EHR, a WHERE with two conditions, ORDER BY, and
// LIMIT / OFFSET paging.
const basicQuery = `SELECT
  c/uid/value,
  c/name/value
FROM EHR e
  CONTAINS COMPOSITION c
WHERE c/uid/value = $cid AND c/name/value LIKE 'Vital%'
ORDER BY c/uid/value DESC
LIMIT 50 OFFSET 100`

// mixedSelectQuery has the shapes a simple walker gets wrong: a `*` mixed with
// column projections, a projected literal (`1 AS rank`), a function call used
// both as a projection and as the left side of a comparison, a path on the
// right side of a comparison, and a boolean junction (`OR`) at the FROM root
// where most queries have a single root class.
const mixedSelectQuery = `SELECT *, 1 AS rank, LENGTH(c/name/value)
FROM COMPOSITION c OR EHR e
WHERE LENGTH(c/name/value) > $min AND c/uid/value = c/name/value`

// deprecatedTopQuery shows two details of the read side.
//
// `TOP 5 BACKWARD` is the deprecated row limit: openEHR AQL 1.1.0 replaces it
// with LIMIT plus ORDER BY. The parser keeps it instead of refusing it, since
// an SDK does not author the queries it is handed; the linter is what reports
// the deprecation.
//
// `1.50` and `"quoted"` are literals whose source text differs from the
// canonical rendering (`1.5`, `'quoted'`). The tree keeps the text as written
// in parse.LiteralExpr.Raw, because the openEHR result set names an unaliased
// column by its expression text, while emission stays canonical.
const deprecatedTopQuery = `SELECT TOP 5 BACKWARD c/uid/value, 1.50, "quoted"
FROM COMPOSITION c
ORDER BY c/uid/value DESC`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// A query on the command line replaces the built-in tour.
	if args := os.Args[1:]; len(args) > 0 {
		return parseAndPrint(strings.Join(args, " "))
	}

	if err := parseAndPrint(basicQuery); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("--- mixed SELECT list, function calls, FROM-root junction ---")
	fmt.Println()
	if err := parseAndPrint(mixedSelectQuery); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("--- deprecated SELECT TOP and literal source text ---")
	fmt.Println()
	return parseAndPrint(deprecatedTopQuery)
}

// parseAndPrint parses one query, prints its tree clause by clause, and then
// prints the canonical text that Emit produces from the tree.
func parseAndPrint(source string) error {
	fmt.Println("input AQL:")
	for line := range strings.SplitSeq(source, "\n") {
		fmt.Println("  " + line)
	}
	fmt.Println()

	// ParseQuery checks the text against the SDK grammar and returns the
	// structured tree. A syntax error comes back wrapping aql.ErrSyntax. A
	// query that parses but uses a shape the tree cannot hold comes back as
	// a partial tree plus an error wrapping aql.ErrIncompleteAST; this
	// example treats both as fatal.
	parsed, err := parse.ParseQuery(source)
	if err != nil {
		return fmt.Errorf("parse query: %w", err)
	}

	fmt.Println("structured AST:")
	printSelect(parsed.Select)
	printFrom(parsed.From)
	printWhere(parsed.Where)
	printOrderBy(parsed.OrderBy)
	printPaging(parsed.Limit, parsed.Offset)
	fmt.Println()

	// Emit renders the tree back to canonical AQL. It re-parses its own
	// output before returning it, so the text it hands back always parses.
	emitted, err := parsed.Emit()
	if err != nil {
		return fmt.Errorf("emit query: %w", err)
	}
	fmt.Println("canonical emission:")
	fmt.Println("  " + emitted)
	return nil
}

// printSelect prints the projection list. Items is authoritative whenever it
// is populated: a `*` mixed with columns appears in it as a parse.StarExpr,
// which keeps the order. Only the bare `SELECT *` leaves Items empty and sets
// the Star flag alone.
func printSelect(clause parse.SelectClause) {
	head := "  SELECT"
	if clause.Distinct {
		head += " DISTINCT"
	}
	// The deprecated TOP row limit sits between DISTINCT and the list. A
	// walker that skipped it would treat a bounded query as unbounded.
	if clause.Top != nil {
		head += " " + aql.FormatTop(clause.Top)
	}
	if len(clause.Items) == 0 {
		if clause.Star {
			fmt.Println(head + " *")
		} else {
			fmt.Println(head + " <no projection>")
		}
		return
	}
	fmt.Println(head + ":")
	for i, item := range clause.Items {
		text := describeSelectExpr(item.Expr)
		if item.Alias != "" {
			text += " AS " + item.Alias
		}
		fmt.Printf("    [%d] %s\n", i, text)
	}
}

// describeSelectExpr renders one projected expression. parse.SelectExpr is a
// sealed set that can grow, so the switch reports an unknown shape instead of
// panicking on it.
func describeSelectExpr(expr parse.SelectExpr) string {
	switch v := expr.(type) {
	case parse.PathExpr:
		return v.Raw
	case parse.StarExpr:
		return "*"
	case parse.LiteralExpr:
		// A projected literal uses the same aql.Value vocabulary as the
		// WHERE side. Raw is the source text as written; show it only when
		// it differs from the canonical rendering (`1.50` against `1.5`,
		// `"x"` against `'x'`), since that difference is the reason the
		// field exists. Compare with aql.FormatValue, the canonical text,
		// and not with describeValue, whose type tag would make every
		// literal look different from its source.
		if v.Raw != "" && v.Raw != aql.FormatValue(v.Value) {
			return fmt.Sprintf("%s (source text: %s)", describeValue(v.Value), v.Raw)
		}
		return describeValue(v.Value)
	case parse.FunctionCall:
		return describeFunctionCall(v)
	}
	return fmt.Sprintf("<out-of-catalogue SelectExpr %T>", expr)
}

// describeFunctionCall renders `NAME(args)`, including the COUNT(*) and
// COUNT(DISTINCT x) aggregate forms.
func describeFunctionCall(call parse.FunctionCall) string {
	if call.Star {
		return call.Name + "(*)"
	}
	args := make([]string, 0, len(call.Args))
	for _, arg := range call.Args {
		args = append(args, describeSelectExpr(arg))
	}
	body := strings.Join(args, ", ")
	if call.Distinct {
		body = "DISTINCT " + body
	}
	return call.Name + "(" + body + ")"
}

// printFrom prints the FROM clause. A boolean junction at the FROM root
// (`FROM COMPOSITION c OR EHR e`) has no single root class, so the parser
// fills Junction and leaves Root and Contains zero. Check Junction first, or
// such a query looks like an empty FROM.
func printFrom(clause parse.FromClause) {
	if clause.Junction != nil {
		fmt.Printf("  FROM %s (root junction, %d operands):\n", clause.Junction.ChildJoin, len(clause.Junction.Children))
		for _, operand := range clause.Junction.Children {
			printOperand("    ", operand)
		}
		return
	}
	fmt.Printf("  FROM %s\n", describeClassExpr(clause.Root))
	if clause.Contains != nil {
		printContainment("    ", *clause.Contains)
	}
}

// printOperand prints one operand of a containment junction. Unlike
// printContainment it writes no CONTAINS keyword: a junction operand is a
// sibling of the others, not something the node above it contains.
func printOperand(indent string, node parse.Containment) {
	if node.Class.RMType == "" && len(node.Children) > 0 {
		fmt.Printf("%s%s (%d operands):\n", indent, node.ChildJoin, len(node.Children))
		for _, operand := range node.Children {
			printOperand(indent+"  ", operand)
		}
		return
	}
	fmt.Printf("%s%s\n", indent, describeClassExpr(node.Class))
	for _, child := range node.Children {
		printContainment(indent+"  ", child)
	}
}

// describeClassExpr renders `CLASS alias[predicate]`. The predicate is either
// an archetype id or a standing predicate such as `[ehr_id/value=$x]`.
func describeClassExpr(class parse.ClassExpr) string {
	out := class.RMType
	if class.Alias != "" {
		out += " " + class.Alias
	}
	switch {
	case class.Archetype != "":
		out += "[" + class.Archetype + "]"
	case class.Predicate != "":
		out += "[" + class.Predicate + "]"
	}
	return out
}

// printContainment prints one node of the CONTAINS tree. A node with no class
// is a boolean grouping of its Children; on a class node the Children are the
// CONTAINS chain below it.
func printContainment(indent string, node parse.Containment) {
	prefix := ""
	if node.Negated {
		prefix = "NOT "
	}
	if node.Class.RMType == "" && len(node.Children) > 0 {
		fmt.Printf("%sCONTAINS %s%s (%d operands):\n", indent, prefix, node.ChildJoin, len(node.Children))
	} else {
		fmt.Printf("%sCONTAINS %s%s\n", indent, prefix, describeClassExpr(node.Class))
	}
	for _, child := range node.Children {
		printContainment(indent+"  ", child)
	}
}

func printWhere(expr aql.WhereExpr) {
	if expr == nil {
		return
	}
	fmt.Println("  WHERE:")
	printWhereExpr("    ", expr)
}

// printWhereExpr prints one node of the WHERE tree. aql.WhereExpr is a sealed
// set that can grow, so an unknown shape is reported and never a panic.
func printWhereExpr(indent string, expr aql.WhereExpr) {
	switch v := expr.(type) {
	case aql.Comparison:
		// Path carries a plain left operand. When the left side is a
		// function call, Left carries it instead and Path is empty.
		left := v.Path
		if v.Left != nil {
			left = describeValue(v.Left)
		}
		fmt.Printf("%s%s %s %s\n", indent, left, v.Op, describeValue(v.Val))
	case aql.Junction:
		fmt.Printf("%s%s:\n", indent, v.Op)
		for _, term := range v.Terms {
			printWhereExpr(indent+"  ", term)
		}
	case aql.NotExpr:
		fmt.Printf("%sNOT:\n", indent)
		printWhereExpr(indent+"  ", v.Operand)
	case aql.ExistsExpr:
		fmt.Printf("%sEXISTS %s\n", indent, v.Path)
	case aql.LikeExpr:
		fmt.Printf("%s%s LIKE %s\n", indent, v.Path, describeValue(v.Pattern))
	case aql.MatchesExpr:
		// Exactly one of Values, Terminology and URI carries the operand.
		// The bare TERMINOLOGY(...) and {uri} forms take no braces around a
		// value list.
		switch {
		case v.Terminology != nil:
			fmt.Printf("%s%s MATCHES %s\n", indent, v.Path, describeValue(*v.Terminology))
		case v.URI != "":
			fmt.Printf("%s%s MATCHES {%s} (uri)\n", indent, v.Path, v.URI)
		default:
			values := make([]string, 0, len(v.Values))
			for _, value := range v.Values {
				values = append(values, describeValue(value))
			}
			fmt.Printf("%s%s MATCHES {%s}\n", indent, v.Path, strings.Join(values, ", "))
		}
	default:
		fmt.Printf("%s<out-of-catalogue WhereExpr %T>\n", indent, expr)
	}
}

// describeValue renders a value in its wire form plus a type tag. The wire
// form comes from aql.FormatValue, so quoting and escaping match what the
// emitter writes; the tag exists only to make the tree readable here.
func describeValue(value aql.Value) string {
	if value == nil {
		return "<nil>"
	}
	wire := aql.FormatValue(value)
	switch value.(type) {
	case aql.ParamValue:
		return wire + " (param)"
	case aql.StringValue:
		return wire + " (string)"
	case aql.IntValue:
		return wire + " (int)"
	case aql.RealValue:
		return wire + " (real)"
	case aql.BoolValue:
		return wire + " (bool)"
	case aql.NullValue:
		return wire + " (null)"
	case aql.PathValue:
		// A path used as a value: the right side of a path-to-path
		// comparison, or a function argument.
		return wire + " (path)"
	case aql.FuncCall:
		// A function call used as a value. Its arguments are Values too, so
		// nesting is uniform.
		return wire + " (func)"
	}
	return wire
}

func printOrderBy(terms []parse.OrderTerm) {
	if len(terms) == 0 {
		return
	}
	fmt.Println("  ORDER BY:")
	for i, term := range terms {
		fmt.Printf("    [%d] %s %s\n", i, term.Path.Raw, term.Dir)
	}
}

func printPaging(limit, offset parse.LimitExpr) {
	if limit != nil {
		fmt.Printf("  LIMIT %s\n", describeLimit(limit))
	}
	if offset != nil {
		fmt.Printf("  OFFSET %s\n", describeLimit(offset))
	}
}

// describeLimit renders a LIMIT or OFFSET value, which is either an integer
// literal or a $parameter bound at execution time.
func describeLimit(limit parse.LimitExpr) string {
	switch v := limit.(type) {
	case parse.IntLimit:
		return fmt.Sprintf("%d (int)", v.N)
	case parse.ParamLimit:
		return "$" + v.Name + " (param)"
	}
	return fmt.Sprintf("%T", limit)
}
