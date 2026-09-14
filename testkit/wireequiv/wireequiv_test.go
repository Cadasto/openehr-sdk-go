package wireequiv_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// TestEquivalent covers the wire-equivalence model REQ-052 allows as a
// secondary check: member order is ignored, array element order is compared,
// string escaping does not matter, and a number past 2^53 is compared as its
// literal text rather than through a lossy float64. A difference is reported at
// its JSON Pointer.
func TestEquivalent(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want bool
		// ptr, when want is false, must appear in the diagnostic so a caller
		// is pointed at the first difference.
		ptr string
	}{
		{
			name: "member order ignored",
			a:    `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`,
			b:    `{"units":"kg","magnitude":80.5,"_type":"DV_QUANTITY"}`,
			want: true,
		},
		{
			name: "nested member order ignored",
			a:    `{"outer":{"a":1,"b":2},"list":[1,2]}`,
			b:    `{"list":[1,2],"outer":{"b":2,"a":1}}`,
			want: true,
		},
		{
			name: "array order compared",
			a:    `{"xs":[1,2,3]}`,
			b:    `{"xs":[3,2,1]}`,
			want: false,
			ptr:  "/xs/0",
		},
		{
			name: "array length compared",
			a:    `{"xs":[1,2,3]}`,
			b:    `{"xs":[1,2]}`,
			want: false,
			ptr:  "/xs",
		},
		{
			name: "escaping ignored",
			a:    `{"match":"<"}`,
			b:    `{"match":"<"}`,
			want: true,
		},
		{
			name: "large integer equal as text",
			a:    `{"n":9007199254740993}`,
			b:    `{"n":9007199254740993}`,
			want: true,
		},
		{
			// 9007199254740993 (2^53 + 1) and 9007199254740992 (2^53) both
			// round to the same float64, so a float-based oracle would call
			// them equal; keeping the literal text catches the difference.
			name: "large integer difference caught as text",
			a:    `{"n":9007199254740993}`,
			b:    `{"n":9007199254740992}`,
			want: false,
			ptr:  "/n",
		},
		{
			name: "differing value reported with its path",
			a:    `{"a":{"b":1}}`,
			b:    `{"a":{"b":2}}`,
			want: false,
			ptr:  "/a/b",
		},
		{
			name: "member present on one side only",
			a:    `{"a":1}`,
			b:    `{"a":1,"c":2}`,
			want: false,
			ptr:  "/c",
		},
		{
			name: "pointer token escaping",
			a:    `{"a/b":1}`,
			b:    `{"a/b":2}`,
			want: false,
			ptr:  "/a~1b",
		},
		{
			name: "type mismatch at root",
			a:    `"1"`,
			b:    `1`,
			want: false,
			ptr:  "(document root)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diag := wireequiv.Equivalent([]byte(tc.a), []byte(tc.b))
			if got != tc.want {
				t.Fatalf("Equivalent(%s, %s) = %v (%q), want %v", tc.a, tc.b, got, diag, tc.want)
			}
			if !tc.want {
				if diag == "" {
					t.Fatalf("Equivalent(%s, %s) reported unequal with an empty diagnostic", tc.a, tc.b)
				}
				if !strings.Contains(diag, tc.ptr) {
					t.Fatalf("Equivalent(%s, %s) diagnostic %q does not name the first difference %q", tc.a, tc.b, diag, tc.ptr)
				}
			}
		})
	}
}

// TestEquivalentReportsInvalidJSON pins the no-panic contract: a document that
// does not parse is reported through the diagnostic, and which side failed is
// named.
func TestEquivalentReportsInvalidJSON(t *testing.T) {
	if ok, diag := wireequiv.Equivalent([]byte(`{`), []byte(`{}`)); ok || !strings.Contains(diag, "left document is not valid JSON") {
		t.Fatalf("Equivalent(invalid, valid) = %v, %q; want false naming the left document", ok, diag)
	}
	if ok, diag := wireequiv.Equivalent([]byte(`{}`), []byte(`]`)); ok || !strings.Contains(diag, "right document is not valid JSON") {
		t.Fatalf("Equivalent(valid, invalid) = %v, %q; want false naming the right document", ok, diag)
	}
}
