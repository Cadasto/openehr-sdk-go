package validation_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-102 § Building-block independence — openehr/validation MUST
// NOT depend on openehr/serialize. The validator operates on
// in-memory RM graphs; wire decoding belongs to callers. Pulling
// serialize into the validator's transitive deps would invert the
// dependency direction (validators feed into codecs, not the
// reverse) and bloat any consumer that just wants in-memory checks.
//
// The check enumerates non-test files only — the test files are
// allowed to use serialize for fixture decode.
func TestValidationNoSerializeImport(t *testing.T) {
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "openehr/serialize") {
			t.Errorf("openehr/validation MUST NOT import %q (REQ-013 building-block independence; serialize handles wire bytes, validation handles in-memory RM)", imp)
		}
	}
}
