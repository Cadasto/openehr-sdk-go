package refused_test

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
)

// REQ-052 (wire.md § Value-free decode errors): the literal a primitive codec
// refused reaches DecodeError.Value through this carrier, and the carrier
// changes neither the refusal's message nor its chain.
//
// TestLiteral_REQ052_MessageAndChainUnchanged pins what the literal carrier
// adds to a codec's refusal: the JSON text it refused, as written, and
// nothing a caller reads. Its message is the refusal's, one errors.Unwrap step
// reaches the refusal, and a nil refusal stays nil.
//
// Can-fail control: drop the text, or let the message or the chain differ
// from the refusal's, and the matching assertion fails.
func TestLiteral_REQ052_MessageAndChainUnchanged(t *testing.T) {
	cause := fmt.Errorf("rm.Real: parse quoted literal: %w", &strconv.NumError{Func: "ParseFloat", Num: "x", Err: strconv.ErrSyntax})
	lit := []byte(`"x"`)
	err := refused.Literal(lit, cause)
	if err.Error() != cause.Error() {
		t.Errorf("Literal(...).Error() = %q; want the refusal's own %q", err, cause)
	}
	if got := errors.Unwrap(err); got != cause { //nolint:errorlint // identity: one step reaches the refusal
		t.Errorf("errors.Unwrap(Literal(...)) = %v; want the refusal", got)
	}
	if !errors.Is(err, strconv.ErrSyntax) {
		t.Errorf("Literal(...) = %v; want errors.Is(_, strconv.ErrSyntax), the refusal's own cause", err)
	}
	e, ok := errors.AsType[*refused.Error](err)
	if !ok || e == nil {
		t.Fatalf("errors.AsType[*refused.Error](Literal(...)) found nothing in %T", err)
	}
	lit[1] = 'y' // the codec's buffer is reused after the method returns
	if got, want := e.Text(), `"x"`; got != want {
		t.Errorf("Text() = %q; want %q, the literal as written when it was refused", got, want)
	}
	if got := refused.Literal(lit, nil); got != nil {
		t.Errorf("Literal(lit, nil) = %v; want nil", got)
	}
}

// TestError_NilReceiver pins the nil-receiver guards: a typed nil left behind
// by a failed match answers with a fixed text, an empty literal and no cause
// rather than panicking.
//
// Can-fail control: remove any one guard and that call panics.
func TestError_NilReceiver(t *testing.T) {
	var e *refused.Error
	if got := e.Error(); got != "unspecified error" {
		t.Errorf("(*Error)(nil).Error() = %q; want %q", got, "unspecified error")
	}
	if got := e.Text(); got != "" {
		t.Errorf("(*Error)(nil).Text() = %q; want \"\"", got)
	}
	if got := e.Unwrap(); got != nil {
		t.Errorf("(*Error)(nil).Unwrap() = %v; want nil", got)
	}
}
