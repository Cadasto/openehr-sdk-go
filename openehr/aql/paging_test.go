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
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
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

// TestSelectTopEmission tables the canonical `SELECT TOP` forms and their
// clause position — between DISTINCT and the projection list, keywords
// upper-cased, direction present only when set (REQ-118).
//
// The construct is deprecated by openEHR QUERY Release-1.1.0 §4.4.3; the
// builder can still write it because the SDK must be able to author the
// deprecated shape for a corpus, a migration, or a round-trip fixture.
// REQ-118 · PROBE-088
func TestSelectTopEmission(t *testing.T) {
	tests := []struct {
		name string
		top  func(*aql.Builder) *aql.Builder
		want string
	}{
		{
			name: "plain",
			top:  func(b *aql.Builder) *aql.Builder { return b.Top(5) },
			want: "SELECT TOP 5 e/ehr_id/value FROM EHR e",
		},
		{
			name: "forward",
			top:  func(b *aql.Builder) *aql.Builder { return b.TopDirected(5, aql.TopForward) },
			want: "SELECT TOP 5 FORWARD e/ehr_id/value FROM EHR e",
		},
		{
			name: "backward",
			top:  func(b *aql.Builder) *aql.Builder { return b.TopDirected(5, aql.TopBackward) },
			want: "SELECT TOP 5 BACKWARD e/ehr_id/value FROM EHR e",
		},
		{
			name: "directed with unspecified direction equals plain",
			top:  func(b *aql.Builder) *aql.Builder { return b.TopDirected(5, aql.TopDirUnspecified) },
			want: "SELECT TOP 5 e/ehr_id/value FROM EHR e",
		},
		{
			name: "zero is a bound, not an omission",
			top:  func(b *aql.Builder) *aql.Builder { return b.Top(0) },
			want: "SELECT TOP 0 e/ehr_id/value FROM EHR e",
		},
		{
			name: "later call replaces earlier",
			top:  func(b *aql.Builder) *aql.Builder { return b.Top(10).TopDirected(3, aql.TopBackward) },
			want: "SELECT TOP 3 BACKWARD e/ehr_id/value FROM EHR e",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := aql.NewBuilder().Select(aql.Col("e/ehr_id/value")).From("EHR", "e")
			q, err := tc.top(b).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if q.String() != tc.want {
				t.Fatalf("TOP emission mismatch:\n got: %q\nwant: %q", q.String(), tc.want)
			}
			// A TOP travels in the string, so the envelope must stay clear —
			// the same rule the in-text LIMIT channel follows.
			if q.Offset != 0 || q.Fetch != 0 {
				t.Errorf("envelope paging populated: Offset=%d Fetch=%d, want 0/0", q.Offset, q.Fetch)
			}
		})
	}
}

// TestSelectTopRefusals locks the fail-loud rules for the deprecated clause:
// openEHR QUERY Release-1.1.0 §4.4.3 forbids `TOP` together with `LIMIT`, and
// a TOP is itself an in-text row bound — so it is exclusive with BOTH row-limit
// channels, exactly as the two existing channels are with each other. A
// negative count is unrepresentable in the `top` production.
// REQ-118 · PROBE-088
func TestSelectTopRefusals(t *testing.T) {
	base := func() *aql.Builder {
		return aql.NewBuilder().Select(aql.Col("e/ehr_id/value")).From("EHR", "e")
	}
	tests := map[string]*aql.Builder{
		"top with in-text limit":             base().Top(5).LimitInline(10),
		"in-text limit set first":            base().LimitInline(10).Top(5),
		"top with in-text param limit":       base().Top(5).LimitInlineParam("rows"),
		"top with in-text offset":            base().Top(5).LimitInline(10).OffsetInline(2),
		"top with envelope limit":            base().Top(5).Limit(20),
		"top with envelope offset":           base().Top(5).Offset(10),
		"directed top with envelope limit":   base().TopDirected(5, aql.TopBackward).Limit(20),
		"negative top count":                 base().Top(-1),
		"negative directed top count":        base().TopDirected(-1, aql.TopForward),
		"unknown direction is not emittable": base().TopDirected(5, aql.TopDir(99)),
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

// TestSelectTopRoundTripsThroughParse ties the write side to the read side: a
// builder-emitted TOP must parse back into the shared carrier with the same
// count and direction, so neither half can drift alone.
// REQ-118 · PROBE-087 · PROBE-088
func TestSelectTopRoundTripsThroughParse(t *testing.T) {
	for _, dir := range []aql.TopDir{aql.TopDirUnspecified, aql.TopForward, aql.TopBackward} {
		t.Run(dir.String(), func(t *testing.T) {
			q, err := aql.NewBuilder().Select(aql.Col("e/ehr_id/value")).From("EHR", "e").
				TopDirected(7, dir).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			parsed, err := parse.ParseQuery(q.String())
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", q.String(), err)
			}
			if parsed.Select.Top == nil {
				t.Fatal("parsed Select.Top = nil, want the built clause")
			}
			if got := *parsed.Select.Top; got != (aql.TopClause{N: 7, Dir: dir}) {
				t.Errorf("parsed Top = %+v, want {N:7 Dir:%v}", got, dir)
			}
		})
	}
}
