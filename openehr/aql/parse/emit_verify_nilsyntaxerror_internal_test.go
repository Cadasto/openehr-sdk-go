package parse

// emit_verify_nilsyntaxerror_internal_test.go — REQ-025 § No panics, on the
// consumer half of the nil-receiver axis at verifyEmitted's errors.AsType[*
// SyntaxError] extraction site (emit_verify.go). errors.As / errors.AsType
// report ok=true for a boxed typed nil (the value a caller's own failed match
// leaves behind and passes onward), so `ok` alone is not proof there is a
// *SyntaxError to read — exactly the shape
// openehr/client/system/system.go:230 guards against for
// *transport.WireError. Internal because syntaxErrorPosition is unexported;
// ParseQuery itself never returns a typed-nil *SyntaxError (its one
// construction site, parse.go's errorCollector, always populates Pos), so
// the guard is exercised directly rather than through Query.Emit.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSyntaxErrorPositionToleratesABoxedNilSyntaxError(t *testing.T) {
	var se *SyntaxError
	boxed := fmt.Errorf("aql: %w", se)

	got, ok := errors.AsType[*SyntaxError](boxed)
	if !ok {
		t.Fatal("premise gone: errors.AsType did not match a boxed typed-nil *SyntaxError")
	}
	if got != nil {
		t.Fatalf("premise gone: errors.AsType returned a non-nil *SyntaxError (%v) for a boxed typed nil", got)
	}

	if _, ok := syntaxErrorPosition(boxed); ok { // panics here today = the finding
		t.Error("syntaxErrorPosition(boxed typed-nil SyntaxError) ok = true, want false — there is no position to read")
	}

	// A positioned SyntaxError still yields its position, so the guard did not
	// swallow the signal.
	positioned := &SyntaxError{Pos: Position{Line: 4, Col: 9}, Msg: "unexpected token"}
	pos, ok := syntaxErrorPosition(positioned)
	if !ok {
		t.Fatal("syntaxErrorPosition(positioned SyntaxError) ok = false, want true")
	}
	if want := (Position{Line: 4, Col: 9}); pos != want {
		t.Errorf("syntaxErrorPosition(positioned SyntaxError) = %+v, want %+v", pos, want)
	}
}

// TestSyntaxErrorPositionTreatsAZeroPositionAsAbsent is the position-honesty
// half of the same helper. A non-nil *SyntaxError whose Pos is the zero value
// carries NO position — [SyntaxError.Error] and lint's syntaxDetail both omit a
// fabricated "0:0" rather than print one — so reporting ok=true here would let
// verifyEmitted format "syntax error at 0:0" and invent a coordinate the
// diagnostic never had.
func TestSyntaxErrorPositionTreatsAZeroPositionAsAbsent(t *testing.T) {
	unpositioned := &SyntaxError{Msg: "unexpected token"}

	if _, ok := syntaxErrorPosition(unpositioned); ok {
		t.Error("syntaxErrorPosition(zero-Pos SyntaxError) ok = true, want false — the zero Position is the \"no position\" value")
	}

	// The honesty rule it mirrors, pinned here so the two cannot drift apart.
	if got := unpositioned.Error(); strings.Contains(got, "0:0") {
		t.Errorf("SyntaxError.Error() = %q, want no fabricated 0:0 coordinate", got)
	}
}
