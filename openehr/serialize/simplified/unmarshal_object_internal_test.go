package simplified

import (
	"os"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// objectSamples probe where json/v2 and the v1 decoder part ways: duplicate
// names, invalid UTF-8, numbers past float64's exact range, content after the
// object, and values that are not an object.
var objectSamples = []string{
	`{}`, `null`, `{"a":1}`, `{"a":12345678901234567890123}`, `{"a":-0.0e+00,"b":[1,2.5,{"c":3}]}`,
	`{"a":1,"a":2}`, `{"a":{"x":1},"a":{"y":2}}`, "{\"a\":\"\xff\"}", `{"a":"\ud800"}`,
	`{"a":1} ]`, `{"a":1} }`, `{"a":1}{"b":2}`, `{"a":1} x`, `{"a":1}   `, `[1]`, `"s"`, `1`,
	`{"a":}`, `{"a":1`, ``, ` `, `{"a":true,"b":false,"c":null}`, `{"A":1,"a":2}`,
}

// TestUnmarshalObjectAgreesWithV1 pins that decoding in place with json/v2
// gives what the v1 decoder gives: the same map for every FLAT conformance
// body and probe, and the same error text for each one it refuses.
func TestUnmarshalObjectAgreesWithV1(t *testing.T) {
	inputs := append([]string(nil), objectSamples...)
	names, err := fixtures.ListFlatConformance()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		raw, err := os.ReadFile(fixtures.FlatConformanceFlat(name))
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, string(raw))
	}
	if len(inputs) < len(objectSamples)+30 {
		t.Fatalf("only %d inputs, the FLAT conformance corpus went missing", len(inputs))
	}
	for _, in := range inputs {
		checkUnmarshalObjectAgrees(t, in)
	}
}

func checkUnmarshalObjectAgrees(t *testing.T, in string) {
	t.Helper()
	got, gotErr := unmarshalObject([]byte(in))
	want, wantErr := unmarshalObjectV1([]byte(in))
	if (gotErr == nil) != (wantErr == nil) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
		t.Errorf("unmarshalObject(%.60q) error = %v, the v1 decoder gives %v", in, gotErr, wantErr)
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unmarshalObject(%.60q) = %v, the v1 decoder gives %v", in, got, want)
	}
}

// FuzzUnmarshalObjectAgreesWithV1 holds the same agreement over generated
// documents.
func FuzzUnmarshalObjectAgreesWithV1(f *testing.F) {
	for _, s := range objectSamples {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		checkUnmarshalObjectAgrees(t, in)
	})
}
