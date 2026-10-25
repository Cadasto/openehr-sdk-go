package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestOPTSignature_PROBE108(t *testing.T) {
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
		"another node":      replaceOnce(sigOPT, "<rm_attribute_name>category</rm_attribute_name>", "<rm_attribute_name>category</rm_attribute_name><existence/>"),
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

func TestRenamedOPT_RenamesTheTemplateIDOnly_PROBE108(t *testing.T) {
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

func TestCollectTemplates_KeepsTheFirstOfOneID_PROBE108(t *testing.T) {
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
