package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
)

// renameSuffix is appended to a template id whose CDR copy holds other
// content. The OPT is uploaded again under the new id, to every CDR, and
// generated from under it, so both CDRs and the SDK read the same content.
const renameSuffix = ".xc1"

// forEach calls fn for 0..n-1 on at most workers goroutines at once, and
// returns when every call has. It stops starting calls once ctx ends, and
// then returns the context's cause.
func forEach(ctx context.Context, workers, n int, fn func(ctx context.Context, i int)) error {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range n {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return context.Cause(ctx)
		}
		wg.Go(func() {
			defer func() { <-sem }()
			fn(ctx, i)
		})
	}
	wg.Wait()
	return context.Cause(ctx)
}

// execute runs the cross-check cfg describes and writes its output.
func execute(ctx context.Context, cfg config, stdout io.Writer) error {
	if err := prepareOut(cfg.out); err != nil {
		return err
	}
	s := &sink{dir: cfg.out, red: newRedactor(cfg.cdrs), log: stdout}
	info := runInfo{
		Started:   time.Now().UTC().Format(time.RFC3339),
		SDKCommit: cfg.sdkCommit,
		Templates: cfg.templates,
		CDRStages: "ran",
		Now:       generationNow.Format(time.RFC3339),
	}
	if cfg.only != nil {
		info.Only = cfg.only.String()
	}
	for _, p := range policies {
		info.Policies = append(info.Policies, p.String())
	}
	for _, f := range fills {
		info.Fills = append(info.Fills, f.name)
	}
	if cfg.cdrSkip != "" {
		info.CDRStages = "skipped: " + cfg.cdrSkip
		s.logf("cdrcheck: CDR stages skipped: %s", cfg.cdrSkip)
	}
	s.logf("cdrcheck: output in %s", cfg.out)

	var data runData
	var clients []*cdrClient
	for _, t := range cfg.cdrs {
		c, err := newCDRClient(t, cfg.workers)
		if err != nil {
			return err
		}
		clients = append(clients, c)
		data.cdrs = append(data.cdrs, t.name)
		info.CDRs = append(info.CDRs, cdrRef{Name: t.name, Base: t.base})
	}

	all, err := collectTemplates(cfg.templates)
	if err != nil {
		return err
	}
	var tpls []templateRecord
	for i, t := range all {
		if t.Kept && cfg.only != nil && !cfg.only.MatchString(t.TemplateID) {
			all[i].Kept, all[i].Skipped = false, "not matched by -only"
		}
		if all[i].Kept {
			tpls = append(tpls, all[i])
		}
	}
	s.logf("collect: %d OPT files, %d templates checked", len(all), len(tpls))

	present := map[string]map[string]bool{}
	if len(clients) > 0 {
		var renamed []templateRecord
		data.uploads, tpls, renamed, err = uploadStage(ctx, s, clients, tpls, cfg.workers)
		if err != nil {
			return err
		}
		for _, nt := range renamed {
			for i := range all {
				if all[i].Kept && all[i].TemplateID == nt.RenamedFrom {
					all[i].Kept, all[i].RenamedTo = false, nt.TemplateID
				}
			}
		}
		all = append(all, renamed...)
		for _, u := range data.uploads {
			if present[u.CDR] == nil {
				present[u.CDR] = map[string]bool{}
			}
			present[u.CDR][u.TemplateID] = u.Present
		}
		for _, c := range clients {
			n := 0
			for _, ok := range present[c.name] {
				if ok {
					n++
				}
			}
			s.logf("upload %s: %d of %d templates present", c.name, n, len(tpls))
		}
	}
	data.templates = all

	cc := &compiler{}
	data.gens, data.posts, err = generateStage(ctx, s, cc, clients, present, tpls, cfg.workers)
	if err != nil {
		return err
	}
	s.logf("generate: %d runs", len(data.gens))

	if len(clients) > 0 {
		files := map[string]string{}
		for _, t := range tpls {
			files[t.TemplateID] = t.File
		}
		data.examples, err = exampleStage(ctx, s, clients, present, tpls, cfg.workers)
		if err != nil {
			return err
		}
		data.validates, err = validateStage(ctx, s, cc, files, data.examples, cfg.workers)
		if err != nil {
			return err
		}
		data.cross, err = crossStage(ctx, s, clients, present, data.examples, cfg.workers)
		if err != nil {
			return err
		}
	}

	sum := summarize(data)
	info.Finished = time.Now().UTC().Format(time.RFC3339)
	if err := writeAll(s, info, data, sum); err != nil {
		return err
	}
	logSummary(s, sum)
	return nil
}

// prepareOut creates the output directory. It refuses one that already
// holds files, so that two runs never mix.
func prepareOut(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("output directory: %w", err)
	case len(entries) > 0:
		return fmt.Errorf("output directory %s is not empty; name a new one with -out or %s", dir, envOut)
	}
	return os.MkdirAll(dir, 0o755)
}

// uploadStage uploads every template to every CDR. A template some CDR
// holds other content under is renamed (renameSuffix) and uploaded again,
// to every CDR, in place of the original; renamed lists the new records.
func uploadStage(ctx context.Context, s *sink, clients []*cdrClient, tpls []templateRecord, workers int) (recs []uploadRecord, out, renamed []templateRecord, err error) {
	recs, err = uploadAll(ctx, clients, tpls, workers)
	if err != nil {
		return nil, nil, nil, err
	}
	stale := map[string]bool{}
	for _, r := range recs {
		if r.StoredSame != nil && !*r.StoredSame {
			stale[r.TemplateID] = true
		}
	}
	if len(stale) == 0 {
		return recs, tpls, nil, nil
	}
	out = slices.Clone(tpls)
	for i, t := range out {
		if !stale[t.TemplateID] {
			continue
		}
		raw, err := os.ReadFile(t.File)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read OPT: %w", err)
		}
		newID := t.TemplateID + renameSuffix
		b, err := renamedOPT(raw, newID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("rename %s: %w", t.TemplateID, err)
		}
		rel, err := s.writeFile(path.Join("opts", fileName(newID)+".opt"), b)
		if err != nil {
			return nil, nil, nil, err
		}
		nt := templateRecord{
			TemplateID: newID, File: filepath.Join(s.dir, rel), Root: t.Root,
			Signature: signatureOf(b), Kept: true, RenamedFrom: t.TemplateID,
		}
		renamed = append(renamed, nt)
		out[i] = nt
	}
	recs = slices.DeleteFunc(recs, func(r uploadRecord) bool { return stale[r.TemplateID] })
	again, err := uploadAll(ctx, clients, renamed, workers)
	if err != nil {
		return nil, nil, nil, err
	}
	recs = append(recs, again...)
	slices.SortFunc(recs, func(a, b uploadRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.TemplateID, b.TemplateID))
	})
	s.logf("upload: renamed %d templates a CDR holds other content under", len(renamed))
	return recs, out, renamed, nil
}

func uploadAll(ctx context.Context, clients []*cdrClient, tpls []templateRecord, workers int) ([]uploadRecord, error) {
	recs := make([]uploadRecord, len(clients)*len(tpls))
	err := forEach(ctx, workers, len(recs), func(ctx context.Context, i int) {
		recs[i] = uploadOne(ctx, clients[i/len(tpls)], tpls[i%len(tpls)])
	})
	return recs, err
}

// uploadOne uploads one OPT. When the CDR refuses it as already present,
// the copy it holds is fetched and compared by signature.
func uploadOne(ctx context.Context, c *cdrClient, t templateRecord) uploadRecord {
	rec := uploadRecord{CDR: c.name, TemplateID: t.TemplateID, File: t.File, RenamedFrom: t.RenamedFrom}
	raw, err := os.ReadFile(t.File)
	if err != nil {
		rec.Error = "read OPT: " + err.Error()
		return rec
	}
	o, storedID := c.upload(ctx, raw)
	rec.outcome = o
	if storedID != "" && storedID != t.TemplateID {
		rec.StoredID = storedID
	}
	if o.OK {
		rec.Present = true
		return rec
	}
	if !refusedAsPresent(o) {
		return rec
	}
	stored, so := c.storedTemplate(ctx, t.TemplateID)
	rec.StoredStatus = so.Status
	if !so.OK {
		// The copy could not be read: a 409 still says the CDR holds one.
		rec.Present = o.Status == http.StatusConflict
		return rec
	}
	same := signatureOf(stored) == t.Signature
	rec.StoredSame, rec.Present = &same, same
	return rec
}

// generateStage generates every template's documents and posts each
// COMPOSITION to every CDR that holds its template, each to a fresh EHR.
func generateStage(ctx context.Context, s *sink, cc *compiler, clients []*cdrClient, present map[string]map[string]bool, tpls []templateRecord, workers int) ([]genRecord, []postRecord, error) {
	type result struct {
		gens  []genRecord
		posts []postRecord
	}
	results := make([]result, len(tpls))
	err := forEach(ctx, workers, len(tpls), func(ctx context.Context, i int) {
		t := tpls[i]
		for _, d := range generateAll(ctx, s, cc, t) {
			results[i].gens = append(results[i].gens, d.rec)
			if d.comp == nil || d.rec.File == "" {
				continue
			}
			for _, c := range clients {
				if !present[c.name][t.TemplateID] {
					continue
				}
				id, o, created := c.postComposition(ctx, d.comp)
				p := postRecord{CDR: c.name, TemplateID: t.TemplateID, Label: d.rec.label(), File: d.rec.File, EHRID: string(id), outcome: o}
				if !created {
					p.Stage = "ehr"
				}
				results[i].posts = append(results[i].posts, p)
			}
		}
	})
	var gens []genRecord
	var posts []postRecord
	for _, r := range results {
		gens = append(gens, r.gens...)
		posts = append(posts, r.posts...)
	}
	slices.SortStableFunc(posts, func(a, b postRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.TemplateID, b.TemplateID), strings.Compare(a.Label, b.Label))
	})
	return gens, posts, err
}

// exampleStage fetches each CDR's own example of each template it holds,
// at each level it builds.
func exampleStage(ctx context.Context, s *sink, clients []*cdrClient, present map[string]map[string]bool, tpls []templateRecord, workers int) ([]exampleRecord, error) {
	type job struct {
		c     *cdrClient
		tid   string
		level definition.ExampleDetailLevel
	}
	var jobs []job
	for _, c := range clients {
		for _, t := range tpls {
			if !present[c.name][t.TemplateID] {
				continue
			}
			for _, l := range c.levels {
				jobs = append(jobs, job{c: c, tid: t.TemplateID, level: l})
			}
		}
	}
	recs := make([]exampleRecord, len(jobs))
	err := forEach(ctx, workers, len(jobs), func(ctx context.Context, i int) {
		j := jobs[i]
		level := cmp.Or(string(j.level), "default")
		rec := exampleRecord{CDR: j.c.name, TemplateID: j.tid, Level: level}
		body, o := j.c.example(ctx, j.tid, j.level)
		rec.outcome = o
		if o.OK {
			var probe struct {
				Type string `json:"_type"`
			}
			if err := json.Unmarshal(body, &probe); err != nil {
				rec.OK, rec.Error = false, "the example is not a JSON object: "+err.Error()
				rec.Body = clip(string(body))
			} else {
				rec.RootType = probe.Type
				rel, err := s.writeFile(path.Join("examples", j.c.name, fileName(j.tid), level+".json"), body)
				if err != nil {
					rec.Error = err.Error()
				}
				rec.File = rel
			}
		}
		recs[i] = rec
	})
	for _, c := range clients {
		n, of := 0, 0
		for _, r := range recs {
			if r.CDR == c.name {
				of++
				if r.File != "" {
					n++
				}
			}
		}
		s.logf("examples %s: %d of %d fetched", c.name, n, of)
	}
	return recs, err
}

// validateStage validates every fetched CDR example with the SDK.
func validateStage(ctx context.Context, s *sink, cc *compiler, files map[string]string, examples []exampleRecord, workers int) ([]validateRecord, error) {
	var fetched []exampleRecord
	for _, e := range examples {
		if e.File != "" {
			fetched = append(fetched, e)
		}
	}
	recs := make([]validateRecord, len(fetched))
	err := forEach(ctx, workers, len(fetched), func(ctx context.Context, i int) {
		e := fetched[i]
		doc, err := os.ReadFile(filepath.Join(s.dir, e.File))
		if err != nil {
			recs[i] = validateRecord{CDR: e.CDR, TemplateID: e.TemplateID, Level: e.Level, File: e.File, Stage: "read", Error: err.Error()}
			return
		}
		recs[i] = validateExample(cc, files[e.TemplateID], e, doc)
	})
	return recs, err
}

// crossStage posts every CDR example that is a COMPOSITION, its top-level
// uid removed, to every CDR that holds its template, each to a fresh EHR.
func crossStage(ctx context.Context, s *sink, clients []*cdrClient, present map[string]map[string]bool, examples []exampleRecord, workers int) ([]postRecord, error) {
	var docs []exampleRecord
	for _, e := range examples {
		if e.File != "" && e.RootType == "COMPOSITION" {
			docs = append(docs, e)
		}
	}
	results := make([][]postRecord, len(docs))
	err := forEach(ctx, workers, len(docs), func(ctx context.Context, i int) {
		e := docs[i]
		label := e.CDR + ":" + e.Level
		raw, err := os.ReadFile(filepath.Join(s.dir, e.File))
		var doc []byte
		if err == nil {
			doc, err = withoutUID(raw)
		}
		for _, c := range clients {
			if !present[c.name][e.TemplateID] {
				continue
			}
			p := postRecord{CDR: c.name, TemplateID: e.TemplateID, Label: label, Source: e.CDR, File: e.File}
			if err != nil {
				p.Stage, p.Error = "read", err.Error()
				results[i] = append(results[i], p)
				continue
			}
			id, o, created := c.postRaw(ctx, doc)
			p.EHRID, p.outcome = string(id), o
			if !created {
				p.Stage = "ehr"
			}
			results[i] = append(results[i], p)
		}
	})
	var out []postRecord
	for _, r := range results {
		out = append(out, r...)
	}
	slices.SortStableFunc(out, func(a, b postRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.TemplateID, b.TemplateID), strings.Compare(a.Label, b.Label))
	})
	return out, err
}

// writeAll writes each stage's JSON Lines, the summary and the run's
// provenance.
func writeAll(s *sink, info runInfo, d runData, sum summary) error {
	errs := []error{
		writeJSONL(s, "templates.jsonl", d.templates),
		writeJSONL(s, "gen.jsonl", d.gens),
	}
	if len(d.cdrs) > 0 {
		errs = append(errs,
			writeJSONL(s, "upload.jsonl", d.uploads),
			writeJSONL(s, "post.jsonl", d.posts),
			writeJSONL(s, "examples.jsonl", d.examples),
			writeJSONL(s, "validate.jsonl", d.validates),
			writeJSONL(s, "cross.jsonl", d.cross),
		)
	}
	errs = append(errs, s.writeJSON("run.json", info), s.writeJSON("summary.json", sum))
	_, err := s.writeFile("summary.md", []byte(markdown(info, sum)))
	return errors.Join(append(errs, err)...)
}

// logSummary prints the headline numbers.
func logSummary(s *sink, sum summary) {
	g := sum.Generation
	s.logf("generation: %d runs, %d errors, %d not self-valid, %d not RM-valid", g.Runs, g.Errors, g.NotSelfValid, g.NotRMValid)
	for _, c := range sum.PerCDR {
		s.logf("%s: templates %s, SDK documents accepted %s, examples fetched %s, SDK-valid examples %s, own examples accepted %s, other CDRs' examples accepted %s",
			c.Name, c.TemplatesPresent, c.DocumentsAccepted, c.ExamplesFetched, c.ExamplesSDKValid, c.OwnExamplesAccepted, c.OtherExamplesAccepted)
	}
	if len(sum.PerCDR) > 0 {
		var alone []string
		for _, c := range sum.PerCDR {
			alone = append(alone, fmt.Sprintf("%d by %s alone", sum.SDKValidRefused.Alone[c.Name], c.Name))
		}
		s.logf("SDK-valid documents refused: %d by every CDR, %s", sum.SDKValidRefused.ByAll, strings.Join(alone, ", "))
	}
	s.logf("summary: %s", filepath.Join(s.dir, "summary.md"))
}
