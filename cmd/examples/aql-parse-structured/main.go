// Example: parse an AQL query into the REQ-113 Tier-2 structured
// AST (parse.Query) and walk its shape.
//
// Demonstrates the read-side mirror of aql.Builder: SELECT items / FROM
// containment tree / WHERE expression tree / ORDER BY / LIMIT / OFFSET,
// all readable WITHOUT importing openehr/aql/parse/gen or any internal/
// package. The unified WhereExpr / Value vocabulary (aql.Comparison /
// aql.Junction / aql.NotExpr / aql.ExistsExpr / aql.LikeExpr /
// aql.MatchesExpr / aql.ParamValue / aql.StringValue / aql.IntValue /
// aql.RealValue / aql.BoolValue / aql.PathValue / aql.FuncCall) is the
// same one Builder constructs.
//
// Run:
//
//	go run ./cmd/examples/aql-parse-structured
//	go run ./cmd/examples/aql-parse-structured "SELECT c FROM EHR e CONTAINS COMPOSITION c WHERE c/uid/value = \$id"
//
// With no argument it walks two built-in queries: a representative one
// exercising SELECT projection, CONTAINS chain, WHERE comparison, ORDER BY
// DESC and LIMIT/OFFSET, then a REQ-117 query exercising the catalogue
// closures the v1 extractor refused — a mixed `SELECT *, col` list with a
// primitive literal, a function-call WHERE left operand, a path-vs-path
// comparison, and a containment junction at the FROM root.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

const defaultQuery = `SELECT
  c/uid/value,
  c/name/value
FROM EHR e
  CONTAINS COMPOSITION c
WHERE c/uid/value = $cid AND c/name/value LIKE 'Vital%'
ORDER BY c/uid/value DESC
LIMIT 50 OFFSET 100`

// req117Query exercises the catalogue closures REQ-117 landed — each of these
// shapes used to make ParseQuery return aql.ErrIncompleteAST, so a consumer
// had to refuse the whole statement:
//
//   - `*, 1 AS rank` — a star mixed with column projections, plus a primitive
//     literal item (parse.StarExpr / parse.LiteralExpr);
//   - `LENGTH(c/name/value)` as a projection AND as a comparison left operand;
//   - `c/uid/value = c/name/value` — a path as the comparison right operand;
//   - `FROM COMPOSITION c OR EHR e` — a containment junction at the FROM root.
const req117Query = `SELECT *, 1 AS rank, LENGTH(c/name/value)
FROM COMPOSITION c OR EHR e
WHERE LENGTH(c/name/value) > $min AND c/uid/value = c/name/value`

func main() {
	if args := os.Args[1:]; len(args) > 0 {
		walk(strings.Join(args, " "))
		return
	}
	walk(defaultQuery)
	fmt.Println()
	fmt.Println("--- REQ-117 catalogue closures ---")
	fmt.Println()
	walk(req117Query)
}

// walk parses one query and prints its structured AST plus the canonical
// re-emission.
func walk(q string) {
	fmt.Println("input AQL:")
	for line := range strings.SplitSeq(q, "\n") {
		fmt.Println("  " + line)
	}
	fmt.Println()

	parsed, err := parse.ParseQuery(q)
	if err != nil {
		log.Fatalf("ParseQuery: %v", err)
	}

	fmt.Println("structured AST:")
	printSelect(parsed.Select)
	printFrom(parsed.From)
	printWhere(parsed.Where)
	printOrderBy(parsed.OrderBy)
	printPaging(parsed.Limit, parsed.Offset)

	fmt.Println()
	emitted, err := parsed.Emit()
	if err != nil {
		log.Fatalf("Emit: %v", err)
	}
	fmt.Println("canonical emission:")
	fmt.Println("  " + emitted)
}

func printSelect(s parse.SelectClause) {
	head := "  SELECT"
	if s.Distinct {
		head += " DISTINCT"
	}
	// REQ-118: the deprecated `TOP n [FORWARD|BACKWARD]` row limit sits between
	// DISTINCT and the projection list. A consumer that ignored it would run
	// unbounded where the source asked for n rows.
	if s.Top != nil {
		head += " " + aql.FormatTop(s.Top)
	}
	// Items lead: since REQ-117 a mixed `SELECT *, col` carries the star as a
	// parse.StarExpr item, so the list is authoritative whenever it is
	// populated. The bare `SELECT *` form leaves Items empty and is carried by
	// the Star flag alone.
	if len(s.Items) == 0 {
		if s.Star {
			fmt.Println(head + " *")
		} else {
			fmt.Println(head + " <no projection>")
		}
		return
	}
	fmt.Println(head + ":")
	for i, item := range s.Items {
		desc := describeSelectExpr(item.Expr)
		if item.Alias != "" {
			desc += " AS " + item.Alias
		}
		fmt.Printf("    [%d] %s\n", i, desc)
	}
}

func describeSelectExpr(e parse.SelectExpr) string {
	switch v := e.(type) {
	case parse.PathExpr:
		return v.Raw
	case parse.StarExpr:
		// REQ-117: an explicit star item inside a mixed projection list.
		return "*"
	case parse.LiteralExpr:
		// REQ-117: a primitive or parameter literal projection — same
		// aql.Value vocabulary the WHERE side uses.
		return describeValue(v.Value)
	case parse.FunctionCall:
		var body string
		switch {
		case v.Star:
			body = "*"
		case v.Distinct:
			args := make([]string, 0, len(v.Args))
			for _, a := range v.Args {
				args = append(args, describeSelectExpr(a))
			}
			body = "DISTINCT " + strings.Join(args, ", ")
		default:
			args := make([]string, 0, len(v.Args))
			for _, a := range v.Args {
				args = append(args, describeSelectExpr(a))
			}
			body = strings.Join(args, ", ")
		}
		return v.Name + "(" + body + ")"
	}
	// The SelectExpr set grows additively, so an unrecognised case is
	// out-of-catalogue for this consumer — report it, never panic.
	return fmt.Sprintf("<out-of-catalogue SelectExpr %T>", e)
}

func printFrom(f parse.FromClause) {
	// REQ-117: a boolean junction AT the FROM root has no single root class,
	// so Root and Contains are left zero — a consumer reading only Root would
	// see an empty FROM. Check Junction first.
	if f.Junction != nil {
		fmt.Printf("  FROM %s (root junction, %d operands):\n", f.Junction.ChildJoin, len(f.Junction.Children))
		for _, operand := range f.Junction.Children {
			printOperand("    ", operand)
		}
		return
	}
	fmt.Printf("  FROM %s\n", describeClassExpr(f.Root))
	if f.Contains != nil {
		printContainment("    ", *f.Contains)
	}
}

// printOperand prints one operand of a containment junction. Unlike
// printContainment it writes no CONTAINS keyword — a junction operand is a
// sibling, not something the node above it contains.
func printOperand(indent string, c parse.Containment) {
	if c.Class.RMType == "" && len(c.Children) > 0 {
		fmt.Printf("%s%s (%d operands):\n", indent, c.ChildJoin, len(c.Children))
		for _, operand := range c.Children {
			printOperand(indent+"  ", operand)
		}
		return
	}
	fmt.Printf("%s%s\n", indent, describeClassExpr(c.Class))
	for _, ch := range c.Children {
		printContainment(indent+"  ", ch)
	}
}

func describeClassExpr(c parse.ClassExpr) string {
	out := c.RMType
	if c.Alias != "" {
		out += " " + c.Alias
	}
	switch {
	case c.Archetype != "":
		out += "[" + c.Archetype + "]"
	case c.Predicate != "":
		out += "[" + c.Predicate + "]"
	}
	return out
}

func printContainment(indent string, c parse.Containment) {
	prefix := ""
	if c.Negated {
		prefix = "NOT "
	}
	switch {
	case len(c.Children) > 0 && c.Class.RMType == "":
		fmt.Printf("%sCONTAINS %s(%d operands):\n", indent, prefix+c.ChildJoin.String()+" ", len(c.Children))
		for _, ch := range c.Children {
			printContainment(indent+"  ", ch)
		}
	default:
		fmt.Printf("%sCONTAINS %s%s\n", indent, prefix, describeClassExpr(c.Class))
		for _, ch := range c.Children {
			printContainment(indent+"  ", ch)
		}
	}
}

func printWhere(w aql.WhereExpr) {
	if w == nil {
		return
	}
	fmt.Println("  WHERE:")
	printWhereExpr("    ", w)
}

func printWhereExpr(indent string, w aql.WhereExpr) {
	switch v := w.(type) {
	case aql.Comparison:
		// REQ-117: Left carries the left operand when it is not a plain path
		// (a function call); Path is empty then.
		left := v.Path
		if v.Left != nil {
			left = describeValue(v.Left)
		}
		fmt.Printf("%s%s %s %s\n", indent, left, v.Op, describeValue(v.Val))
	case aql.Junction:
		fmt.Printf("%s%s:\n", indent, v.Op)
		for _, t := range v.Terms {
			printWhereExpr(indent+"  ", t)
		}
	case aql.NotExpr:
		fmt.Printf("%sNOT:\n", indent)
		printWhereExpr(indent+"  ", v.Operand)
	case aql.ExistsExpr:
		fmt.Printf("%sEXISTS %s\n", indent, v.Path)
	case aql.LikeExpr:
		fmt.Printf("%s%s LIKE %s\n", indent, v.Path, describeValue(v.Pattern))
	case aql.MatchesExpr:
		// REQ-117: exactly one of Values / Terminology / URI carries the
		// operand — the bare TERMINOLOGY(...) and {uri} forms take no braces
		// around a value list.
		switch {
		case v.Terminology != nil:
			fmt.Printf("%s%s MATCHES %s\n", indent, v.Path, describeValue(*v.Terminology))
		case v.URI != "":
			fmt.Printf("%s%s MATCHES {%s} (uri)\n", indent, v.Path, v.URI)
		default:
			vals := make([]string, 0, len(v.Values))
			for _, val := range v.Values {
				vals = append(vals, describeValue(val))
			}
			fmt.Printf("%s%s MATCHES {%s}\n", indent, v.Path, strings.Join(vals, ", "))
		}
	default:
		// The WhereExpr set grows additively (REQ-117): report an
		// unrecognised case as out-of-catalogue rather than panicking.
		fmt.Printf("%s<out-of-catalogue WhereExpr %T>\n", indent, w)
	}
}

func describeValue(v aql.Value) string {
	if v == nil {
		return "<nil>"
	}
	// Delegate the wire form to aql.FormatValue so embedded apostrophes
	// are escape-doubled consistently with the emitter; append a
	// type-tag suffix for the demo.
	wire := aql.FormatValue(v)
	switch v.(type) {
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
		// REQ-117: a path in a value position (path-vs-path comparison, or a
		// function argument).
		return wire + " (path)"
	case aql.FuncCall:
		// REQ-117: a function call in a value position — args are themselves
		// Values, so nesting is uniform.
		return wire + " (func)"
	}
	return wire
}

func printOrderBy(terms []parse.OrderTerm) {
	if len(terms) == 0 {
		return
	}
	fmt.Println("  ORDER BY:")
	for i, t := range terms {
		fmt.Printf("    [%d] %s %s\n", i, t.Path.Raw, t.Dir)
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

func describeLimit(l parse.LimitExpr) string {
	switch v := l.(type) {
	case parse.IntLimit:
		return fmt.Sprintf("%d (int)", v.N)
	case parse.ParamLimit:
		return "$" + v.Name + " (param)"
	}
	return fmt.Sprintf("%T", l)
}
