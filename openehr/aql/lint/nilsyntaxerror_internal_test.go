package lint

// nilsyntaxerror_internal_test.go — REQ-025 § No panics, on the consumer half
// of the nil-receiver axis at this package's two errors.AsType[*parse.
// SyntaxError] extraction sites. errors.As / errors.AsType report ok=true for
// a boxed typed nil (the value a caller's own failed match leaves behind and
// passes onward), so `ok` alone is not proof there is a *SyntaxError to read
// — exactly the shape openehr/client/system/system.go:230 guards against for
// *transport.WireError. This test is internal because syntaxSpan and
// syntaxDetail are unexported.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// boxedNilSyntaxError is what a caller hands back after its own errors.AsType
// against *parse.SyntaxError failed and it wraps the (nil) result onward.
func boxedNilSyntaxError() error {
	var se *parse.SyntaxError
	return fmt.Errorf("aql_syntax: %w", se)
}

// TestSyntaxSpanToleratesABoxedNilSyntaxError pins the guard at syntaxSpan.
// Deleting `se != nil` there dereferences se.Pos on a nil receiver and panics.
func TestSyntaxSpanToleratesABoxedNilSyntaxError(t *testing.T) {
	boxed := boxedNilSyntaxError()

	se, ok := errors.AsType[*parse.SyntaxError](boxed)
	if !ok {
		t.Fatal("premise gone: errors.AsType did not match a boxed typed-nil *parse.SyntaxError")
	}
	if se != nil {
		t.Fatalf("premise gone: errors.AsType returned a non-nil *SyntaxError (%v) for a boxed typed nil", se)
	}

	if got := syntaxSpan(boxed); !got.IsZero() { // panics here today = the finding
		t.Errorf("syntaxSpan(boxed typed-nil SyntaxError) = %+v, want the zero Span", got)
	}

	// A positioned SyntaxError still carries its position, so the guard did not
	// swallow the signal.
	positioned := &parse.SyntaxError{Pos: parse.Position{Line: 2, Col: 5}, Msg: "unexpected token"}
	want := Span{Start: parse.Position{Line: 2, Col: 5}, End: parse.Position{Line: 2, Col: 5}}
	if got := syntaxSpan(positioned); got != want {
		t.Errorf("syntaxSpan(positioned SyntaxError) = %+v, want %+v", got, want)
	}
}

// TestSyntaxDetailToleratesABoxedNilSyntaxError pins the guard at
// syntaxDetail. Deleting `se != nil` there dereferences se.Pos on a nil
// receiver and panics.
func TestSyntaxDetailToleratesABoxedNilSyntaxError(t *testing.T) {
	boxed := boxedNilSyntaxError()

	if got := syntaxDetail(boxed); got == "" { // panics here today = the finding
		t.Error(`syntaxDetail(boxed typed-nil SyntaxError) = "", want a non-empty fallback`)
	} else if got != boxed.Error() {
		t.Errorf("syntaxDetail(boxed typed-nil SyntaxError) = %q, want the wrapping error's own text %q (no position to read)", got, boxed.Error())
	}

	// A positioned SyntaxError still formats its position, so the guard did not
	// swallow the signal.
	positioned := &parse.SyntaxError{Pos: parse.Position{Line: 2, Col: 5}, Msg: "unexpected token"}
	if got, want := syntaxDetail(positioned), "2:5: unexpected token"; got != want {
		t.Errorf("syntaxDetail(positioned SyntaxError) = %q, want %q", got, want)
	}
}

// TestSyntaxDetailOmitsPositionWhenZero pins the same no-fabricated-"0:0:"
// rule [parse.SyntaxError.Error] applies: a non-nil *SyntaxError whose Pos is
// the zero value has no real position to report, so syntaxDetail must not
// invent one (REQ-025 nil-receiver axis).
func TestSyntaxDetailOmitsPositionWhenZero(t *testing.T) {
	zeroPos := &parse.SyntaxError{Msg: "unexpected token"}
	if got, want := syntaxDetail(zeroPos), "unexpected token"; got != want {
		t.Errorf("syntaxDetail(zero-Pos SyntaxError) = %q, want %q (no dangling %q)", got, want, "0:0:")
	}
}
