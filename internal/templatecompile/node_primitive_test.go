package templatecompile_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// REQ-102 / REQ-103: STRING is an AOM 1.4 primitive short name, the
// same closed set the validator and the synthesiser share. A BMM
// class name is not a primitive short name.
func TestREQ102_REQ103_IsAOMPrimitiveShortNameSTRING(t *testing.T) {
	t.Parallel()
	if !templatecompile.IsAOMPrimitiveShortName("STRING") {
		t.Error(`IsAOMPrimitiveShortName("STRING") = false, want true`)
	}
	if templatecompile.IsAOMPrimitiveShortName("DV_TEXT") {
		t.Error(`IsAOMPrimitiveShortName("DV_TEXT") = true, want false`)
	}
}
