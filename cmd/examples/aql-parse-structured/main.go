// Example: parse an AQL query into the SDK-GAP-17 Tier-2 structured
// AST (parse.Query, REQ-113) and walk its shape.
//
// Demonstrates the read-side mirror of aql.Builder: SELECT items / FROM
// containment tree / WHERE expression tree / ORDER BY / LIMIT / OFFSET,
// all readable WITHOUT importing openehr/aql/parse/gen or any internal/
// package. The unified WhereExpr / Value vocabulary (aql.Comparison /
// aql.Junction / aql.NotExpr / aql.ExistsExpr / aql.LikeExpr /
// aql.MatchesExpr / aql.ParamValue / aql.StringValue / aql.IntValue /
// aql.RealValue / aql.BoolValue) is the same one Builder constructs.
//
// Run:
//
//	go run ./cmd/examples/aql-parse-structured
//	go run ./cmd/examples/aql-parse-structured "SELECT c FROM EHR e CONTAINS COMPOSITION c WHERE c/uid/value = \$id"
//
// With no argument it uses a representative built-in query exercising
// SELECT projection, CONTAINS chain, WHERE comparison, ORDER BY DESC,
// and LIMIT/OFFSET.
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

func main() {
	q := defaultQuery
	if args := os.Args[1:]; len(args) > 0 {
		q = strings.Join(args, " ")
	}

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
	switch {
	case s.Star:
		fmt.Println("  SELECT *")
	case s.Distinct:
		fmt.Println("  SELECT DISTINCT:")
	default:
		fmt.Println("  SELECT:")
	}
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
	case parse.FunctionCall:
		args := make([]string, 0, len(v.Args))
		for _, a := range v.Args {
			args = append(args, describeSelectExpr(a))
		}
		return v.Name + "(" + strings.Join(args, ", ") + ")"
	}
	return fmt.Sprintf("%T", e)
}

func printFrom(f parse.FromClause) {
	fmt.Printf("  FROM %s\n", describeClassExpr(f.Root))
	if f.Contains != nil {
		printContainment("    ", *f.Contains)
	}
}

func describeClassExpr(c parse.ClassExpr) string {
	out := c.RMType
	if c.Alias != "" {
		out += " " + c.Alias
	}
	if c.Archetype != "" {
		out += "[" + c.Archetype + "]"
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
		fmt.Printf("%s%s %s %s\n", indent, v.Path, v.Op, describeValue(v.Val))
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
		vals := make([]string, 0, len(v.Values))
		for _, val := range v.Values {
			vals = append(vals, describeValue(val))
		}
		fmt.Printf("%s%s MATCHES {%s}\n", indent, v.Path, strings.Join(vals, ", "))
	default:
		fmt.Printf("%s<unknown WhereExpr %T>\n", indent, w)
	}
}

func describeValue(v aql.Value) string {
	if v == nil {
		return "<nil>"
	}
	switch x := v.(type) {
	case aql.ParamValue:
		return "$" + x.Name + " (param)"
	case aql.StringValue:
		return fmt.Sprintf("'%s' (string)", x.S)
	case aql.IntValue:
		return fmt.Sprintf("%d (int)", x.N)
	case aql.RealValue:
		return fmt.Sprintf("%v (real)", x.F)
	case aql.BoolValue:
		return fmt.Sprintf("%v (bool)", x.B)
	}
	return aql.FormatValue(v)
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

func printPaging(limit, offset *int) {
	if limit != nil {
		fmt.Printf("  LIMIT %d\n", *limit)
	}
	if offset != nil {
		fmt.Printf("  OFFSET %d\n", *offset)
	}
}
