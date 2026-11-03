package bmmgen

// REQ-041 § SDK-authored root schemas — the note a root declaration carries,
// its reason and upstream reference, documents the root schema, not the Go
// type: the generator leaves it out of every doc comment it emits and keeps
// the rest of the documentation.

import (
	"strings"
	"testing"
)

// TestRender_REQ041_LeavesTheRootNoteOutOfDocComments renders every file of
// both targets and holds each to carrying no root note, and pins that the
// RM root's TRANSLATION_DETAILS keeps the documentation before its note.
func TestRender_REQ041_LeavesTheRootNoteOutOfDocComments(t *testing.T) {
	t.Parallel()
	var resource string
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan, err := BuildPlanForTarget(t.Context(), target, testResolver())
		if err != nil {
			t.Fatalf("BuildPlanForTarget(%s): %v", target.RootID, err)
		}
		for _, f := range plan.Files {
			src, err := RenderFile(plan, f)
			if err != nil {
				t.Fatalf("RenderFile(%s): %v", f.FileBase, err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				if c := strings.TrimSpace(line); strings.HasPrefix(c, "//") && (strings.Contains(c, rootNoteReason) || strings.Contains(c, rootNoteUpstream)) {
					t.Errorf("%s/%s_gen.go:%d carries a root note in its doc comment: %s", target.OutSubDir, f.FileBase, i+1, c)
				}
			}
			if target.GoPackage == TargetRM.GoPackage && f.FileBase == "common_resource" {
				resource = string(src)
			}
		}
	}
	for _, want := range []string{
		"// TranslationDetails Class providing details of a natural language translation.\ntype TranslationDetails struct {\n",
		"\t// Accreditation Accreditation of translator, usually a national translator's registration or association membership id.\n\tAccreditation *string",
		"\t// Language Language of the translation.\n\tLanguage CodePhrase",
	} {
		if !strings.Contains(resource, want) {
			t.Errorf("openehr/rm/common_resource_gen.go lacks %q, the documentation before the root's note", want)
		}
	}
}

// TestCleanDoc_REQ041_LeavesOutTheRootNote pins the rule on the text itself:
// the note runs from its "Reason: " to the end, and only a "Reason: " an
// "Upstream: " follows starts one.
func TestCleanDoc_REQ041_LeavesOutTheRootNote(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ doc, want string }{
		{"An item.\n\nReason: it lacks a title. Upstream: https://example.org/pull/1.", "An item."},
		{"An item. Reason: unchanged. Upstream: test_base_1.0.0.", "An item."},
		{"Reason: unchanged. Upstream: test_base_1.0.0.", ""},
		{"An item.\n\nReason: no upstream reference follows.", "An item.\n\nReason: no upstream reference follows."},
		{"An item.", "An item."},
	} {
		if got := cleanDoc(tc.doc); got != tc.want {
			t.Errorf("cleanDoc(%q) = %q, want %q", tc.doc, got, tc.want)
		}
	}
}
