package composition_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// textLanguageValueChild is a value child for compileAlternatives: a
// DV_TEXT whose language the template requires and constrains to de or nl
// in ISO_639-1.
const textLanguageValueChild = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id>` +
	`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>language</rm_attribute_name>` +
	`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
	`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>` +
	`<children xsi:type="C_CODE_PHRASE"><rm_type_name>CODE_PHRASE</rm_type_name><node_id></node_id>` +
	`<terminology_id><value>ISO_639-1</value></terminology_id><code_list>de</code_list><code_list>nl</code_list></children>` +
	`</attributes></children>`

// TestREQ101_REQ107_TextLanguageBuildsAndSets checks that a COMPOSITION
// whose ELEMENT holds a DV_TEXT with a language the template constrains
// gets a Builder: the skeleton carries the template's first language
// (REQ-107 Leaf values), Set writes another language the template admits
// at the language's own path, Build keeps it, and the composition passes
// ValidateComposition.
func TestREQ101_REQ107_TextLanguageBuildsAndSets(t *testing.T) {
	t.Parallel()
	c := compileAlternatives(t, textLanguageValueChild)
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v, want a Builder", err)
	}
	language := func(comp *rm.Composition) *rm.CodePhrase {
		t.Helper()
		got, err := rmpath.ItemAtPath(comp, abstractValuePath)
		if err != nil {
			t.Fatalf("ItemAtPath(%s): %v", abstractValuePath, err)
		}
		text, ok := got.(*rm.DVText)
		if !ok {
			t.Fatalf("%s holds %T, want *rm.DVText", abstractValuePath, got)
		}
		return text.Language
	}
	skeleton, err := b.Build()
	if err != nil {
		t.Fatalf("Build of the bare skeleton: %v", err)
	}
	de := rm.CodePhrase{CodeString: "de", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	if got := language(skeleton); got == nil || *got != de {
		t.Errorf("skeleton DV_TEXT.language = %+v, want %+v, the template's first code", got, de)
	}

	path := abstractValuePath + "/language"
	nl := &rm.CodePhrase{CodeString: "nl", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	if err := b.Set(path, nl); err != nil {
		t.Fatalf("Set(%s, %+v) = %v, want nil", path, *nl, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set(%s) = %v, want nil", path, err)
	}
	if got := language(comp); got == nil || *got != *nl {
		t.Errorf("built DV_TEXT.language = %+v, want the %+v Set was given", got, *nl)
	}
	if r := validation.ValidateComposition(comp, c); !r.OK {
		t.Errorf("ValidateComposition: %+v, want no issues", r.Issues)
	}
}
