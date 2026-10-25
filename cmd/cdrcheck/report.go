package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// runData is everything one run recorded: the lines of each stage's JSON
// Lines file.
type runData struct {
	cdrs      []string
	templates []templateRecord
	uploads   []uploadRecord
	gens      []genRecord
	ehrs      []ehrRecord
	posts     []postRecord
	examples  []exampleRecord
	validates []validateRecord
	cross     []postRecord
}

// uploadRecord is one OPT sent to one CDR, one line of upload.jsonl.
type uploadRecord struct {
	CDR         string `json:"cdr"`
	TemplateID  string `json:"template_id"`
	File        string `json:"file"`
	RenamedFrom string `json:"renamed_from,omitempty"`
	outcome
	// StoredID is the id the CDR answered with, where it differs from the
	// OPT's own (a CDR may normalise the spaces in an id).
	StoredID string `json:"stored_id,omitempty"`
	// StoredStatus and StoredSame describe the copy a CDR already holds
	// under the id, when the upload was refused for that reason: the status
	// of fetching it, and whether its signature (scanOPT) equals the OPT's.
	StoredStatus int   `json:"stored_status,omitempty"`
	StoredSame   *bool `json:"stored_same,omitempty"`
	// Present is true when the CDR holds this template after the upload, so
	// documents of it are posted there.
	Present bool `json:"present"`
	// SupersededBy names the id the template was uploaded again under,
	// because a CDR holds other content under this one (uploadTemplate). The
	// record is kept, as what this CDR answered, and left out of the counts.
	SupersededBy string `json:"superseded_by,omitempty"`
}

// counted reports whether the counts take in u: not a record superseded by
// a rename, nor one whose request the end of the run cut short.
func (u uploadRecord) counted() bool { return u.SupersededBy == "" && !u.Stopped }

// written is the record as upload.jsonl holds it.
func (u uploadRecord) written() uploadRecord {
	u.outcome = u.outcome.written()
	return u
}

// ehrRecord is one EHR the run asked a CDR to create, one line of
// ehrs.jsonl, written as the create returns. EHRID is the id the run named
// (runIDs), so a create cut short by the end of the run is recorded with it
// too.
type ehrRecord struct {
	CDR   string `json:"cdr"`
	EHRID string `json:"ehr_id"`
	// TemplateID, Label and Source name the document the EHR is for, as its
	// postRecord does.
	TemplateID string `json:"template_id"`
	Label      string `json:"label"`
	Source     string `json:"source,omitempty"`
	outcome
}

// written is the record as ehrs.jsonl holds it.
func (e ehrRecord) written() ehrRecord {
	e.outcome = e.outcome.written()
	return e
}

// postRecord is one document posted to one CDR, a line of post.jsonl (an
// SDK document) or of cross.jsonl (a CDR's example).
type postRecord struct {
	CDR        string `json:"cdr"`
	TemplateID string `json:"template_id"`
	// Label is <policy>-<fill> for an SDK document and <source>:<level> for
	// a CDR example.
	Label  string `json:"label"`
	Source string `json:"source,omitempty"`
	File   string `json:"file"`
	EHRID  string `json:"ehr_id,omitempty"`
	// Stage is "ehr" when the run could not create the EHR the document
	// was to go to, the outcome then being that of the EHR create, and
	// "read" when it could not read the document.
	Stage string `json:"stage,omitempty"`
	outcome
}

// posted reports whether the document reached the CDR and the CDR's
// verdict on it is known.
func (p postRecord) posted() bool { return p.Stage == "" && !p.Stopped }

// written is the record as post.jsonl or cross.jsonl holds it.
func (p postRecord) written() postRecord {
	p.outcome = p.outcome.written()
	return p
}

// exampleRecord is one CDR example asked for, one line of examples.jsonl.
type exampleRecord struct {
	CDR        string `json:"cdr"`
	TemplateID string `json:"template_id"`
	Level      string `json:"level"`
	File       string `json:"file,omitempty"`
	RootType   string `json:"root_type,omitempty"`
	outcome
}

// written is the record as examples.jsonl holds it.
func (e exampleRecord) written() exampleRecord {
	e.outcome = e.outcome.written()
	return e
}

// ratio is n of a total.
type ratio struct {
	N  int `json:"n"`
	Of int `json:"of"`
}

func (r ratio) String() string { return fmt.Sprintf("%d / %d", r.N, r.Of) }

// refusalGroup is the documents a CDR refused with one status and one detail
// line.
type refusalGroup struct {
	Status    int            `json:"status"`
	Detail    string         `json:"detail"`
	Documents int            `json:"documents"`
	Templates int            `json:"templates"`
	ByPolicy  map[string]int `json:"by_policy,omitempty"`
	// Examples are up to five of the templates, by id.
	Examples []string `json:"examples"`
}

// cdrSummary is one CDR's numbers.
type cdrSummary struct {
	Name             string         `json:"name"`
	TemplatesPresent ratio          `json:"templates_present"`
	UploadRefusals   []refusalGroup `json:"upload_refusals,omitempty"`
	StoredDiffers    []string       `json:"stored_differs,omitempty"`
	// EHRsCreated counts the EHR creates the CDR accepted, of those whose
	// answer came; EHRRefusals groups the others.
	EHRsCreated           ratio          `json:"ehrs_created"`
	EHRRefusals           []refusalGroup `json:"ehr_refusals,omitempty"`
	DocumentsAccepted     ratio          `json:"documents_accepted"`
	DocumentRefusals      []refusalGroup `json:"document_refusals,omitempty"`
	EHRFailures           int            `json:"ehr_failures"`
	ExamplesFetched       ratio          `json:"examples_fetched"`
	ExampleStatuses       map[string]int `json:"example_statuses,omitempty"`
	ExamplesSDKValid      ratio          `json:"examples_sdk_valid"`
	OwnExamplesAccepted   ratio          `json:"own_examples_accepted"`
	OtherExamplesAccepted ratio          `json:"other_examples_accepted"`
	ExampleRefusals       []refusalGroup `json:"example_refusals,omitempty"`
}

// refusedDocs is one template's SDK-valid documents that a CDR refused.
type refusedDocs struct {
	TemplateID string              `json:"template_id"`
	Labels     []string            `json:"labels"`
	Details    map[string][]string `json:"details"`
}

// summary is the run's numbers.
type summary struct {
	CDRs []string `json:"cdrs"`
	// Only is the -only expression of a run that checked only the
	// templates it matches.
	Only string `json:"only,omitempty"`
	// Partial says why a run stopped before it finished; its numbers then
	// count the records made until it stopped.
	Partial string `json:"partial,omitempty"`
	// Templates counts the OPT files read, the templates checked (a renamed
	// one under its new id), and the files left out: a second file with an
	// id already read, a file with no id, or one -only does not match.
	Templates struct {
		Files   int            `json:"files"`
		Kept    int            `json:"kept"`
		Skipped int            `json:"skipped"`
		Renamed []string       `json:"renamed,omitempty"`
		Roots   map[string]int `json:"roots"`
	} `json:"templates"`
	Generation struct {
		Runs                  int      `json:"runs"`
		Errors                int      `json:"errors"`
		ErrorTemplates        []string `json:"error_templates,omitempty"`
		NotSelfValid          int      `json:"not_self_valid"`
		NotSelfValidTemplates []string `json:"not_self_valid_templates,omitempty"`
		NotRMValid            int      `json:"not_rm_valid"`
		NotRMValidTemplates   []string `json:"not_rm_valid_templates,omitempty"`
	} `json:"generation"`
	PerCDR []cdrSummary `json:"per_cdr,omitempty"`
	// SDKValidRefused counts the SDK documents the SDK itself accepts
	// (self_ok) that CDRs refused: by every CDR, and by one CDR alone
	// while every other CDR accepted it.
	SDKValidRefused struct {
		ByAll int            `json:"by_all"`
		Alone map[string]int `json:"alone"`
	} `json:"sdk_valid_refused"`
	// GeneratedMatrix counts the SDK's COMPOSITION documents by verdict:
	// "self-ok|<cdr>-ok|<cdr>-bad|<cdr>-na" and so on, na where a document
	// was not posted.
	GeneratedMatrix map[string]int `json:"generated_matrix,omitempty"`
	// ExampleMatrix counts the CDRs' examples by verdict:
	// "<source>|sdk-ok|<cdr>-ok|…".
	ExampleMatrix map[string]int `json:"example_matrix,omitempty"`
	// RefusedSDKValid lists, by template, the SDK-valid documents a CDR
	// refused, with each refusing CDR's first detail lines.
	RefusedSDKValid []refusedDocs `json:"refused_sdk_valid,omitempty"`
}

// verdict is one document's outcome at one CDR, for grouping refusals.
type verdict struct {
	tid, label string
	o          outcome
}

var (
	uidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(::[^"\s,]*)?`)
	spaceRun   = regexp.MustCompile(`\s+`)
	// joinedPath finds where EHRbase joins two findings in one message:
	// ", " before the next finding's path.
	joinedPath = regexp.MustCompile(`, /[a-z_]+[\[/:]`)
)

// normalise makes a refusal line comparable across documents: uids become
// <uid> and whitespace runs one space.
func normalise(s string) string {
	s = uidPattern.ReplaceAllString(s, "<uid>")
	return strings.TrimSpace(spaceRun.ReplaceAllString(s, " "))
}

// refusalDetails splits a refusal into its finding lines. A JSON error body
// with validationErrors (FerroEHR) gives one line per entry; one with a
// message (EHRbase) gives a line per finding the message joins. Anything
// else is one line.
func refusalDetails(o outcome) []string {
	text := cmp.Or(o.Body, o.Error)
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return []string{cut(normalise(text), 300)}
	}
	var out []string
	if ve, ok := body["validationErrors"].([]any); ok && len(ve) > 0 {
		for _, v := range ve {
			out = append(out, normalise(textOf(v)))
		}
		return uniq(out)
	}
	msg := text
	if m, ok := body["message"]; ok && m != nil {
		msg = textOf(m)
	} else if e, ok := body["error"]; ok && e != nil {
		msg = textOf(e)
	}
	start := 0
	for _, loc := range joinedPath.FindAllStringIndex(msg, -1) {
		out = append(out, normalise(msg[start:loc[0]]))
		start = loc[0] + len(", ")
	}
	out = append(out, normalise(msg[start:]))
	return uniq(out)
}

func textOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	out := s[:0]
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// groupRefusals groups the refused verdicts by status and detail line, the
// widest group (most templates, then most documents) first.
func groupRefusals(vs []verdict) []refusalGroup {
	type key struct {
		status int
		detail string
	}
	type acc struct {
		docs, tids map[string]bool
		byPolicy   map[string]int
	}
	groups := map[key]*acc{}
	for _, v := range vs {
		if v.o.OK {
			continue
		}
		for _, d := range refusalDetails(v.o) {
			k := key{v.o.Status, d}
			a := groups[k]
			if a == nil {
				a = &acc{docs: map[string]bool{}, tids: map[string]bool{}, byPolicy: map[string]int{}}
				groups[k] = a
			}
			a.docs[v.tid+"\x00"+v.label] = true
			a.tids[v.tid] = true
			if policy, _, ok := strings.Cut(v.label, "-"); ok {
				a.byPolicy[policy]++
			}
		}
	}
	out := make([]refusalGroup, 0, len(groups))
	for k, a := range groups {
		tids := slices.Sorted(maps.Keys(a.tids))
		g := refusalGroup{Status: k.status, Detail: k.detail, Documents: len(a.docs), Templates: len(tids), Examples: tids[:min(5, len(tids))]}
		if len(a.byPolicy) > 0 {
			g.ByPolicy = a.byPolicy
		}
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b refusalGroup) int {
		return cmp.Or(cmp.Compare(b.Templates, a.Templates), cmp.Compare(b.Documents, a.Documents),
			cmp.Compare(a.Status, b.Status), strings.Compare(a.Detail, b.Detail))
	})
	return out
}

// docKey names a document across stages: its template and label.
type docKey struct{ tid, label string }

// summarize computes the run's numbers from its records.
func summarize(r runData) summary {
	var s summary
	s.CDRs = r.cdrs
	s.Templates.Roots = map[string]int{}
	for _, t := range r.templates {
		if t.RenamedFrom == "" {
			s.Templates.Files++
		}
		if !t.Kept {
			if t.RenamedTo == "" {
				s.Templates.Skipped++
			}
			continue
		}
		s.Templates.Kept++
		s.Templates.Roots[cmp.Or(t.Root, "(none)")]++
		if t.RenamedFrom != "" {
			s.Templates.Renamed = append(s.Templates.Renamed, t.TemplateID)
		}
	}

	errT, selfT, rmT := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, g := range r.gens {
		s.Generation.Runs++
		switch {
		case g.Error != "":
			s.Generation.Errors++
			errT[g.TemplateID] = true
		default:
			if g.SelfOK != nil && !*g.SelfOK {
				s.Generation.NotSelfValid++
				selfT[g.TemplateID] = true
			}
			if g.RMOK != nil && !*g.RMOK {
				s.Generation.NotRMValid++
				rmT[g.TemplateID] = true
			}
		}
	}
	s.Generation.ErrorTemplates = slices.Sorted(maps.Keys(errT))
	s.Generation.NotSelfValidTemplates = slices.Sorted(maps.Keys(selfT))
	s.Generation.NotRMValidTemplates = slices.Sorted(maps.Keys(rmT))

	if len(r.cdrs) == 0 {
		return s
	}

	// Per CDR.
	for _, cdr := range r.cdrs {
		cs := cdrSummary{Name: cdr}
		var up []verdict
		for _, u := range r.uploads {
			if u.CDR != cdr {
				continue
			}
			if u.StoredSame != nil && !*u.StoredSame {
				cs.StoredDiffers = append(cs.StoredDiffers, u.TemplateID)
			}
			if !u.counted() {
				continue
			}
			cs.TemplatesPresent.Of++
			if u.Present {
				cs.TemplatesPresent.N++
			} else {
				up = append(up, verdict{tid: u.TemplateID, o: u.outcome})
			}
		}
		cs.UploadRefusals = groupRefusals(up)
		var ehrs []verdict
		for _, e := range r.ehrs {
			if e.CDR != cdr || e.Stopped {
				continue
			}
			cs.EHRsCreated.Of++
			if e.OK {
				cs.EHRsCreated.N++
			}
			ehrs = append(ehrs, verdict{tid: e.TemplateID, label: e.Label, o: e.outcome})
		}
		cs.EHRRefusals = groupRefusals(ehrs)
		var docs []verdict
		for _, p := range r.posts {
			if p.CDR != cdr || p.Stopped {
				continue
			}
			if p.Stage == "ehr" {
				cs.EHRFailures++
			}
			if !p.posted() {
				continue
			}
			cs.DocumentsAccepted.Of++
			if p.OK {
				cs.DocumentsAccepted.N++
			}
			docs = append(docs, verdict{tid: p.TemplateID, label: p.Label, o: p.outcome})
		}
		cs.DocumentRefusals = groupRefusals(docs)
		for _, e := range r.examples {
			if e.CDR != cdr || e.Stopped {
				continue
			}
			cs.ExamplesFetched.Of++
			if e.File != "" {
				cs.ExamplesFetched.N++
			} else {
				if cs.ExampleStatuses == nil {
					cs.ExampleStatuses = map[string]int{}
				}
				cs.ExampleStatuses[strconv.Itoa(e.Status)]++
			}
		}
		for _, v := range r.validates {
			if v.CDR != cdr {
				continue
			}
			cs.ExamplesSDKValid.Of++
			if v.OK != nil && *v.OK {
				cs.ExamplesSDKValid.N++
			}
		}
		var ex []verdict
		for _, c := range r.cross {
			if c.CDR != cdr || c.Stopped {
				continue
			}
			if c.Stage == "ehr" {
				cs.EHRFailures++
			}
			if !c.posted() {
				continue
			}
			share := &cs.OtherExamplesAccepted
			if c.Source == cdr {
				share = &cs.OwnExamplesAccepted
			}
			share.Of++
			if c.OK {
				share.N++
			}
			ex = append(ex, verdict{tid: c.TemplateID, label: c.Label, o: c.outcome})
		}
		cs.ExampleRefusals = groupRefusals(ex)
		s.PerCDR = append(s.PerCDR, cs)
	}

	// The SDK's own documents, by verdict.
	posted := map[docKey]map[string]postRecord{}
	for _, p := range r.posts {
		if !p.posted() {
			continue
		}
		k := docKey{p.TemplateID, p.Label}
		if posted[k] == nil {
			posted[k] = map[string]postRecord{}
		}
		posted[k][p.CDR] = p
	}
	s.SDKValidRefused.Alone = map[string]int{}
	s.GeneratedMatrix = map[string]int{}
	refusedBy := map[string]*refusedDocs{}
	for _, g := range r.gens {
		if g.Error != "" || g.RootType != "COMPOSITION" {
			continue
		}
		k := docKey{g.TemplateID, g.label()}
		selfOK := g.SelfOK != nil && *g.SelfOK
		parts := []string{"self-bad"}
		if selfOK {
			parts[0] = "self-ok"
		}
		var refused, accepted []string
		for _, cdr := range r.cdrs {
			p, ok := posted[k][cdr]
			switch {
			case !ok:
				parts = append(parts, cdr+"-na")
			case p.OK:
				parts = append(parts, cdr+"-ok")
				accepted = append(accepted, cdr)
			default:
				parts = append(parts, cdr+"-bad")
				refused = append(refused, cdr)
			}
		}
		s.GeneratedMatrix[strings.Join(parts, "|")]++
		if !selfOK || len(refused) == 0 {
			continue
		}
		switch {
		case len(refused) == len(r.cdrs):
			s.SDKValidRefused.ByAll++
		case len(refused) == 1 && len(accepted) == len(r.cdrs)-1:
			s.SDKValidRefused.Alone[refused[0]]++
		}
		rd := refusedBy[g.TemplateID]
		if rd == nil {
			rd = &refusedDocs{TemplateID: g.TemplateID, Details: map[string][]string{}}
			refusedBy[g.TemplateID] = rd
		}
		rd.Labels = append(rd.Labels, g.label())
		for _, cdr := range refused {
			if _, done := rd.Details[cdr]; !done {
				d := refusalDetails(posted[k][cdr].outcome)
				rd.Details[cdr] = d[:min(3, len(d))]
			}
		}
	}
	for _, tid := range slices.Sorted(maps.Keys(refusedBy)) {
		rd := refusedBy[tid]
		slices.Sort(rd.Labels)
		s.RefusedSDKValid = append(s.RefusedSDKValid, *rd)
	}

	// The CDRs' examples, by verdict.
	type exKey struct{ src, tid, level string }
	crossBy := map[exKey]map[string]bool{}
	for _, c := range r.cross {
		if !c.posted() {
			continue
		}
		_, level, _ := strings.Cut(c.Label, ":")
		k := exKey{c.Source, c.TemplateID, level}
		if crossBy[k] == nil {
			crossBy[k] = map[string]bool{}
		}
		crossBy[k][c.CDR] = c.OK
	}
	s.ExampleMatrix = map[string]int{}
	for _, v := range r.validates {
		sdk := "sdk-bad"
		switch {
		case v.Error != "":
			sdk = "sdk-error"
		case v.OK != nil && *v.OK:
			sdk = "sdk-ok"
		}
		parts := []string{v.CDR, sdk}
		got := crossBy[exKey{v.CDR, v.TemplateID, v.Level}]
		for _, cdr := range r.cdrs {
			ok, posted := got[cdr]
			switch {
			case !posted:
				parts = append(parts, cdr+"-na")
			case ok:
				parts = append(parts, cdr+"-ok")
			default:
				parts = append(parts, cdr+"-bad")
			}
		}
		s.ExampleMatrix[strings.Join(parts, "|")]++
	}
	return s
}
