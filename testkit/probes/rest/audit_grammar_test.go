package restprobes

import (
	"maps"
	"testing"
)

// TestParseAuditDetailsHeader exercises the PROBE-062 grammar oracle directly.
// The probe's can-fail plants reach only the shapes an SDK write can be made to
// emit; the parser also has to be right about escapes, duplicates and stray
// whitespace, which no plant can reach — and being right about them is the
// whole reason the probe parses the header instead of comparing it with the
// encoder that produced it (REQ-059).
func TestParseAuditDetailsHeader(t *testing.T) { // PROBE-062, REQ-059
	accepted := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "a single assignment",
			in:   `system_id="cdr.example"`,
			want: map[string]string{"system_id": "cdr.example"},
		},
		{
			name: "the worked example from the ITS-REST contract",
			in:   `committer.name="John Doe",committer.external_ref.id="BC8132EA",committer.external_ref.namespace="demographic",committer.external_ref.type="PERSON"`,
			want: map[string]string{
				"committer.name":                   "John Doe",
				"committer.external_ref.id":        "BC8132EA",
				"committer.external_ref.namespace": "demographic",
				"committer.external_ref.type":      "PERSON",
			},
		},
		{
			name: "escaped quote and backslash are unescaped",
			in:   `description.value="a \"quoted\" path C:\\tmp"`,
			want: map[string]string{"description.value": `a "quoted" path C:\tmp`},
		},
		{
			name: "an empty value",
			in:   `description.value=""`,
			want: map[string]string{"description.value": ""},
		},
		{
			name: "a comma inside a value does not split the assignment",
			in:   `committer.name="Doe, John",system_id="cdr.example"`,
			want: map[string]string{"committer.name": "Doe, John", "system_id": "cdr.example"},
		},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAuditDetailsHeader(tc.in)
			if err != nil {
				t.Fatalf("parseAuditDetailsHeader(%q) = error %v, want it to parse", tc.in, err)
			}
			if !maps.Equal(got, tc.want) {
				t.Errorf("parseAuditDetailsHeader(%q) =\n got: %v\nwant: %v", tc.in, got, tc.want)
			}
		})
	}

	refused := []struct {
		name string
		in   string
	}{
		{"empty input", ""},
		{"a JSON object", `{"system_id":"cdr.example"}`},
		{"semicolon separator", `change_type.code_string="251";system_id="cdr.example"`},
		{"an unquoted value", `system_id=cdr.example`},
		{"a single-quoted value", `system_id='cdr.example'`},
		{"whitespace before the equals sign", `system_id ="cdr.example"`},
		{"whitespace after the equals sign", `system_id= "cdr.example"`},
		{"whitespace after the separator", `change_type.code_string="251", system_id="cdr.example"`},
		{"a duplicate key", `system_id="a",system_id="b"`},
		{"an unterminated value", `system_id="cdr.example`},
		{"a dangling escape", `system_id="cdr.example\`},
		{"an undefined escape", `system_id="cdr\.example"`},
		{"a raw quote inside a value", `committer.name="say "hi""`},
		{"a key starting with a digit", `1system_id="cdr.example"`},
		{"a key with an empty dotted segment", `committer..name="Alice"`},
		{"a trailing dot on the key", `committer.="Alice"`},
		{"a trailing separator", `system_id="cdr.example",`},
		{"a leading separator", `,system_id="cdr.example"`},
		{"an assignment with no value", `system_id=`},
		{"a bare key", `system_id`},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAuditDetailsHeader(tc.in)
			if err == nil {
				t.Fatalf("parseAuditDetailsHeader(%q) = %v, want an error", tc.in, got)
			}
		})
	}
}
