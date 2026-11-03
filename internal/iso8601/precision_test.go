package iso8601_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
)

// dateFields reads s as a date, failing the test when the parse refuses it.
func dateFields(t *testing.T, s string) iso8601.Fields {
	t.Helper()
	d, err := iso8601.ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", s, err)
	}
	return iso8601.DateFields(d)
}

// timeFields reads s as a time, failing the test when the parse refuses it.
func timeFields(t *testing.T, s string) iso8601.Fields {
	t.Helper()
	tm, err := iso8601.ParseTime(s)
	if err != nil {
		t.Fatalf("ParseTime(%q): %v", s, err)
	}
	return iso8601.TimeFields(tm)
}

// dateTimeFields reads s as a date-time, failing the test when the parse
// refuses it. The text carries a time exactly when it holds a T.
func dateTimeFields(t *testing.T, s string) iso8601.Fields {
	t.Helper()
	d, tm, err := iso8601.ParseDateTime(s)
	if err != nil {
		t.Fatalf("ParseDateTime(%q): %v", s, err)
	}
	return iso8601.DateTimeFields(d, tm, strings.Contains(s, "T"))
}

// TestREQ103_FieldsCompareSharedPrecision pins the order of REQ-103's
// temporal range check on two values of one type written with one zone
// designator: field by field from the year down, over the fields both
// carry, the second with its fraction as one field. The first field that
// differs decides, and values equal on every shared field are equal, so a
// reduced value covers the whole period it names. The zone is not applied.
func TestREQ103_FieldsCompareSharedPrecision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind string // "date", "time" or "date-time"
		a, b string
		want int
	}{
		{kind: "date", a: "2021", b: "2021-05", want: 0},
		{kind: "date", a: "2021-05", b: "2021-05-15", want: 0},
		{kind: "date", a: "2021-12-31", b: "2021", want: 0},
		{kind: "date", a: "2021-05", b: "2021-06-01", want: -1},
		{kind: "date", a: "2022", b: "2021-12-31", want: 1},
		{kind: "date", a: "2021-05-15", b: "2021-05-14", want: 1},
		{kind: "date", a: "20210515", b: "2021-05-15", want: 0},
		{kind: "time", a: "10", b: "10:30", want: 0},
		{kind: "time", a: "10:30", b: "10:30:15.5", want: 0},
		{kind: "time", a: "10:30:15", b: "10:30:15.5", want: -1},
		{kind: "time", a: "10:30:16", b: "10:30:15.5", want: 1},
		{kind: "time", a: "10:30:15.5", b: "10:30:15,50", want: 0},
		{kind: "time", a: "09:59:59.999", b: "10", want: -1},
		{kind: "time", a: "10:00Z", b: "10:00+02:00", want: 0},
		{kind: "time", a: "1030", b: "10:29", want: 1},
		{kind: "date-time", a: "2021-05-15", b: "2021-05-15T10", want: 0},
		{kind: "date-time", a: "2021", b: "2021-05-15T10:30", want: 0},
		{kind: "date-time", a: "2021-12-31T23:59:30", b: "2021-12-31T23:59", want: 0},
		{kind: "date-time", a: "2021-05-15T00", b: "2021-05-15T10", want: -1},
		{kind: "date-time", a: "2021-05-15T00", b: "2021-05-14T23:59", want: 1},
		{kind: "date-time", a: "2021-05-15T00", b: "2021-05-16", want: -1},
		{kind: "date-time", a: "2021-05-15T10:30:15.5", b: "2021-05-15T10:30:15.25", want: 1},
		{kind: "date-time", a: "20210515T1030", b: "2021-05-15T10:30:00", want: 0},
	}
	read := map[string]func(*testing.T, string) iso8601.Fields{
		"date":      dateFields,
		"time":      timeFields,
		"date-time": dateTimeFields,
	}
	for _, tc := range tests {
		t.Run(tc.kind+" "+tc.a+" vs "+tc.b, func(t *testing.T) {
			t.Parallel()
			a, b := read[tc.kind](t, tc.a), read[tc.kind](t, tc.b)
			if got := a.Compare(b); got != tc.want {
				t.Errorf("Fields(%q).Compare(Fields(%q)) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
			if got := b.Compare(a); got != -tc.want {
				t.Errorf("Fields(%q).Compare(Fields(%q)) = %d, want %d", tc.b, tc.a, got, -tc.want)
			}
		})
	}
}
