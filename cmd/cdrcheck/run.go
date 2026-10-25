package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
)

// renameSuffix is appended to a template id whose CDR copy holds other
// content. The OPT is uploaded again under the new id, to every CDR, and
// generated from under it, so both CDRs and the SDK read the same content.
const renameSuffix = ".xc1"

// runFile is the run's provenance, and the file whose creation claims the
// output directory for the run (prepareOut).
const runFile = "run.json"

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

// runner is one run: its output, the CDRs it talks to, the EHR ids it hands
// out, and a stream per stage file.
type runner struct {
	s       *sink
	clients []*cdrClient
	ids     *runIDs
	workers int

	// templates is every OPT file read, a renamed template included. It is
	// written to templates.jsonl once the renames are decided, or when the
	// run stops before that.
	templates        []templateRecord
	templatesWritten bool

	uploads   *stream[uploadRecord]
	gens      *stream[genRecord]
	ehrs      *stream[ehrRecord]
	posts     *stream[postRecord]
	examples  *stream[exampleRecord]
	validates *stream[validateRecord]
	cross     *stream[postRecord]
}

// execute runs the cross-check cfg describes and writes its output. Each
// record is written as it completes. However the run ends, finished,
// interrupted or on an error, it then writes the summary of the records it
// made, marked partial when it stopped early, and returns why it stopped.
func execute(ctx context.Context, cfg config, stdout io.Writer) error {
	dir, err := prepareOut(cfg)
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	s := &sink{dir: dir, red: newRedactor(cfg.cdrs), log: stdout}
	r := &runner{s: s, ids: newRunIDs(), workers: cfg.workers}
	info := runInfo{
		Started:   time.Now().UTC().Format(time.RFC3339),
		RunID:     r.ids.id(),
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
	s.logf("cdrcheck: output in %s, run %s", dir, info.RunID)

	for _, t := range cfg.cdrs {
		c, err := newCDRClient(t, cfg.workers, s.red)
		if err != nil {
			return err
		}
		r.clients = append(r.clients, c)
		info.CDRs = append(info.CDRs, cdrRef{Name: t.name, Base: t.base})
	}
	// The run identifier is on disk before the first request, so every EHR
	// the run creates is attributable even if nothing else gets written.
	if err := s.writeJSON(runFile, info); err != nil {
		return err
	}
	runErr := r.openStreams(stop)
	if runErr == nil {
		runErr = r.run(ctx, cfg)
	}
	return errors.Join(runErr, r.finish(info, runErr))
}

// openStreams creates the stage files: gen.jsonl always, the others when
// the run talks to a CDR.
func (r *runner) openStreams(stop context.CancelCauseFunc) error {
	var err error
	if r.gens, err = openStream[genRecord](r.s, "gen.jsonl", stop, nil); err != nil {
		return err
	}
	if len(r.clients) == 0 {
		return nil
	}
	if r.uploads, err = openStream(r.s, "upload.jsonl", stop, uploadRecord.written); err != nil {
		return err
	}
	if r.ehrs, err = openStream(r.s, "ehrs.jsonl", stop, ehrRecord.written); err != nil {
		return err
	}
	if r.posts, err = openStream(r.s, "post.jsonl", stop, postRecord.written); err != nil {
		return err
	}
	if r.examples, err = openStream(r.s, "examples.jsonl", stop, exampleRecord.written); err != nil {
		return err
	}
	if r.validates, err = openStream[validateRecord](r.s, "validate.jsonl", stop, nil); err != nil {
		return err
	}
	r.cross, err = openStream(r.s, "cross.jsonl", stop, postRecord.written)
	return err
}

// run runs the stages in order and returns the first error, or why the run
// was stopped.
func (r *runner) run(ctx context.Context, cfg config) error {
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
	r.templates = all
	r.s.logf("collect: %d OPT files, %d templates checked", len(all), len(tpls))

	present := map[string]map[string]bool{}
	if len(r.clients) > 0 {
		var renamed []templateRecord
		tpls, renamed, err = r.uploadStage(ctx, tpls)
		r.applyRenames(renamed)
		if err != nil {
			return err
		}
		present = presence(r.uploads.records())
		for _, c := range r.clients {
			n := 0
			for _, ok := range present[c.name] {
				if ok {
					n++
				}
			}
			r.s.logf("upload %s: %d of %d templates present", c.name, n, len(tpls))
		}
	}
	if err := r.writeTemplates(); err != nil {
		return err
	}

	cc := &compiler{}
	if err := r.generateStage(ctx, cc, present, tpls); err != nil {
		return err
	}
	r.s.logf("generate: %d runs", len(r.gens.records()))
	if len(r.clients) == 0 {
		return nil
	}
	files := map[string]string{}
	for _, t := range tpls {
		files[t.TemplateID] = t.File
	}
	if err := r.exampleStage(ctx, present, tpls); err != nil {
		return err
	}
	if err := r.validateStage(ctx, cc, files); err != nil {
		return err
	}
	return r.crossStage(ctx, present)
}

// finish writes what a run writes last, however it ended: templates.jsonl if
// the run stopped before it, the summary of the records made, and run.json
// with the end time and, for a run that stopped early, why.
func (r *runner) finish(info runInfo, runErr error) error {
	errs := []error{r.writeTemplates()}
	for _, c := range []interface{ close() error }{r.uploads, r.gens, r.ehrs, r.posts, r.examples, r.validates, r.cross} {
		errs = append(errs, c.close())
	}
	sum := summarize(r.data())
	sum.Only = info.Only
	info.Finished = time.Now().UTC().Format(time.RFC3339)
	if runErr != nil {
		info.Stopped = runErr.Error()
		sum.Partial = runErr.Error()
	}
	errs = append(errs, r.s.writeJSON(runFile, info), r.s.writeJSON("summary.json", sum))
	_, err := r.s.writeFile("summary.md", []byte(markdown(info, sum)))
	errs = append(errs, err)
	if runErr != nil {
		r.s.logf("cdrcheck: stopped before it finished (%s); the summary counts the records made until then", runErr)
	}
	logSummary(r.s, sum)
	return errors.Join(errs...)
}

// writeTemplates writes templates.jsonl, once.
func (r *runner) writeTemplates() error {
	if r.templatesWritten {
		return nil
	}
	r.templatesWritten = true
	return writeJSONL(r.s, "templates.jsonl", r.templates)
}

// data is what the run recorded, each stage's records in a fixed order: the
// streams hold them in the order they completed.
func (r *runner) data() runData {
	d := runData{templates: r.templates}
	for _, c := range r.clients {
		d.cdrs = append(d.cdrs, c.name)
	}
	byDoc := func(a, b postRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.TemplateID, b.TemplateID), strings.Compare(a.Label, b.Label))
	}
	d.uploads = sorted(r.uploads.records(), func(a, b uploadRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.TemplateID, b.TemplateID))
	})
	d.gens = sorted(r.gens.records(), func(a, b genRecord) int {
		return cmp.Or(strings.Compare(a.TemplateID, b.TemplateID), strings.Compare(a.Policy, b.Policy), strings.Compare(a.Fill, b.Fill))
	})
	d.ehrs = sorted(r.ehrs.records(), func(a, b ehrRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.EHRID, b.EHRID))
	})
	d.posts = sorted(r.posts.records(), byDoc)
	d.examples = sorted(r.examples.records(), func(a, b exampleRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.TemplateID, b.TemplateID), strings.Compare(a.Level, b.Level))
	})
	d.validates = sorted(r.validates.records(), func(a, b validateRecord) int {
		return cmp.Or(strings.Compare(a.CDR, b.CDR), strings.Compare(a.TemplateID, b.TemplateID), strings.Compare(a.Level, b.Level))
	})
	d.cross = sorted(r.cross.records(), byDoc)
	return d
}

func sorted[T any](s []T, by func(a, b T) int) []T {
	slices.SortStableFunc(s, by)
	return s
}

// prepareOut claims the run's output directory and returns its path. A
// directory named by -out or OPENEHR_XC_OUT is created with os.Mkdir, or
// taken as it is when it exists already and is empty; with none named, a
// new one is made under cfg.outRoot with os.MkdirTemp, a name no other run
// gets. Either way the run then claims the directory by creating run.json
// with O_EXCL, so that of two runs given one directory only one gets it, and
// it refuses a directory that holds anything else.
func prepareOut(cfg config) (string, error) {
	dir := cfg.out
	if dir == "" {
		if err := os.MkdirAll(cfg.outRoot, 0o755); err != nil {
			return "", fmt.Errorf("output directory: %w", err)
		}
		d, err := os.MkdirTemp(cfg.outRoot, time.Now().UTC().Format("20060102T150405Z")+"-")
		if err != nil {
			return "", fmt.Errorf("output directory: %w", err)
		}
		dir = d
	} else {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", fmt.Errorf("output directory: %w", err)
		}
		if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("output directory: %w", err)
		}
	}
	claim := filepath.Join(dir, runFile)
	f, err := os.OpenFile(claim, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("output directory %s is in use by another run; name a new one with -out or %s", dir, envOut)
	}
	if err != nil {
		return "", fmt.Errorf("output directory: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("output directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", errors.Join(fmt.Errorf("output directory: %w", err), os.Remove(claim))
	}
	if len(entries) > 1 {
		return "", errors.Join(fmt.Errorf("output directory %s is not empty; name a new one with -out or %s", dir, envOut), os.Remove(claim))
	}
	return dir, nil
}

// presence maps each CDR to the templates it holds after the upload stage.
func presence(ups []uploadRecord) map[string]map[string]bool {
	present := map[string]map[string]bool{}
	for _, u := range ups {
		if present[u.CDR] == nil {
			present[u.CDR] = map[string]bool{}
		}
		present[u.CDR][u.TemplateID] = u.Present
	}
	return present
}

// uploadStage uploads every template to every CDR, a template at a time. out
// is tpls with each renamed template (uploadTemplate) in place of its
// original; renamed lists the new records. Both cover the templates the
// stage finished, when it stops early.
func (r *runner) uploadStage(ctx context.Context, tpls []templateRecord) (out, renamed []templateRecord, err error) {
	news := make([]*templateRecord, len(tpls))
	errs := make([]error, len(tpls))
	ferr := forEach(ctx, r.workers, len(tpls), func(ctx context.Context, i int) {
		news[i], errs[i] = r.uploadTemplate(ctx, tpls[i])
	})
	out = slices.Clone(tpls)
	for i, nt := range news {
		if nt != nil {
			out[i] = *nt
			renamed = append(renamed, *nt)
		}
	}
	if len(renamed) > 0 {
		r.s.logf("upload: renamed %d templates a CDR holds other content under", len(renamed))
	}
	return out, renamed, errors.Join(append(errs, ferr)...)
}

// uploadTemplate uploads t to every CDR and records each answer. Where some
// CDR holds other content under t's id, t is renamed (renameSuffix) and
// uploaded again, to every CDR; the records of t are kept, marked as
// superseded by the new id, and the new record is returned.
func (r *runner) uploadTemplate(ctx context.Context, t templateRecord) (*templateRecord, error) {
	recs := make([]uploadRecord, 0, len(r.clients))
	stale := false
	for _, c := range r.clients {
		rec := uploadOne(ctx, c, t)
		stale = stale || (rec.StoredSame != nil && !*rec.StoredSame)
		recs = append(recs, rec)
	}
	if !stale {
		r.uploads.add(recs...)
		return nil, nil
	}
	nt, err := r.renamedTemplate(t)
	if err != nil {
		r.uploads.add(recs...)
		return nil, err
	}
	for i := range recs {
		recs[i].SupersededBy = nt.TemplateID
	}
	r.uploads.add(recs...)
	for _, c := range r.clients {
		r.uploads.add(uploadOne(ctx, c, nt))
	}
	return &nt, nil
}

// renamedTemplate writes t's OPT under its id plus renameSuffix to the
// output directory and returns its record.
func (r *runner) renamedTemplate(t templateRecord) (templateRecord, error) {
	raw, err := os.ReadFile(t.File)
	if err != nil {
		return templateRecord{}, fmt.Errorf("read OPT: %w", err)
	}
	newID := t.TemplateID + renameSuffix
	b, err := renamedOPT(raw, newID)
	if err != nil {
		return templateRecord{}, fmt.Errorf("rename %s: %w", t.TemplateID, err)
	}
	rel, err := r.s.writeFile(path.Join("opts", fileName(newID)+".opt"), b)
	if err != nil {
		return templateRecord{}, err
	}
	return templateRecord{
		TemplateID: newID, File: filepath.Join(r.s.dir, rel), Root: t.Root,
		Signature: signatureOf(b), Kept: true, RenamedFrom: t.TemplateID,
	}, nil
}

// applyRenames marks each renamed template's original as replaced and adds
// the renamed record.
func (r *runner) applyRenames(renamed []templateRecord) {
	for _, nt := range renamed {
		for i := range r.templates {
			if r.templates[i].Kept && r.templates[i].TemplateID == nt.RenamedFrom {
				r.templates[i].Kept, r.templates[i].RenamedTo = false, nt.TemplateID
			}
		}
	}
	r.templates = append(r.templates, renamed...)
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
func (r *runner) generateStage(ctx context.Context, cc *compiler, present map[string]map[string]bool, tpls []templateRecord) error {
	return forEach(ctx, r.workers, len(tpls), func(ctx context.Context, i int) {
		t := tpls[i]
		generateAll(ctx, r.s, cc, t, func(d generated) {
			r.gens.add(d.rec)
			if d.comp == nil || d.rec.File == "" {
				return
			}
			for _, c := range r.clients {
				if !present[c.name][t.TemplateID] {
					continue
				}
				p := postRecord{CDR: c.name, TemplateID: t.TemplateID, Label: d.rec.label(), File: d.rec.File}
				r.post(ctx, c, p, r.posts, func(ctx context.Context, id openehrclient.EHRID) outcome {
					return c.saveComposition(ctx, id, d.comp)
				})
			}
		})
	})
}

// post creates the next EHR of the run on c and, once it exists, sends the
// document there with send. The EHR create is recorded in ehrs.jsonl as it
// returns, then the post in out. A create the CDR refuses is the document's
// outcome, with stage "ehr".
func (r *runner) post(ctx context.Context, c *cdrClient, p postRecord, out *stream[postRecord], send func(context.Context, openehrclient.EHRID) outcome) {
	id := r.ids.next()
	eo := c.newEHR(ctx, id)
	r.ehrs.add(ehrRecord{CDR: c.name, EHRID: string(id), TemplateID: p.TemplateID, Label: p.Label, Source: p.Source, outcome: eo})
	p.EHRID = string(id)
	if eo.OK {
		p.outcome = send(ctx, id)
	} else {
		p.Stage, p.outcome = "ehr", eo
	}
	out.add(p)
}

// exampleStage fetches each CDR's own example of each template it holds,
// at each level it builds.
func (r *runner) exampleStage(ctx context.Context, present map[string]map[string]bool, tpls []templateRecord) error {
	type job struct {
		c     *cdrClient
		tid   string
		level definition.ExampleDetailLevel
	}
	var jobs []job
	for _, c := range r.clients {
		for _, t := range tpls {
			if !present[c.name][t.TemplateID] {
				continue
			}
			for _, l := range c.levels {
				jobs = append(jobs, job{c: c, tid: t.TemplateID, level: l})
			}
		}
	}
	err := forEach(ctx, r.workers, len(jobs), func(ctx context.Context, i int) {
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
				rec.Body = r.s.red.string(string(body))
			} else {
				rec.RootType = probe.Type
				rel, err := r.s.writeFile(path.Join("examples", j.c.name, fileName(j.tid), level+".json"), body)
				if err != nil {
					rec.Error = err.Error()
				}
				rec.File = rel
			}
		}
		r.examples.add(rec)
	})
	recs := r.examples.records()
	for _, c := range r.clients {
		n, of := 0, 0
		for _, e := range recs {
			if e.CDR == c.name {
				of++
				if e.File != "" {
					n++
				}
			}
		}
		r.s.logf("examples %s: %d of %d fetched", c.name, n, of)
	}
	return err
}

// validateStage validates every fetched CDR example with the SDK.
func (r *runner) validateStage(ctx context.Context, cc *compiler, files map[string]string) error {
	var fetched []exampleRecord
	for _, e := range r.examples.records() {
		if e.File != "" {
			fetched = append(fetched, e)
		}
	}
	return forEach(ctx, r.workers, len(fetched), func(ctx context.Context, i int) {
		e := fetched[i]
		doc, err := os.ReadFile(filepath.Join(r.s.dir, e.File))
		if err != nil {
			r.validates.add(validateRecord{CDR: e.CDR, TemplateID: e.TemplateID, Level: e.Level, File: e.File, Stage: "read", Error: err.Error()})
			return
		}
		r.validates.add(validateExample(cc, files[e.TemplateID], e, doc))
	})
}

// crossStage posts every CDR example that is a COMPOSITION, its top-level
// uid removed, to every CDR that holds its template, each to a fresh EHR.
func (r *runner) crossStage(ctx context.Context, present map[string]map[string]bool) error {
	var docs []exampleRecord
	for _, e := range r.examples.records() {
		if e.File != "" && e.RootType == "COMPOSITION" {
			docs = append(docs, e)
		}
	}
	return forEach(ctx, r.workers, len(docs), func(ctx context.Context, i int) {
		e := docs[i]
		raw, err := os.ReadFile(filepath.Join(r.s.dir, e.File))
		var doc []byte
		if err == nil {
			doc, err = withoutUID(raw)
		}
		for _, c := range r.clients {
			if !present[c.name][e.TemplateID] {
				continue
			}
			p := postRecord{CDR: c.name, TemplateID: e.TemplateID, Label: e.CDR + ":" + e.Level, Source: e.CDR, File: e.File}
			if err != nil {
				p.Stage, p.Error = "read", err.Error()
				r.cross.add(p)
				continue
			}
			r.post(ctx, c, p, r.cross, func(ctx context.Context, id openehrclient.EHRID) outcome {
				return c.postRaw(ctx, id, doc)
			})
		}
	})
}

// logSummary prints the headline numbers.
func logSummary(s *sink, sum summary) {
	g := sum.Generation
	s.logf("generation: %d runs, %d errors, %d not self-valid, %d not RM-valid", g.Runs, g.Errors, g.NotSelfValid, g.NotRMValid)
	for _, c := range sum.PerCDR {
		s.logf("%s: templates %s, EHRs created %s, SDK documents accepted %s, examples fetched %s, SDK-valid examples %s, own examples accepted %s, other CDRs' examples accepted %s",
			c.Name, c.TemplatesPresent, c.EHRsCreated, c.DocumentsAccepted, c.ExamplesFetched, c.ExamplesSDKValid, c.OwnExamplesAccepted, c.OtherExamplesAccepted)
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
