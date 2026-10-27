package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

const sigOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id>
    <value>sig.v1</value>
  </template_id>
  <concept>Signature</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE" kind="x"><rm_attribute_name>category</rm_attribute_name></attributes>
    <template_id><value>sig.v1</value></template_id>
  </definition>
</template>`

func TestOPTSignature_REQ082(t *testing.T) {
	t.Parallel()
	base := signatureOf([]byte(sigOPT))
	same := map[string]string{
		"a byte-order mark": string(utf8BOM) + sigOPT,
		"another xsi prefix": `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:x="http://www.w3.org/2001/XMLSchema-instance">
  <template_id>
    <value>sig.v1</value>
  </template_id>
  <concept>Signature</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <attributes x:type="C_SINGLE_ATTRIBUTE" kind="x"><rm_attribute_name>category</rm_attribute_name></attributes>
    <template_id><value>sig.v1</value></template_id>
  </definition>
</template>`,
		"other indentation and attribute order": `<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1"><template_id><value> sig.v1 </value></template_id><concept>Signature</concept><definition><rm_type_name>COMPOSITION</rm_type_name><attributes kind="x" xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>category</rm_attribute_name></attributes><template_id><value>sig.v1</value></template_id></definition></template>`,
	}
	for name, doc := range same {
		if got := signatureOf([]byte(doc)); got != base {
			t.Errorf("signatureOf(%s) = %s, want %s: the same template", name, got, base)
		}
	}
	other := map[string]string{
		"another concept":   replaceOnce(sigOPT, "<concept>Signature</concept>", "<concept>Other</concept>"),
		"another attribute": replaceOnce(sigOPT, `kind="x"`, `kind="y"`),
		// A term code names its terminology before the colon.
		"another terminology in an attribute value": replaceOnce(sigOPT, `kind="x"`, `kind="SNOMED-CT::x"`),
		"another node": replaceOnce(sigOPT, "<rm_attribute_name>category</rm_attribute_name>", "<rm_attribute_name>category</rm_attribute_name><existence/>"),
	}
	for name, doc := range other {
		if got := signatureOf([]byte(doc)); got == base {
			t.Errorf("signatureOf(%s) = the original's signature, want a different one", name)
		}
	}
}

func replaceOnce(s, old, repl string) string {
	return string(bytes.Replace([]byte(s), []byte(old), []byte(repl), 1))
}

func TestRenamedOPT_RenamesTheTemplateIDOnly_REQ082(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(vendoredOPT())
	if err != nil {
		t.Fatal(err)
	}
	newID := testTemplate + renameSuffix
	b, err := renamedOPT(append(append([]byte(nil), utf8BOM...), raw...), newID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(b, utf8BOM) {
		t.Error("renamedOPT kept the byte-order mark")
	}
	opt, err := template.ParseOPTStrict(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("ParseOPTStrict(renamed) = %v", err)
	}
	if got := opt.TemplateID(); got != newID {
		t.Errorf("renamed template id = %q, want %q", got, newID)
	}
	if got, want := bytes.Count(b, []byte(newID)), 1; got != want {
		t.Errorf("renamed OPT holds the new id %d times, want %d: only template/template_id/value changes", got, want)
	}
	if got, want := bytes.Count(b, []byte(">"+testTemplate+"<")), bytes.Count(raw, []byte(">"+testTemplate+"<"))-1; got != want {
		t.Errorf("renamed OPT holds the old id %d times, want %d", got, want)
	}
}

func TestRenamedTemplate_SignsTheBytesItWrites_REQ082(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(vendoredOPT())
	if err != nil {
		t.Fatal(err)
	}
	// An OPT that holds the credential text is written redacted.
	opt := filepath.Join(t.TempDir(), "in.opt")
	if err := os.WriteFile(opt, bytes.Replace(raw, []byte("<concept>Minimal evaluation</concept>"), []byte("<concept>op:s3cr3t</concept>"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	tpl := templateRecord{TemplateID: testTemplate, File: opt, Kept: true}
	r := &runner{s: &sink{dir: t.TempDir(), red: newRedactor([]cdrTarget{{name: "x", user: "op", pass: "s3cr3t"}})}, templates: []templateRecord{tpl}}
	nt, err := r.renamedTemplate(tpl)
	if err != nil {
		t.Fatalf("renamedTemplate = %v", err)
	}
	written, err := os.ReadFile(nt.File)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(written, []byte("s3cr3t")) {
		t.Fatalf("the renamed OPT holds the credential; want it written redacted")
	}
	if want := signatureOf(written); nt.Signature != want {
		t.Errorf("renamed template signature = %s, want %s, the signature of the file the upload reads", nt.Signature, want)
	}
}

func TestRenamedTemplate_RefusesAnIDAnotherTemplateHas_REQ082(t *testing.T) {
	t.Parallel()
	newID := testTemplate + renameSuffix
	tpl := templateRecord{TemplateID: testTemplate, File: vendoredOPT(), Kept: true}
	out := t.TempDir()
	r := &runner{s: &sink{dir: out}, templates: []templateRecord{tpl, {TemplateID: newID, File: "other.opt", Kept: true}}}
	if nt, err := r.renamedTemplate(tpl); err == nil || !strings.Contains(err.Error(), "another template") {
		t.Fatalf("renamedTemplate with %s taken = %+v, %v, want a refusal naming another template", newID, nt, err)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Errorf("renamedTemplate wrote %d entries, want none for a refused rename", len(entries))
	}
}

func TestCollectTemplates_KeepsTheFirstOfOneID_REQ082(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, doc string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.opt", sigOPT)
	write("b.opt", replaceOnce(sigOPT, "<concept>Signature</concept>", "<concept>Other</concept>"))
	write("c.OPT", sigOPT)
	write("d.opt", "<template><concept>no id</concept></template>")
	write("e.xml", sigOPT)
	recs, err := collectTemplates(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		file    string
		kept    bool
		skipped string
	}{
		{"a.opt", true, ""},
		{"b.opt", false, "same template id, other content"},
		{"c.OPT", false, "same template id, same content"},
		{"d.opt", false, "no template_id"},
	}
	if len(recs) != len(want) {
		t.Fatalf("collectTemplates = %d records, want %d: %+v", len(recs), len(want), recs)
	}
	for i, w := range want {
		r := recs[i]
		if filepath.Base(r.File) != w.file || r.Kept != w.kept || r.Skipped != w.skipped {
			t.Errorf("record %d = {%s kept=%v skipped=%q}, want {%s kept=%v skipped=%q}", i, filepath.Base(r.File), r.Kept, r.Skipped, w.file, w.kept, w.skipped)
		}
	}
	if recs[0].TemplateID != "sig.v1" || recs[0].Root != "COMPOSITION" {
		t.Errorf("record 0 = id %q root %q, want sig.v1 COMPOSITION", recs[0].TemplateID, recs[0].Root)
	}
}
