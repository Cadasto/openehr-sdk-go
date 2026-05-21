package itemtags_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/itemtags"
)

func TestFormatParseRoundTrip(t *testing.T) {
	const sample = `key="category",value="final"; key="flag",value="follow-up",target_path="/composition/start_time/value"`
	got, err := itemtags.ParseHeader(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	encoded, err := itemtags.FormatHeader(got)
	if err != nil {
		t.Fatal(err)
	}
	again, err := itemtags.ParseHeader(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 2 || again[0].Key != "category" || again[1].TargetPath == "" {
		t.Fatalf("round trip = %#v", again)
	}
}
