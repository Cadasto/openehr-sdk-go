package aql_test

// paging_test.go pins the REQ-117 in-text paging channel: `LIMIT n [OFFSET m]`
// emitted into the AQL string after ORDER BY (the grammar's
// `LIMIT limitValue (OFFSET limitValue)?`), opt-in and mutually exclusive with
// the envelope channel (Query.Fetch / Query.Offset).
// PROBE-088

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// TestInlinePagingEmission tables the canonical in-text paging forms and
// their clause position — after ORDER BY, LIMIT before OFFSET.
// REQ-117
func TestInlinePagingEmission(t *testing.T) {
	tests := []struct {
		name string
		page func(*aql.Builder) *aql.Builder
		want string
	}{
		{
			name: "limit literal",
			page: func(b *aql.Builder) *aql.Builder { return b.LimitInline(50) },
			want: "LIMIT 50",
		},
		{
			name: "limit and offset literal",
			page: func(b *aql.Builder) *aql.Builder { return b.LimitInline(50).OffsetInline(100) },
			want: "LIMIT 50 OFFSET 100",
		},
		{
			name: "limit param",
			page: func(b *aql.Builder) *aql.Builder { return b.LimitInlineParam("rows") },
			want: "LIMIT $rows",
		},
		{
			name: "limit and offset param",
			page: func(b *aql.Builder) *aql.Builder {
				return b.LimitInlineParam("$rows").OffsetInlineParam("skip")
			},
			want: "LIMIT $rows OFFSET $skip",
		},
		{
			name: "mixed literal limit and param offset",
			page: func(b *aql.Builder) *aql.Builder { return b.LimitInline(25).OffsetInlineParam("skip") },
			want: "LIMIT 25 OFFSET $skip",
		},
		{
			name: "zero limit is a bound, not an omission",
			page: func(b *aql.Builder) *aql.Builder { return b.LimitInline(0) },
			want: "LIMIT 0",
		},
		{
			name: "later call replaces earlier",
			page: func(b *aql.Builder) *aql.Builder { return b.LimitInline(10).LimitInlineParam("rows") },
			want: "LIMIT $rows",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := aql.NewBuilder().Select(aql.Col("e/ehr_id/value")).From("EHR", "e").
				OrderBy("e/time_created/value", aql.Descending)
			q, err := tc.page(b).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			want := "SELECT e/ehr_id/value FROM EHR e ORDER BY e/time_created/value DESC " + tc.want
			if q.String() != want {
				t.Fatalf("in-text paging mismatch:\n got: %q\nwant: %q", q.String(), want)
			}
			// The in-text channel leaves the envelope untouched: the bound
			// travels in the string, so the executor must not also page.
			if q.Offset != 0 || q.Fetch != 0 {
				t.Errorf("envelope paging populated: Offset=%d Fetch=%d, want 0/0", q.Offset, q.Fetch)
			}
		})
	}
}

// TestEnvelopePagingStillOmitsClause locks the pre-REQ-117 default: the
// envelope channel emits no LIMIT / OFFSET into the string.
// REQ-117
func TestEnvelopePagingStillOmitsClause(t *testing.T) {
	q, err := aql.NewBuilder().Select(aql.Col("e/ehr_id/value")).From("EHR", "e").
		Limit(20).Offset(10).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(q.String(), "LIMIT") || strings.Contains(q.String(), "OFFSET") {
		t.Fatalf("envelope paging leaked into the string: %q", q.String())
	}
	if q.Fetch != 20 || q.Offset != 10 {
		t.Fatalf("envelope paging lost: Fetch=%d Offset=%d, want 20/10", q.Fetch, q.Offset)
	}
}

// TestInlinePagingRefusals locks the fail-loud rules: the two paging channels
// are never silently combined, an in-text OFFSET without a LIMIT is
// grammar-invalid, and an operand the grammar's `limitValue` cannot carry is
// refused at build time.
// REQ-117
func TestInlinePagingRefusals(t *testing.T) {
	base := func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("e/ehr_id/value")).From("EHR", "e")
	}
	tests := map[string]*aql.Builder{
		"in-text limit with envelope limit":     base().LimitInline(50).Limit(20),
		"in-text limit with envelope offset":    base().LimitInline(50).Offset(10),
		"in-text offset with envelope limit":    base().LimitInline(50).OffsetInline(100).Limit(20),
		"envelope limit set first":              base().Limit(20).LimitInline(50),
		"in-text param limit with envelope":     base().LimitInlineParam("rows").Limit(20),
		"in-text offset without limit":          base().OffsetInline(100),
		"in-text param offset without limit":    base().OffsetInlineParam("skip"),
		"negative in-text limit":                base().LimitInline(-1),
		"negative in-text offset":               base().LimitInline(10).OffsetInline(-5),
		"empty in-text limit param name":        base().LimitInlineParam(""),
		"empty in-text offset param name":       base().LimitInline(10).OffsetInlineParam("  "),
		"empty in-text offset param dollaronly": base().LimitInline(10).OffsetInlineParam("$"),
	}
	for name, b := range tests {
		t.Run(name, func(t *testing.T) {
			q, err := b.Build()
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v (query %q), want ErrInvalidQuery", err, q.String())
			}
		})
	}
}
