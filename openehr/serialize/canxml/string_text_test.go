package canxml_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// stringTextStatus is an EHR_STATUS whose feeder audit sends its optional
// version_id with no text; %s stands for the version_id element.
const stringTextStatus = `<ehr_status xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" archetype_node_id="openEHR-EHR-EHR_STATUS.generic.v1">` +
	`<name><value>EHR Status</value></name>` +
	`<feeder_audit><originating_system_audit><system_id>s</system_id>%s</originating_system_audit></feeder_audit>` +
	`<subject/><is_queryable>true</is_queryable><is_modifiable>true</is_modifiable></ehr_status>`

// TestREQ056_REQ175_WithStringTextKeepsAStringsText pins canxml.WithStringText
// (REQ-056, REQ-175): a String member whose element is not nilled keeps the
// text it carries, empty or white space alone, as canonical JSON keeps "" and
// "  ", so one content decodes to one value in both formats. Such a member
// carries a value: WithMissing leaves it out, and WithPresence still reports
// it as sent empty. Without the option the decode reads it as before; a
// nilled string member and an empty number element read as before under it
// too, and a Decoder applies it as Unmarshal does.
//
// Can-fail control: drop the option's check in the optional or the mandatory
// string path, and the version_id or the name row reads the default value;
// count a kept string as sent without a value, and WithMissing reports
// /name/value.
func TestREQ056_REQ175_WithStringTextKeepsAStringsText(t *testing.T) {
	bmi, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), "compositions", "BMI.xml"))
	if err != nil {
		t.Fatal(err)
	}
	const name = `<name><value>Result Report Date</value></name>`
	if !bytes.Contains(bmi, []byte(name)) {
		t.Fatal("the corpus BMI.xml no longer names its COMPOSITION as the test expects")
	}
	blankName := bytes.Replace(bmi, []byte(name), []byte(`<name><value>  </value></name>`), 1)

	t.Run("mandatory string of white space", func(t *testing.T) {
		for _, tc := range []struct {
			mode string
			opts []canxml.DecoderOption
			want string
		}{
			{"default", nil, ""},
			{"WithStringText", []canxml.DecoderOption{canxml.WithStringText()}, "  "},
			{"WithStringText and WithStrict", []canxml.DecoderOption{canxml.WithStringText(), canxml.WithStrict()}, "  "},
		} {
			var c rm.Composition
			if err := canxml.Unmarshal(blankName, &c, tc.opts...); err != nil {
				t.Fatalf("%s: canxml.Unmarshal = %v", tc.mode, err)
			}
			if got := c.Name.GetValue(); got != tc.want {
				t.Errorf("%s: Name value = %q; want %q", tc.mode, got, tc.want)
			}
		}
		// Canonical JSON keeps the same text.
		var c rm.Composition
		if err := canxml.Unmarshal(blankName, &c, canxml.WithStringText()); err != nil {
			t.Fatal(err)
		}
		js, err := canjson.Marshal(&c)
		if err != nil {
			t.Fatal(err)
		}
		var back rm.Composition
		if err := canjson.Unmarshal(js, &back); err != nil {
			t.Fatal(err)
		}
		if got := back.Name.GetValue(); got != "  " {
			t.Errorf("canonical JSON round trip: Name value = %q; want %q", got, "  ")
		}
	})

	t.Run("optional string with no text", func(t *testing.T) {
		doc := []byte(strings.Replace(stringTextStatus, "%s", `<version_id></version_id>`, 1))
		var plain, kept rm.EHRStatus
		if err := canxml.Unmarshal(doc, &plain); err != nil {
			t.Fatal(err)
		}
		if v := plain.FeederAudit.OriginatingSystemAudit.VersionID; v != nil {
			t.Errorf("default: VersionID = %q; want nil", *v)
		}
		if err := canxml.Unmarshal(doc, &kept, canxml.WithStringText()); err != nil {
			t.Fatal(err)
		}
		if v := kept.FeederAudit.OriginatingSystemAudit.VersionID; v == nil || *v != "" {
			t.Fatalf("WithStringText: VersionID = %v; want a pointer to the empty string", v)
		}
		js, err := canjson.Marshal(&kept)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(js, []byte(`"version_id":""`)) {
			t.Errorf("canjson.Marshal = %s; want it to write \"version_id\":\"\"", js)
		}
	})

	t.Run("reports", func(t *testing.T) {
		var missing []string
		var sent []string
		opts := []canxml.DecoderOption{
			canxml.WithStringText(),
			canxml.WithMissing(func(p string) { missing = append(missing, p) }),
			canxml.WithPresence(func(p string, pr canxml.Presence) { sent = append(sent, p+" "+pr.String()) }),
		}
		if err := canxml.Unmarshal(blankName, new(rm.Composition), opts...); err != nil {
			t.Fatal(err)
		}
		doc := []byte(strings.Replace(stringTextStatus, "%s", `<version_id>`+"\n\t"+`</version_id>`, 1))
		if err := canxml.Unmarshal(doc, new(rm.EHRStatus), opts...); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(missing, "/name/value") {
			t.Errorf("WithMissing reported %q; a kept string carries a value", missing)
		}
		for _, want := range []string{"/name/value empty", "/feeder_audit/originating_system_audit/version_id empty"} {
			if n := count(sent, want); n != 1 {
				t.Errorf("WithPresence reported %q %d times; want once (all: %q)", want, n, sent)
			}
		}
	})

	t.Run("what the option leaves as before", func(t *testing.T) {
		doc := []byte(strings.Replace(stringTextStatus, "%s", `<version_id xsi:nil="true"/>`, 1))
		var s rm.EHRStatus
		if err := canxml.Unmarshal(doc, &s, canxml.WithStringText()); err != nil {
			t.Fatal(err)
		}
		if v := s.FeederAudit.OriginatingSystemAudit.VersionID; v != nil {
			t.Errorf("nilled version_id: VersionID = %q; want nil", *v)
		}
		const quantity = `<dv_quantity xmlns="http://schemas.openehr.org/v1"><magnitude>1</magnitude><units>kg</units><precision></precision></dv_quantity>`
		var q rm.DVQuantity
		if err := canxml.Unmarshal([]byte(quantity), &q, canxml.WithStringText()); err != nil {
			t.Fatal(err)
		}
		if q.Precision != nil {
			t.Errorf("empty precision: Precision = %d; want nil", *q.Precision)
		}
	})

	t.Run("stream decoder", func(t *testing.T) {
		var c rm.Composition
		if err := canxml.NewDecoder(bytes.NewReader(blankName), canxml.WithStringText()).Decode(&c); err != nil {
			t.Fatal(err)
		}
		if got := c.Name.GetValue(); got != "  " {
			t.Errorf("Decoder: Name value = %q; want %q", got, "  ")
		}
	})

	t.Run("terminology code", func(t *testing.T) {
		const doc = `<terminology_code xmlns="http://schemas.openehr.org/v1"><terminology_id> </terminology_id><code_string>c</code_string></terminology_code>`
		var plain, kept rm.TerminologyCode
		if err := canxml.Unmarshal([]byte(doc), &plain); err != nil {
			t.Fatal(err)
		}
		if err := canxml.Unmarshal([]byte(doc), &kept, canxml.WithStringText()); err != nil {
			t.Fatal(err)
		}
		if plain.TerminologyID != "" || kept.TerminologyID != " " {
			t.Errorf("TerminologyID = %q by default and %q with WithStringText; want %q and %q", plain.TerminologyID, kept.TerminologyID, "", " ")
		}
	})
}

// count returns how many items of list equal s.
func count(list []string, s string) int {
	n := 0
	for _, item := range list {
		if item == s {
			n++
		}
	}
	return n
}
