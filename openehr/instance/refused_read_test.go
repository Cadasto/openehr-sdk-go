package instance_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// requireReadRefused fails t unless reading the operational template text
// xml is refused with an error wrapping canxml.ErrInvalidShape because a
// number is out of range, not because it is malformed. The model types an
// Integer of a template as the 32 bits of an RM Integer, so a template
// with one past that range never reaches the compile step. xml writes such
// a number in plain decimal digits: a float64 formatted by a helper reads
// as 5e+09, which is refused for its syntax.
func requireReadRefused(t *testing.T, xml string) {
	t.Helper()
	opt, err := aom14.ReadOperationalTemplate(strings.NewReader(xml))
	if !errors.Is(err, canxml.ErrInvalidShape) {
		t.Fatalf("ReadOperationalTemplate = %T, error %v, want an error wrapping canxml.ErrInvalidShape", opt, err)
	}
	if !errors.Is(err, strconv.ErrRange) {
		t.Errorf("ReadOperationalTemplate error = %v, want it to say a number is out of range", err)
	}
}
