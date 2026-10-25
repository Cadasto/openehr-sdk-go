package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// maxGroups is how many refusal groups summary.md lists per CDR and stage;
// summary.json holds them all.
const maxGroups = 25

// runInfo is the run's provenance, written to run.json and heading
// summary.md.
type runInfo struct {
	Started   string   `json:"started"`
	Finished  string   `json:"finished"`
	SDKCommit string   `json:"sdk_commit,omitempty"`
	Templates string   `json:"templates"`
	Only      string   `json:"only,omitempty"`
	CDRs      []cdrRef `json:"cdrs,omitempty"`
	CDRStages string   `json:"cdr_stages"`
	Policies  []string `json:"policies"`
	Fills     []string `json:"fills"`
	Now       string   `json:"now"`
}

// cdrRef names a CDR and its base URL, which carries no credential.
type cdrRef struct {
	Name string `json:"name"`
	Base string `json:"base"`
}

// markdown renders the summary as summary.md: the tables the evidence
// README carries, then the refusals grouped.
func markdown(info runInfo, s summary) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w("# CDR cross-check\n\n")
	w("- **Run:** %s to %s, SDK `%s`.\n", info.Started, info.Finished, cmp.Or(info.SDKCommit, "unknown"))
	w("- **Templates:** %d OPT files in `%s`, %d templates checked, %d files left out (an id read already, no id, or not matched by -only).\n",
		s.Templates.Files, info.Templates, s.Templates.Kept, s.Templates.Skipped)
	if info.Only != "" {
		w("- **Only:** template ids matching `%s`.\n", info.Only)
	}
	if len(s.Templates.Renamed) > 0 {
		w("- **Renamed:** uploaded under a new id because a CDR holds other content under the original: %s.\n", codeList(s.Templates.Renamed))
	}
	w("- **CDR stages:** %s.\n", info.CDRStages)
	w("- **Generation:** policies %s × fills %s, clock %s.\n\n", strings.Join(info.Policies, ", "), strings.Join(info.Fills, ", "), info.Now)

	w("## Generation\n\n")
	w("- %d runs; %d failed in %d templates%s.\n", s.Generation.Runs, s.Generation.Errors, len(s.Generation.ErrorTemplates), listSuffix(s.Generation.ErrorTemplates))
	w("- %d outputs fail the SDK template validator, in %d templates%s.\n", s.Generation.NotSelfValid, len(s.Generation.NotSelfValidTemplates), listSuffix(s.Generation.NotSelfValidTemplates))
	w("- %d outputs fail the RM floor, in %d templates%s.\n\n", s.Generation.NotRMValid, len(s.Generation.NotRMValidTemplates), listSuffix(s.Generation.NotRMValidTemplates))
	roots := slices.Sorted(maps.Keys(s.Templates.Roots))
	parts := make([]string, 0, len(roots))
	for _, r := range roots {
		parts = append(parts, fmt.Sprintf("%s %d", r, s.Templates.Roots[r]))
	}
	w("Template roots: %s.\n\n", strings.Join(parts, ", "))

	if len(s.PerCDR) == 0 {
		return b.String()
	}

	w("## Results\n\n| Measure |")
	for _, c := range s.PerCDR {
		w(" %s |", c.Name)
	}
	w("\n|---|")
	for range s.PerCDR {
		w("---|")
	}
	w("\n")
	row := func(label string, get func(cdrSummary) ratio) {
		w("| %s |", label)
		for _, c := range s.PerCDR {
			w(" %s |", get(c))
		}
		w("\n")
	}
	row("Templates accepted at upload", func(c cdrSummary) ratio { return c.TemplatesPresent })
	row("SDK documents accepted", func(c cdrSummary) ratio { return c.DocumentsAccepted })
	row("CDR's own examples fetched", func(c cdrSummary) ratio { return c.ExamplesFetched })
	row("SDK validator: CDR examples valid", func(c cdrSummary) ratio { return c.ExamplesSDKValid })
	row("CDR accepts its own examples", func(c cdrSummary) ratio { return c.OwnExamplesAccepted })
	row("CDR accepts the other CDRs' examples", func(c cdrSummary) ratio { return c.OtherExamplesAccepted })
	w("\nOf the SDK documents the SDK itself accepts:\n\n")
	w("- %d were refused by every CDR;\n", s.SDKValidRefused.ByAll)
	for _, c := range s.PerCDR {
		w("- %d by %s alone (every other CDR accepted them);\n", s.SDKValidRefused.Alone[c.Name], c.Name)
	}
	w("\nThe other cases are in the verdict tables below.\n\n")

	w("## SDK documents by verdict\n\n| Verdicts | Documents |\n|---|---|\n")
	for _, k := range slices.Sorted(maps.Keys(s.GeneratedMatrix)) {
		w("| %s | %d |\n", strings.ReplaceAll(k, "|", " · "), s.GeneratedMatrix[k])
	}
	w("\n## CDR examples by verdict\n\nThe example's CDR, the SDK validator's verdict, then each CDR's verdict on posting it.\n\n| Verdicts | Documents |\n|---|---|\n")
	for _, k := range slices.Sorted(maps.Keys(s.ExampleMatrix)) {
		w("| %s | %d |\n", strings.ReplaceAll(k, "|", " · "), s.ExampleMatrix[k])
	}

	for _, c := range s.PerCDR {
		w("\n## %s\n\n", c.Name)
		if len(c.StoredDiffers) > 0 {
			w("- Holds other content under: %s.\n", codeList(c.StoredDiffers))
		}
		if c.EHRFailures > 0 {
			w("- %d documents not posted: the EHR create failed.\n", c.EHRFailures)
		}
		if len(c.ExampleStatuses) > 0 {
			var st []string
			for _, k := range slices.Sorted(maps.Keys(c.ExampleStatuses)) {
				st = append(st, fmt.Sprintf("%s × %d", k, c.ExampleStatuses[k]))
			}
			w("- Examples not fetched, by status: %s.\n", strings.Join(st, ", "))
		}
		groupTable(&b, "Template upload refusals", c.UploadRefusals)
		groupTable(&b, "SDK document refusals", c.DocumentRefusals)
		groupTable(&b, "CDR example refusals", c.ExampleRefusals)
	}

	if len(s.RefusedSDKValid) > 0 {
		w("\n## SDK-valid documents refused, by template\n\n")
		for _, r := range s.RefusedSDKValid {
			w("- `%s`: %s\n", r.TemplateID, strings.Join(r.Labels, ", "))
			for _, cdr := range slices.Sorted(maps.Keys(r.Details)) {
				for _, d := range r.Details[cdr] {
					w("  - %s: %s\n", cdr, cell(d))
				}
			}
		}
	}
	return b.String()
}

func groupTable(b *strings.Builder, title string, gs []refusalGroup) {
	if len(gs) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n| Status | Documents | Templates | By policy | Detail | Templates, first five |\n|---|---|---|---|---|---|\n", title)
	for _, g := range gs[:min(maxGroups, len(gs))] {
		var pol []string
		for _, k := range slices.Sorted(maps.Keys(g.ByPolicy)) {
			pol = append(pol, fmt.Sprintf("%s %d", k, g.ByPolicy[k]))
		}
		fmt.Fprintf(b, "| %d | %d | %d | %s | %s | %s |\n", g.Status, g.Documents, g.Templates, strings.Join(pol, ", "), cell(g.Detail), cell(strings.Join(g.Examples, "; ")))
	}
	if len(gs) > maxGroups {
		fmt.Fprintf(b, "\n%d more groups in summary.json.\n", len(gs)-maxGroups)
	}
}

// cell makes s safe inside a Markdown table cell.
func cell(s string) string {
	return strings.ReplaceAll(cut(strings.Join(strings.Fields(s), " "), 300), "|", `\|`)
}

func codeList(ids []string) string {
	q := make([]string, 0, len(ids))
	for _, id := range ids {
		q = append(q, "`"+id+"`")
	}
	return strings.Join(q, ", ")
}

func listSuffix(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ": " + codeList(ids)
}
