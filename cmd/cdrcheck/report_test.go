package main

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// cannedRun is a run's records written by hand: two CDRs, two templates
// (one renamed), and documents whose verdicts cover every case the summary
// counts.
func cannedRun() runData {
	ok, bad := new(true), new(false)
	ebRefusal := outcome{Status: 422, Body: `{"error":"Unprocessable Entity","message":"first problem, /content[openEHR-EHR-X]/data: second problem"}`}
	feRefusal := outcome{Status: 400, Body: `{"message":"invalid","validationErrors":["Mandatory node missing at /x","uid 0a0b0c0d-1111-4222-8333-444455556666::fe::1  refused"]}`}
	feRefusal2 := outcome{Status: 400, Body: `{"message":"invalid","validationErrors":["Mandatory node missing at /x"]}`}
	accepted := outcome{Status: 201, OK: true}
	gen := func(tid, policy, fill string, self *bool) genRecord {
		return genRecord{TemplateID: tid, Policy: policy, Fill: fill, File: "gen/" + tid + "/" + policy + "-" + fill + ".json", RootType: "COMPOSITION", SelfOK: self, RMOK: ok}
	}
	post := func(cdr, tid, label string, o outcome) postRecord {
		return postRecord{CDR: cdr, TemplateID: tid, Label: label, outcome: o}
	}
	cross := func(cdr, src, level string, o outcome) postRecord {
		return postRecord{CDR: cdr, TemplateID: "A", Label: src + ":" + level, Source: src, outcome: o}
	}
	return runData{
		cdrs: []string{"ehrbase", "ferroehr"},
		templates: []templateRecord{
			{TemplateID: "A", Root: "COMPOSITION", Kept: true},
			{TemplateID: "B", Root: "COMPOSITION", RenamedTo: "B.xc1"},
			{TemplateID: "A", Root: "COMPOSITION", Skipped: "same template id, same content"},
			{TemplateID: "B.xc1", Root: "COMPOSITION", Kept: true, RenamedFrom: "B"},
		},
		uploads: []uploadRecord{
			// B's own uploads, superseded by B.xc1: kept, and left out of the counts.
			{CDR: "ehrbase", TemplateID: "B", outcome: accepted, Present: true, SupersededBy: "B.xc1"},
			{CDR: "ferroehr", TemplateID: "B", Status: 409, StoredSame: bad, SupersededBy: "B.xc1"},
			{CDR: "ehrbase", TemplateID: "A", outcome: accepted, Present: true},
			{CDR: "ehrbase", TemplateID: "B.xc1", outcome: accepted, Present: true},
			{CDR: "ferroehr", TemplateID: "A", Status: 409, StoredSame: ok, Present: true},
			{CDR: "ferroehr", TemplateID: "B.xc1", Status: 400, Body: `{"message":"existence is looser than the RM's"}`},
		},
		gens: []genRecord{
			gen("A", "required", "example", ok),
			gen("A", "medium", "example", ok),
			gen("A", "large", "example", bad),
			{TemplateID: "A", Policy: "required", Fill: "random1", Stage: "generate", Error: "boom"},
			gen("A", "medium", "random1", ok),
			gen("B.xc1", "required", "example", ok),
		},
		posts: []postRecord{
			post("ehrbase", "A", "required-example", ebRefusal),
			post("ferroehr", "A", "required-example", feRefusal),
			post("ehrbase", "A", "medium-example", accepted),
			post("ferroehr", "A", "medium-example", feRefusal2),
			post("ehrbase", "A", "large-example", ebRefusal),
			post("ferroehr", "A", "large-example", accepted),
			{CDR: "ehrbase", TemplateID: "A", Label: "medium-random1", Stage: "ehr", Status: 503},
			post("ferroehr", "A", "medium-random1", accepted),
			post("ehrbase", "B.xc1", "required-example", outcome{Status: 422, Body: "plain text refusal"}),
			// Cut short by the end of the run: recorded, not counted.
			post("ferroehr", "B.xc1", "required-example", outcome{Error: "context canceled", Stopped: true}),
		},
		ehrs: []ehrRecord{
			{CDR: "ehrbase", TemplateID: "A", Label: "required-example", outcome: accepted},
			{CDR: "ehrbase", TemplateID: "A", Label: "medium-random1", Status: 503, Body: `{"message":"unavailable"}`},
			{CDR: "ferroehr", TemplateID: "A", Label: "required-example", outcome: accepted},
			{CDR: "ferroehr", TemplateID: "B.xc1", Label: "required-example", Error: "context canceled", Stopped: true},
		},
		examples: []exampleRecord{
			{CDR: "ehrbase", TemplateID: "A", Level: "default", File: "x", RootType: "COMPOSITION", outcome: accepted},
			{CDR: "ehrbase", TemplateID: "B.xc1", Level: "default", Status: 500},
			{CDR: "ferroehr", TemplateID: "A", Level: "required", File: "x", RootType: "COMPOSITION", outcome: accepted},
			{CDR: "ferroehr", TemplateID: "A", Level: "medium", File: "x", RootType: "COMPOSITION", outcome: accepted},
			{CDR: "ferroehr", TemplateID: "A", Level: "complete", Status: 400},
		},
		validates: []validateRecord{
			{CDR: "ehrbase", TemplateID: "A", Level: "default", OK: ok},
			{CDR: "ferroehr", TemplateID: "A", Level: "required", OK: ok},
			{CDR: "ferroehr", TemplateID: "A", Level: "medium", OK: bad},
		},
		cross: []postRecord{
			cross("ehrbase", "ehrbase", "default", accepted),
			cross("ferroehr", "ehrbase", "default", feRefusal2),
			cross("ehrbase", "ferroehr", "required", accepted),
			cross("ferroehr", "ferroehr", "required", accepted),
			cross("ehrbase", "ferroehr", "medium", ebRefusal),
			cross("ferroehr", "ferroehr", "medium", accepted),
		},
	}
}

func TestSummarize_CannedRun_PROBE108_REQ107_REQ102(t *testing.T) {
	t.Parallel()
	s := summarize(cannedRun())

	if got := s.Templates; got.Files != 3 || got.Kept != 2 || got.Skipped != 1 || !slices.Equal(got.Renamed, []string{"B.xc1"}) || got.Roots["COMPOSITION"] != 2 {
		t.Errorf("templates = %+v, want 3 files, 2 kept, 1 skipped, B.xc1 renamed, 2 COMPOSITION roots", got)
	}
	g := s.Generation
	if g.Runs != 6 || g.Errors != 1 || !slices.Equal(g.ErrorTemplates, []string{"A"}) || g.NotSelfValid != 1 || g.NotRMValid != 0 {
		t.Errorf("generation = %+v, want 6 runs, 1 error in A, 1 not self-valid, 0 not RM-valid", g)
	}

	want := map[string]cdrSummary{
		"ehrbase": {
			TemplatesPresent: ratio{2, 2}, DocumentsAccepted: ratio{1, 4}, EHRFailures: 1,
			ExamplesFetched: ratio{1, 2}, ExamplesSDKValid: ratio{1, 1},
			OwnExamplesAccepted: ratio{1, 1}, OtherExamplesAccepted: ratio{1, 2},
		},
		"ferroehr": {
			TemplatesPresent: ratio{1, 2}, DocumentsAccepted: ratio{2, 4},
			ExamplesFetched: ratio{2, 3}, ExamplesSDKValid: ratio{1, 2},
			OwnExamplesAccepted: ratio{2, 2}, OtherExamplesAccepted: ratio{0, 1},
		},
	}
	if len(s.PerCDR) != 2 {
		t.Fatalf("per CDR = %d entries, want 2", len(s.PerCDR))
	}
	for _, c := range s.PerCDR {
		w := want[c.Name]
		got := cdrSummary{
			TemplatesPresent: c.TemplatesPresent, DocumentsAccepted: c.DocumentsAccepted, EHRFailures: c.EHRFailures,
			ExamplesFetched: c.ExamplesFetched, ExamplesSDKValid: c.ExamplesSDKValid,
			OwnExamplesAccepted: c.OwnExamplesAccepted, OtherExamplesAccepted: c.OtherExamplesAccepted,
		}
		if got.TemplatesPresent != w.TemplatesPresent || got.DocumentsAccepted != w.DocumentsAccepted || got.EHRFailures != w.EHRFailures ||
			got.ExamplesFetched != w.ExamplesFetched || got.ExamplesSDKValid != w.ExamplesSDKValid ||
			got.OwnExamplesAccepted != w.OwnExamplesAccepted || got.OtherExamplesAccepted != w.OtherExamplesAccepted {
			t.Errorf("%s = %+v, want %+v", c.Name, got, w)
		}
	}

	// Of the SDK-valid documents: A required-example refused by both, A
	// medium-example by FerroEHR alone. B.xc1 required-example is refused by
	// EHRbase where FerroEHR holds no template, and A large-example is not
	// SDK-valid: neither counts.
	if s.SDKValidRefused.ByAll != 1 || s.SDKValidRefused.Alone["ferroehr"] != 1 || s.SDKValidRefused.Alone["ehrbase"] != 0 {
		t.Errorf("SDK-valid refused = %+v, want 1 by all, 1 by ferroehr alone, 0 by ehrbase alone", s.SDKValidRefused)
	}
	wantGen := map[string]int{
		"self-ok|ehrbase-bad|ferroehr-bad": 1,
		"self-ok|ehrbase-ok|ferroehr-bad":  1,
		"self-bad|ehrbase-bad|ferroehr-ok": 1,
		"self-ok|ehrbase-na|ferroehr-ok":   1,
		"self-ok|ehrbase-bad|ferroehr-na":  1,
	}
	if !maps.Equal(s.GeneratedMatrix, wantGen) {
		t.Errorf("generated matrix = %v, want %v", s.GeneratedMatrix, wantGen)
	}
	wantEx := map[string]int{
		"ehrbase|sdk-ok|ehrbase-ok|ferroehr-bad":   1,
		"ferroehr|sdk-ok|ehrbase-ok|ferroehr-ok":   1,
		"ferroehr|sdk-bad|ehrbase-bad|ferroehr-ok": 1,
	}
	if !maps.Equal(s.ExampleMatrix, wantEx) {
		t.Errorf("example matrix = %v, want %v", s.ExampleMatrix, wantEx)
	}

	eb := s.PerCDR[0].DocumentRefusals
	wantEB := []struct {
		detail string
		docs   int
	}{
		{"/content[openEHR-EHR-X]/data: second problem", 2},
		{"first problem", 2},
		{"plain text refusal", 1},
	}
	if len(eb) != len(wantEB) {
		t.Fatalf("ehrbase document refusals = %+v, want %d groups", eb, len(wantEB))
	}
	for i, w := range wantEB {
		if eb[i].Detail != w.detail || eb[i].Documents != w.docs || eb[i].Status != 422 {
			t.Errorf("ehrbase refusal group %d = %+v, want 422 %q on %d documents", i, eb[i], w.detail, w.docs)
		}
	}
	if got := eb[1].ByPolicy; got["required"] != 1 || got["large"] != 1 {
		t.Errorf("ehrbase group %q by policy = %v, want required 1, large 1", eb[1].Detail, got)
	}
	fe := s.PerCDR[1].DocumentRefusals
	if len(fe) != 2 || fe[0].Detail != "Mandatory node missing at /x" || fe[0].Documents != 2 || fe[1].Detail != "uid <uid> refused" {
		t.Errorf("ferroehr document refusals = %+v, want the missing node on 2 documents, then the uid line normalised", fe)
	}
	if up := s.PerCDR[1].UploadRefusals; len(up) != 1 || up[0].Status != 400 || up[0].Templates != 1 {
		t.Errorf("ferroehr upload refusals = %+v, want one 400 group on one template", up)
	}
	if got := s.PerCDR[1].StoredDiffers; !slices.Equal(got, []string{"B"}) {
		t.Errorf("ferroehr stored differs = %q, want B, from its superseded upload", got)
	}
	if eb := s.PerCDR[0]; eb.EHRsCreated != (ratio{1, 2}) || len(eb.EHRRefusals) != 1 || eb.EHRRefusals[0].Status != 503 || eb.EHRRefusals[0].Detail != "unavailable" {
		t.Errorf("ehrbase EHRs created %v, refusals %+v, want 1 / 2 and one 503 group", eb.EHRsCreated, eb.EHRRefusals)
	}
	if fe := s.PerCDR[1]; fe.EHRsCreated != (ratio{1, 1}) || len(fe.EHRRefusals) != 0 {
		t.Errorf("ferroehr EHRs created %v, refusals %+v, want 1 / 1 and none: a stopped create is not counted", fe.EHRsCreated, fe.EHRRefusals)
	}

	if len(s.RefusedSDKValid) != 2 || s.RefusedSDKValid[0].TemplateID != "A" ||
		!slices.Equal(s.RefusedSDKValid[0].Labels, []string{"medium-example", "required-example"}) {
		t.Errorf("refused SDK-valid = %+v, want A (medium-example, required-example) then B.xc1", s.RefusedSDKValid)
	}

	md := markdown(runInfo{CDRStages: "ran"}, s)
	for _, want := range []string{
		"| Templates accepted at upload | 2 / 2 | 1 / 2 |",
		"| SDK documents accepted | 1 / 4 | 2 / 4 |",
		"| CDR accepts the other CDRs' examples | 1 / 2 | 0 / 1 |",
		"- 1 were refused by every CDR;",
		"- 1 by ferroehr alone",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("summary.md lacks %q", want)
		}
	}
}

func TestRefusalDetails_PROBE108(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		o    outcome
		want []string
	}{
		{
			name: "joined message", o: outcome{Body: `{"message":"a is wrong, /content[x]/items: b is wrong, /context/other_context: c"}`},
			want: []string{"a is wrong", "/content[x]/items: b is wrong", "/context/other_context: c"},
		},
		{
			name: "validationErrors", o: outcome{Body: `{"message":"m","validationErrors":["x  y", {"path":"/p"}]}`},
			want: []string{"x y", `{"path":"/p"}`},
		},
		{name: "not JSON", o: outcome{Body: "  Internal\n error  "}, want: []string{"Internal error"}},
		{name: "no body", o: outcome{Error: "connection refused"}, want: []string{"connection refused"}},
	}
	for _, tc := range cases {
		if got := refusalDetails(tc.o); !slices.Equal(got, tc.want) {
			t.Errorf("refusalDetails(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestWithoutUID_DropsTheTopLevelUIDOnly_PROBE108(t *testing.T) {
	t.Parallel()
	in := `{"_type":"COMPOSITION","uid":{"value":"u::x::1"},"name":{"value":"n"},"content":[{"_type":"OBSERVATION","uid":{"value":"keep"}}]}`
	got, err := withoutUID([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"_type":"COMPOSITION","name":{"value":"n"},"content":[{"_type":"OBSERVATION","uid":{"value":"keep"}}]}`
	if string(got) != want {
		t.Errorf("withoutUID = %s, want %s", got, want)
	}
	if _, err := withoutUID([]byte(`[1]`)); err == nil {
		t.Error("withoutUID([1]) = nil error, want a refusal: not an object")
	}
}
