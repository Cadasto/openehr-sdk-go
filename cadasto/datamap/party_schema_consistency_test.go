package datamap

import (
	"strings"
	"testing"
)

// TestSchemaAcceptsFromPartyOutput is the consistency oracle: a PARTY decoded
// by FromParty MUST validate against the Schema for the same template. Any
// failure is a divergence between the decoder and the schema generator.
func TestSchemaAcceptsFromPartyOutput(t *testing.T) {
	opt := loadTestkitOPT(t, "TestPerson.v2")
	raw := loadTestkitPartyJSON(t, "TestPerson.v2")

	dm, err := FromParty(opt, raw)
	if err != nil {
		t.Fatalf("FromParty: %v", err)
	}
	ok, errs := Validate(opt, dm)
	if !ok {
		t.Fatalf("decoded PARTY does not validate against its own schema:\n  - %s", strings.Join(errs, "\n  - "))
	}
}
