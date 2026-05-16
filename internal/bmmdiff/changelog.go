package bmmdiff

import (
	"fmt"
	"strings"
)

// SuggestChangelogEntry returns a one-line CHANGELOG bullet
// summarising the diff at the highest level. Format:
//
//   - openEHR <model> <old_release> -> <new_release>: <summary>. [bmm-bump]
//
// where <summary> is a comma-joined list of the most salient
// changes (added class names, removed class names, per-class
// property add/remove/change counts). If there are no changes,
// returns an empty string.
//
// The suggestion is intentionally compact — see
// `AGENTS.md § Code style and conventions` for the "short and
// high-level" CHANGELOG bullet rule. The companion runbook
// (docs/adr/0001-...) tells the maintainer to drop the suggestion
// into `## [Unreleased]` under the appropriate sub-section
// (Added/Changed/Removed).
func SuggestChangelogEntry(r *Report) string {
	if r == nil || !r.HasChanges() {
		return ""
	}
	model, oldRel, newRel := splitSchemaIDs(r.OldSchemaID, r.NewSchemaID)
	var parts []string
	if n := len(r.AddedClasses); n > 0 {
		parts = append(parts, addedClassesSummary(r.AddedClasses))
	}
	if n := len(r.RemovedClasses); n > 0 {
		parts = append(parts, removedClassesSummary(r.RemovedClasses))
	}
	for _, cc := range r.ChangedClasses {
		if s := changedClassSummary(cc); s != "" {
			parts = append(parts, s)
		}
	}
	if len(r.AddedPrimitives) > 0 {
		parts = append(parts, fmt.Sprintf("added primitives [%s]", strings.Join(r.AddedPrimitives, ",")))
	}
	if len(r.RemovedPrimitives) > 0 {
		parts = append(parts, fmt.Sprintf("removed primitives [%s]", strings.Join(r.RemovedPrimitives, ",")))
	}
	summary := strings.Join(parts, "; ")
	if summary == "" {
		// No-changes report — should have been caught above, but
		// be defensive.
		return ""
	}
	prefix := model
	if prefix == "" {
		prefix = "openEHR BMM"
	}
	header := prefix
	if oldRel != "" && newRel != "" {
		header = fmt.Sprintf("%s %s -> %s", prefix, oldRel, newRel)
	}
	return fmt.Sprintf("- %s: %s. [bmm-bump]", header, summary)
}

// splitSchemaIDs derives (model, oldRelease, newRelease) from a
// pair of canonical schema ids. A schema id has the shape
// "<publisher>_<schema_name>_<release>" (e.g.
// "openehr_rm_1.2.0"). The "model" is "openEHR <SchemaName-upper>"
// when the publisher equals "openehr"; otherwise the function falls
// back to the raw schema-name.
//
// If the two ids disagree on (publisher, schema_name) the model is
// returned as "openEHR BMM" (no specific upgrade direction); callers
// should not rely on the diff in that case.
func splitSchemaIDs(oldID, newID string) (model, oldRel, newRel string) {
	oldPub, oldName, oldR := parseSchemaID(oldID)
	newPub, newName, newR := parseSchemaID(newID)
	if oldPub == newPub && oldName == newName {
		if oldPub == "openehr" && oldName != "" {
			model = "openEHR " + strings.ToUpper(oldName)
		} else if oldName != "" {
			model = oldName
		}
		return model, oldR, newR
	}
	return "openEHR BMM", "", ""
}

// parseSchemaID decomposes a canonical schema id of the form
// "publisher_name_release" into its three parts. The release is the
// trailing tail starting from the first dotted-version-like token.
func parseSchemaID(id string) (pub, name, release string) {
	if id == "" {
		return "", "", ""
	}
	parts := strings.Split(id, "_")
	if len(parts) == 1 {
		return "", parts[0], ""
	}
	// Walk from the right: collect dotted-version segments into the
	// release; everything before is publisher (first segment) + name.
	relStart := len(parts)
	for i := len(parts) - 1; i >= 0; i-- {
		if isVersionToken(parts[i]) {
			relStart = i
		} else {
			break
		}
	}
	if relStart >= len(parts) {
		// No version tail.
		pub = parts[0]
		name = strings.Join(parts[1:], "_")
		return pub, name, ""
	}
	pub = parts[0]
	if relStart > 1 {
		name = strings.Join(parts[1:relStart], "_")
	}
	release = strings.Join(parts[relStart:], "_")
	return pub, name, release
}

func isVersionToken(s string) bool {
	if s == "" {
		return false
	}
	// A version token contains at least one '.' and only digits/dots.
	hasDot := false
	for _, r := range s {
		switch {
		case r == '.':
			hasDot = true
		case r >= '0' && r <= '9':
			// ok
		default:
			return false
		}
	}
	return hasDot
}

func addedClassesSummary(refs []ClassRef) string {
	if len(refs) <= 3 {
		var names []string
		for _, r := range refs {
			names = append(names, r.ClassName)
		}
		return fmt.Sprintf("adds %d class(es) [%s]", len(refs), strings.Join(names, ","))
	}
	return fmt.Sprintf("adds %d class(es)", len(refs))
}

func removedClassesSummary(refs []ClassRef) string {
	if len(refs) <= 3 {
		var names []string
		for _, r := range refs {
			names = append(names, r.ClassName)
		}
		return fmt.Sprintf("removes %d class(es) [%s]", len(refs), strings.Join(names, ","))
	}
	return fmt.Sprintf("removes %d class(es)", len(refs))
}

func changedClassSummary(cc ClassChange) string {
	var parts []string
	if len(cc.AddedProperties) > 0 {
		var names []string
		for _, p := range cc.AddedProperties {
			names = append(names, fmt.Sprintf("`%s` (%s)", p.Name, p.TypeName))
		}
		// Mention the first few; if longer, fall back to a count.
		if len(names) <= 3 {
			parts = append(parts, fmt.Sprintf("gains optional property %s", strings.Join(names, ", ")))
		} else {
			parts = append(parts, fmt.Sprintf("gains %d properties", len(names)))
		}
	}
	if len(cc.RemovedProperties) > 0 {
		if len(cc.RemovedProperties) <= 3 {
			parts = append(parts, fmt.Sprintf("loses property `%s`", strings.Join(cc.RemovedProperties, "`, `")))
		} else {
			parts = append(parts, fmt.Sprintf("loses %d properties", len(cc.RemovedProperties)))
		}
	}
	if len(cc.ChangedProperties) > 0 {
		parts = append(parts, fmt.Sprintf("changes %d property/properties", len(cc.ChangedProperties)))
	}
	if cc.AncestorsDiffer {
		parts = append(parts, "ancestor chain changed")
	}
	if len(cc.CardinalityChanges) > 0 {
		parts = append(parts, fmt.Sprintf("%d cardinality change(s)", len(cc.CardinalityChanges)))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s %s", cc.ClassName, strings.Join(parts, ", "))
}
