package simplified_test

// REQ-053 — canonical Simplified Formats media types. The `.schema`-suffixed
// EHRbase variants are accepted on input only and MUST NOT be emitted.
import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

func TestMediaTypeConstants(t *testing.T) {
	if simplified.MediaTypeFlat != "application/openehr.wt.flat+json" {
		t.Errorf("MediaTypeFlat = %q", simplified.MediaTypeFlat)
	}
	if simplified.MediaTypeStructured != "application/openehr.wt.structured+json" {
		t.Errorf("MediaTypeStructured = %q", simplified.MediaTypeStructured)
	}
}
