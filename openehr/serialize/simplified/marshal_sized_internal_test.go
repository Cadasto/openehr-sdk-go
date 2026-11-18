package simplified

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestMarshalSizedMatchesMarshal holds marshalSized to json.Marshal byte for
// byte, on the shapes the FLAT and canonical maps hold and on strings whose
// escapes outgrow the size estimate.
func TestMarshalSizedMatchesMarshal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		v    any
	}{
		{name: "empty map", v: map[string]any{}},
		{name: "flat scalars", v: map[string]any{
			"t/a|value": "x", "t/b|magnitude": 1.5, "t/c": true, "t/d": nil,
			"t/e|count": int64(7), "t/f|number": json.Number("12345678901234567890"),
		}},
		{name: "nested", v: map[string]any{
			"_type": "COMPOSITION",
			"content": []any{
				map[string]any{"_type": "OBSERVATION", "name": map[string]any{"value": "n"}},
				"s", 2.0, nil, false,
				[]any{},
			},
		}},
		{name: "html and line separators", v: map[string]any{"k<&>": "a<b>&c\u2028\u2029"}},
		{name: "invalid utf-8", v: map[string]any{"k": "a\xffb"}},
		{name: "escapes past the estimate", v: map[string]any{"k": strings.Repeat("<\"\\\n", 512)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want, wantErr := json.Marshal(tc.v)
			got, gotErr := marshalSized(tc.v)
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("marshalSized error = %v, json.Marshal error = %v", gotErr, wantErr)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("marshalSized(%s) =\n%s\nwant json.Marshal's\n%s", tc.name, got, want)
			}
		})
	}
}
