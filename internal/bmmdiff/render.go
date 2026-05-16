package bmmdiff

import (
	"fmt"
	"strings"
)

// Format renders a Report as plain text suitable for terminal output
// or for inclusion in a PR review. Output is deterministic; sections
// with no entries are omitted. When the report has zero changes,
// returns "no semantic changes".
//
// Section headings mirror the format described in the Phase 5
// architecture notes (cmd/bmmdiff). The first line is always a
// schema-id banner so the consumer can confirm what was compared.
func Format(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bmmdiff: %s -> %s\n", banner(r.OldSchemaID), banner(r.NewSchemaID))
	if !r.HasChanges() {
		b.WriteString("no semantic changes\n")
		return b.String()
	}
	if len(r.AddedClasses) > 0 {
		b.WriteString("\nAdded classes:\n")
		for _, c := range r.AddedClasses {
			if c.Package == "" {
				fmt.Fprintf(&b, "  - %s\n", c.ClassName)
			} else {
				fmt.Fprintf(&b, "  - %s  (package: %s)\n", c.ClassName, c.Package)
			}
		}
	}
	if len(r.RemovedClasses) > 0 {
		b.WriteString("\nRemoved classes:\n")
		for _, c := range r.RemovedClasses {
			if c.Package == "" {
				fmt.Fprintf(&b, "  - %s\n", c.ClassName)
			} else {
				fmt.Fprintf(&b, "  - %s  (was in package: %s)\n", c.ClassName, c.Package)
			}
		}
	}
	if len(r.ChangedClasses) > 0 {
		b.WriteString("\nChanged classes:\n")
		for _, c := range r.ChangedClasses {
			renderClassChange(&b, c)
		}
	}
	if len(r.AddedPrimitives) > 0 || len(r.RemovedPrimitives) > 0 {
		b.WriteString("\nPrimitives:\n")
		if len(r.AddedPrimitives) > 0 {
			fmt.Fprintf(&b, "  added:   [%s]\n", strings.Join(r.AddedPrimitives, ", "))
		}
		if len(r.RemovedPrimitives) > 0 {
			fmt.Fprintf(&b, "  removed: [%s]\n", strings.Join(r.RemovedPrimitives, ", "))
		}
	}
	return b.String()
}

func banner(id string) string {
	if id == "" {
		return "(unnamed)"
	}
	return id
}

func renderClassChange(b *strings.Builder, c ClassChange) {
	fmt.Fprintf(b, "  %s:\n", c.ClassName)
	if c.AncestorsDiffer {
		fmt.Fprintf(b, "    ancestors: [%s] -> [%s]\n",
			strings.Join(c.OldAncestors, ", "), strings.Join(c.NewAncestors, ", "))
	}
	if len(c.AddedProperties) > 0 {
		var parts []string
		for _, p := range c.AddedProperties {
			parts = append(parts, fmt.Sprintf("%s: %s", p.Name, p.TypeName))
		}
		fmt.Fprintf(b, "    added properties:   [%s]\n", strings.Join(parts, ", "))
	}
	if len(c.RemovedProperties) > 0 {
		fmt.Fprintf(b, "    removed properties: [%s]\n", strings.Join(c.RemovedProperties, ", "))
	}
	if len(c.ChangedProperties) > 0 {
		b.WriteString("    changed properties:\n")
		for _, pc := range c.ChangedProperties {
			fmt.Fprintf(b, "      - %s: %s\n", pc.Name, summarisePropertyChange(pc))
		}
	}
	if len(c.AddedFunctions) > 0 {
		fmt.Fprintf(b, "    added functions:    [%s]\n", strings.Join(c.AddedFunctions, ", "))
	}
	if len(c.RemovedFunctions) > 0 {
		fmt.Fprintf(b, "    removed functions:  [%s]\n", strings.Join(c.RemovedFunctions, ", "))
	}
	if len(c.CardinalityChanges) > 0 {
		b.WriteString("    cardinality changes:\n")
		for _, cc := range c.CardinalityChanges {
			fmt.Fprintf(b, "      - %s: {lower:%d,upper:%s} -> {lower:%d,upper:%s}\n",
				cc.Name, cc.OldLower, cc.OldUpper, cc.NewLower, cc.NewUpper)
		}
	}
}

// summarisePropertyChange builds the "type T1 -> T2 (mandatory:
// false -> true)" suffix.
func summarisePropertyChange(pc PropertyChange) string {
	var parts []string
	if pc.TypeDiff {
		parts = append(parts, fmt.Sprintf("Type %s -> %s", pc.OldType, pc.NewType))
	}
	if pc.KindDiff {
		parts = append(parts, fmt.Sprintf("kind %s -> %s", shortKind(pc.OldPropertyKey), shortKind(pc.NewPropertyKey)))
	}
	if pc.MandatoryDiff {
		parts = append(parts, fmt.Sprintf("mandatory: %t -> %t", pc.OldMandatory, pc.NewMandatory))
	}
	if len(parts) == 0 {
		return "(no salient change)"
	}
	return strings.Join(parts, "  ")
}

// shortKind trims the P_BMM_ prefix so output stays readable.
func shortKind(k string) string {
	return strings.TrimPrefix(k, "P_BMM_")
}
