package ehr_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
)

func TestItemTagHeaderRoundTrip(t *testing.T) {
	in := []ehr.ItemTag{
		{Key: "category", Value: "final"},
		{Key: `say "hi"`, Value: `with "quote"`, TargetPath: "/path"},
	}
	encoded, err := ehr.FormatItemTagHeader(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ehr.ParseItemTagHeader(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "category" || got[1].Key != `say "hi"` {
		t.Fatalf("got = %#v", got)
	}
}
