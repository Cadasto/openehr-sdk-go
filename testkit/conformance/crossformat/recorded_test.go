package crossformat_test

// Tests for the recorded outcomes of PROBE-105 (REQ-080): the rules every
// record obeys, how a measured outcome is matched against its record, and the
// mismatch lines the probe reports.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/conformance/crossformat"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestProbe105RecordedFollowsRules holds the committed table to the record
// rules against the vendored corpus: every set and every leg it runs has a
// record, nothing else does, and every record states its reason and compares
// something.
func TestProbe105RecordedFollowsRules(t *testing.T) {
	sets, err := fixtures.ListCrossFormatSets()
	if err != nil {
		t.Fatalf("ListCrossFormatSets() error = %v", err)
	}
	if len(sets) == 0 {
		t.Fatal("ListCrossFormatSets() returned no sets; the rules would be checked against nothing")
	}
	if err := crossformat.CheckRecords(sets, crossformat.Recorded); err != nil {
		t.Errorf("CheckRecords(corpus, Recorded) = %v", err)
	}
}

// flatStructuredSet runs exactly the two FLAT and STRUCTURED legs.
var flatStructuredSet = fixtures.CrossFormatSet{Name: "s", OPT: "s.opt", FLAT: "flat.json", STRUCTURED: "structured.json"}

func cleanRecords() map[crossformat.Leg]crossformat.Record {
	return map[crossformat.Leg]crossformat.Record{
		crossformat.LegFlatStructured: {Outcome: crossformat.Outcome{Compared: 3}},
		crossformat.LegStructuredFlat: {Outcome: crossformat.Outcome{Compared: 2}},
	}
}

// TestProbe105CheckSetRecords pins each record rule PROBE-105 states: a
// refusal or difference states why it exists, a leg that is not refused
// compares at least one key, a leg the set runs has a record and a record names
// only a leg the set runs.
func TestProbe105CheckSetRecords(t *testing.T) {
	tests := []struct {
		name    string
		change  func(map[crossformat.Leg]crossformat.Record)
		wantErr string
	}{
		{name: "clean records", change: func(map[crossformat.Leg]crossformat.Record) {}},
		{
			name: "difference with a reason",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 3, Missing: 1}, Reason: "why"}
			},
		},
		{
			name: "refusal with a reason",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Refused: "boom"}, Reason: "why"}
			},
		},
		{
			name: "difference without a reason",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 3, Extra: 1}}
			},
			wantErr: "must state why",
		},
		{
			name: "refusal with a blank reason",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Refused: "boom"}, Reason: "  "}
			},
			wantErr: "must state why",
		},
		{
			name: "clean record with a stale reason",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 3}, Reason: "gap closed"}
			},
			wantErr: "carries no reason",
		},
		{
			name: "comparison over nothing",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{}}
			},
			wantErr: "at least one key",
		},
		{
			name: "refusal with counts",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Refused: "boom", Compared: 2}, Reason: "why"}
			},
			wantErr: "no counts",
		},
		{
			name: "negative count",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 3, Extra: -1}, Reason: "why"}
			},
			wantErr: "negative",
		},
		{
			name: "more missing and altered than compared",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 3, Missing: 2, Altered: 2}, Reason: "why"}
			},
			wantErr: "exceed",
		},
		{
			name:    "leg the set runs without a record",
			change:  func(r map[crossformat.Leg]crossformat.Record) { delete(r, crossformat.LegStructuredFlat) },
			wantErr: "structured-flat: the set runs this leg but it has no record",
		},
		{
			name: "record for a leg the set lacks",
			change: func(r map[crossformat.Leg]crossformat.Record) {
				r[crossformat.LegJSONXML] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 1}}
			},
			wantErr: "json-xml: recorded, but the set does not carry both of its formats",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := cleanRecords()
			tt.change(recs)
			err := crossformat.CheckSetRecords(flatStructuredSet, recs)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("CheckSetRecords() = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("CheckSetRecords() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestProbe105CheckRecords pins the corpus-level rules: a set the corpus
// carries has records, and the table records no set the corpus lacks.
func TestProbe105CheckRecords(t *testing.T) {
	sets := []fixtures.CrossFormatSet{flatStructuredSet}
	if err := crossformat.CheckRecords(sets, map[string]map[crossformat.Leg]crossformat.Record{"s": cleanRecords()}); err != nil {
		t.Errorf("CheckRecords(complete table) = %v, want nil", err)
	}
	err := crossformat.CheckRecords(sets, map[string]map[crossformat.Leg]crossformat.Record{"other": cleanRecords()})
	for _, want := range []string{"set s: the corpus carries it but it has no records", "set other: recorded, but the corpus has no such set"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckRecords(table for another set) = %v, want an error containing %q", err, want)
		}
	}
}

// TestProbe105RecordMatches pins the ratchet's comparison: counts match
// exactly, a refusal matches by a substring of the error, and a refusal never
// matches counts or the other way round.
func TestProbe105RecordMatches(t *testing.T) {
	counts := crossformat.Outcome{Compared: 5, Missing: 1, Extra: 2, Altered: 3}
	refused := crossformat.Outcome{Refused: "decode: strconv.ParseUint: parsing \"x\": invalid syntax"}
	tests := []struct {
		name string
		rec  crossformat.Outcome
		got  crossformat.Outcome
		want bool
	}{
		{"same counts", counts, counts, true},
		{"one more compared", counts, crossformat.Outcome{Compared: 6, Missing: 1, Extra: 2, Altered: 3}, false},
		{"one fewer missing", counts, crossformat.Outcome{Compared: 5, Extra: 2, Altered: 3}, false},
		{"one more extra", counts, crossformat.Outcome{Compared: 5, Missing: 1, Extra: 3, Altered: 3}, false},
		{"one fewer altered", counts, crossformat.Outcome{Compared: 5, Missing: 1, Extra: 2, Altered: 2}, false},
		{"refusal containing the record", crossformat.Outcome{Refused: "strconv.ParseUint"}, refused, true},
		{"refusal not containing the record", crossformat.Outcome{Refused: "path resolves to multiple items"}, refused, false},
		{"counts recorded, refusal measured", counts, refused, false},
		{"refusal recorded, counts measured", crossformat.Outcome{Refused: "strconv.ParseUint"}, counts, false},
		{
			"refusal worded with the other json/v2 modal verb",
			crossformat.Outcome{Refused: "cannot unmarshal JSON number 1.0"},
			crossformat.Outcome{Refused: "decode: json: unable to unmarshal JSON number 1.0 into Go int32"},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := crossformat.Record{Outcome: tt.rec, Reason: "why"}
			if got := rec.Matches(tt.got); got != tt.want {
				t.Errorf("Record{%v}.Matches(%v) = %v, want %v", tt.rec, tt.got, got, tt.want)
			}
		})
	}
}

// TestProbe105Verify pins the mismatch lines the probe reports: none when every
// leg matches, and otherwise one per leg naming the set, the leg, the recorded
// and the measured outcome, including a leg with no record and a record whose
// leg did not run.
func TestProbe105Verify(t *testing.T) {
	res := crossformat.SetResult{Set: "s", Legs: []crossformat.LegResult{
		{Leg: crossformat.LegFlatStructured, Outcome: crossformat.Outcome{Compared: 3}},
		{Leg: crossformat.LegStructuredFlat, Outcome: crossformat.Outcome{Compared: 2}},
	}}
	if got := crossformat.Verify(res, cleanRecords()); got != nil {
		t.Errorf("Verify(matching records) = %q, want nil", got)
	}

	recs := cleanRecords()
	recs[crossformat.LegFlatStructured] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 3, Missing: 1}, Reason: "why"}
	delete(recs, crossformat.LegStructuredFlat)
	recs[crossformat.LegJSONXML] = crossformat.Record{Outcome: crossformat.Outcome{Compared: 1}}
	got := crossformat.Verify(res, recs)
	want := []string{
		"s flat-structured: recorded compared 3, missing 1, extra 0, altered 0, measured compared 3, missing 0, extra 0, altered 0",
		"s structured-flat: no record, measured compared 2, missing 0, extra 0, altered 0",
		"s json-xml: recorded compared 1, missing 0, extra 0, altered 0, but the leg did not run",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Verify(changed records) =\n%q\nwant\n%q", got, want)
	}
}

// TestProbe105Legs pins which legs a set runs: each needs both of its formats,
// and a canonical leg takes canonical XML when the set has no canonical JSON.
func TestProbe105Legs(t *testing.T) {
	const j, x, f, s = "c.json", "c.xml", "flat.json", "structured.json"
	all := []crossformat.Leg{
		crossformat.LegJSONXML, crossformat.LegCanonicalFlat, crossformat.LegFlatCanonical,
		crossformat.LegFlatStructured, crossformat.LegStructuredFlat,
	}
	tests := []struct {
		name string
		set  fixtures.CrossFormatSet
		want []crossformat.Leg
	}{
		{"all four formats", fixtures.CrossFormatSet{CanonicalJSON: j, CanonicalXML: x, FLAT: f, STRUCTURED: s}, all},
		{"JSON and XML", fixtures.CrossFormatSet{CanonicalJSON: j, CanonicalXML: x}, []crossformat.Leg{crossformat.LegJSONXML}},
		{"JSON and FLAT", fixtures.CrossFormatSet{CanonicalJSON: j, FLAT: f}, []crossformat.Leg{crossformat.LegCanonicalFlat, crossformat.LegFlatCanonical}},
		{"XML and FLAT", fixtures.CrossFormatSet{CanonicalXML: x, FLAT: f}, []crossformat.Leg{crossformat.LegCanonicalFlat, crossformat.LegFlatCanonical}},
		{"FLAT and STRUCTURED", fixtures.CrossFormatSet{FLAT: f, STRUCTURED: s}, []crossformat.Leg{crossformat.LegFlatStructured, crossformat.LegStructuredFlat}},
		{"JSON and STRUCTURED share no leg", fixtures.CrossFormatSet{CanonicalJSON: j, STRUCTURED: s}, nil},
		{"one format", fixtures.CrossFormatSet{FLAT: f}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := crossformat.Legs(tt.set); !slices.Equal(got, tt.want) {
				t.Errorf("Legs(%+v) = %v, want %v", tt.set, got, tt.want)
			}
		})
	}
}

// TestProbe105RunRefusesMisuse pins Run's harness faults: a set it cannot run
// at all is an error, never a set of empty outcomes. Each case is a corpus set
// with one thing taken away, so only the check under test can refuse it.
func TestProbe105RunRefusesMisuse(t *testing.T) {
	sets, err := fixtures.ListCrossFormatSets()
	if err != nil {
		t.Fatalf("ListCrossFormatSets() error = %v", err)
	}
	i := slices.IndexFunc(sets, func(s fixtures.CrossFormatSet) bool { return s.FLAT != "" && s.STRUCTURED != "" })
	if i < 0 {
		t.Fatal("no corpus set carries FLAT and STRUCTURED")
	}
	base := sets[i]
	if _, err := crossformat.Run(base); err != nil {
		t.Fatalf("Run(%s) error = %v, want the unchanged set to run", base.Name, err)
	}
	for name, change := range map[string]func(*fixtures.CrossFormatSet){
		"no name":     func(s *fixtures.CrossFormatSet) { s.Name = "" },
		"no OPT":      func(s *fixtures.CrossFormatSet) { s.OPT = "" },
		"no leg":      func(s *fixtures.CrossFormatSet) { s.CanonicalJSON, s.CanonicalXML, s.STRUCTURED = "", "", "" },
		"missing OPT": func(s *fixtures.CrossFormatSet) { s.OPT = "does/not/exist.opt" },
	} {
		set := base
		change(&set)
		if _, err := crossformat.Run(set); err == nil {
			t.Errorf("Run(%s with %s) error = nil, want a harness fault", base.Name, name)
		}
	}
}

// TestProbe105LegRefusalIsTheCodecError pins, on the corpus, that a refused
// leg records the error of the codec that ended it, named by its step, rather
// than a harness fault or the reducing decode's account of it.
func TestProbe105LegRefusalIsTheCodecError(t *testing.T) {
	sets, err := fixtures.ListCrossFormatSets()
	if err != nil {
		t.Fatalf("ListCrossFormatSets() error = %v", err)
	}
	var refusals int
	for _, set := range sets {
		res, err := crossformat.Run(set)
		if err != nil {
			t.Fatalf("Run(%s) error = %v", set.Name, err)
		}
		for _, lr := range res.Legs {
			if lr.Outcome.Refused == "" {
				continue
			}
			refusals++
			step, _, ok := strings.Cut(lr.Outcome.Refused, ": ")
			if !ok || !slices.Contains([]string{"canonical JSON decode", "canonical XML decode", "canonical JSON encode", "FLAT encode", "FLAT decode", "FLAT to STRUCTURED", "STRUCTURED to FLAT"}, step) {
				t.Errorf("%s %s: refusal %q does not start with the codec step that ended the leg", set.Name, lr.Leg, crossformat.Abbreviate(lr.Outcome.Refused))
			}
			if strings.Contains(lr.Outcome.Refused, "harness fault") {
				t.Errorf("%s %s: refusal %q carries the reducing decode's account instead of the codec's error", set.Name, lr.Leg, crossformat.Abbreviate(lr.Outcome.Refused))
			}
		}
	}
	if refusals == 0 {
		t.Fatal("no leg of the corpus is refused, so this test checks nothing; drop it or pick another input")
	}
}
