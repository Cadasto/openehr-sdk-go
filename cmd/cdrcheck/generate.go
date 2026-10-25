package main

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"path"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// policies are the generator's three policies, smallest output first.
var policies = []instance.Policy{instance.Required, instance.Medium, instance.Large}

// fill is one value fill: the example fill, or the random fill on a seed.
type fill struct {
	name string
	seed uint64 // 0: the example fill
}

var fills = []fill{{name: "example"}, {name: "random1", seed: 1}, {name: "random2", seed: 2}}

// generationNow is the generator's clock, fixed so that two runs write the
// same date-times and their documents compare.
var generationNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// issue is one validator finding.
type issue struct {
	Path   string `json:"path"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func issuesOf(r validation.Result) []issue {
	out := make([]issue, 0, len(r.Issues))
	for _, i := range r.Issues {
		out = append(out, issue{Path: i.Path, Code: i.Code, Detail: i.Detail})
	}
	return out
}

// genRecord is one generation, one line of gen.jsonl.
type genRecord struct {
	TemplateID string `json:"template_id"`
	Policy     string `json:"policy,omitempty"`
	Fill       string `json:"fill,omitempty"`
	// File is the generated document, relative to the output directory.
	File     string `json:"file,omitempty"`
	RootType string `json:"root_type,omitempty"`
	// Stage names where the run stopped: parse, compile, generate, marshal
	// or write. Error is why.
	Stage string `json:"stage,omitempty"`
	Error string `json:"error,omitempty"`
	// SelfOK is the SDK template validator's verdict (validation.Validate),
	// RMOK the Reference Model floor's (validation.ValidateRM).
	SelfOK     *bool   `json:"self_ok,omitempty"`
	SelfIssues []issue `json:"self_issues,omitempty"`
	RMOK       *bool   `json:"rm_ok,omitempty"`
	RMIssues   []issue `json:"rm_issues,omitempty"`
	Millis     int64   `json:"ms"`
}

// label names a generated document: <policy>-<fill>.
func (g genRecord) label() string { return g.Policy + "-" + g.Fill }

// compiledTemplate is an OPT parsed in strict mode and compiled, once.
type compiledTemplate struct {
	once     sync.Once
	compiled *templatecompile.Compiled
	stage    string
	err      error
}

// compiler caches compiled templates by file.
type compiler struct {
	mu sync.Mutex
	m  map[string]*compiledTemplate
}

func (c *compiler) get(file string) (*templatecompile.Compiled, string, error) {
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]*compiledTemplate{}
	}
	e, ok := c.m[file]
	if !ok {
		e = &compiledTemplate{}
		c.m[file] = e
	}
	c.mu.Unlock()
	e.once.Do(func() {
		opt, err := template.ParseFileStrict(file)
		if err != nil {
			e.stage, e.err = "parse", err
			return
		}
		e.compiled, err = templatecompile.Compile(opt)
		if err != nil {
			e.stage, e.err = "compile", err
		}
	})
	return e.compiled, e.stage, e.err
}

// generated is one document the generator wrote, with the composition to
// post when its root is a COMPOSITION.
type generated struct {
	rec  genRecord
	comp *rm.Composition
}

// generateAll runs every policy and fill on one template and hands each
// generation to emit as it completes. A template that does not parse or
// compile yields a record per policy and fill that says so, so that a run
// counts templates × policies × fills either way.
//
// It stops once ctx ends. A generation the end of the run cut short is not
// handed on: it measured nothing, and it created nothing on a CDR.
func generateAll(ctx context.Context, s *sink, cc *compiler, tpl templateRecord, emit func(generated)) {
	compiled, stage, err := cc.get(tpl.File)
	for _, p := range policies {
		for _, f := range fills {
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				emit(generated{rec: genRecord{TemplateID: tpl.TemplateID, Policy: p.String(), Fill: f.name, Stage: stage, Error: err.Error()}})
				continue
			}
			g := generateOne(ctx, s, compiled, tpl.TemplateID, p, f)
			if g.rec.Stage == "generate" && ctx.Err() != nil {
				return
			}
			emit(g)
		}
	}
}

func generateOne(ctx context.Context, s *sink, compiled *templatecompile.Compiled, tid string, p instance.Policy, f fill) generated {
	rec := genRecord{TemplateID: tid, Policy: p.String(), Fill: f.name}
	start := time.Now()
	opts := instance.Options{
		Policy:    p,
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: new("Example Composer")},
		Now:       generationNow,
	}
	if f.seed != 0 {
		opts.ValueFill = instance.RandomFill
		opts.ValueSource = mrand.NewPCG(f.seed, f.seed*7919)
		opts.UIDSource = instance.UIDsFrom(mrand.NewPCG(f.seed+100, f.seed*104729))
	}
	root, err := instance.Generate(ctx, compiled, opts)
	rec.Millis = time.Since(start).Milliseconds()
	if err != nil {
		rec.Stage, rec.Error = "generate", err.Error()
		return generated{rec: rec}
	}
	b, err := canjson.MarshalIndent(root, "", " ")
	if err != nil {
		rec.Stage, rec.Error = "marshal", err.Error()
		return generated{rec: rec}
	}
	var probe struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(b, &probe); err == nil {
		rec.RootType = probe.Type
	}
	file, err := s.writeFile(path.Join("gen", fileName(tid), rec.label()+".json"), b)
	if err != nil {
		rec.Stage, rec.Error = "write", err.Error()
	}
	rec.File = file
	sr := validation.Validate(root, compiled)
	rec.SelfOK, rec.SelfIssues = new(sr.OK), issuesOf(sr)
	rr := validation.ValidateRM(root)
	rec.RMOK, rec.RMIssues = new(rr.OK), issuesOf(rr)
	g := generated{rec: rec}
	if comp, err := instance.AsComposition(root); err == nil {
		g.comp = comp
	}
	return g
}

// validateRecord is the SDK validator's verdict on a CDR's example, one
// line of validate.jsonl.
type validateRecord struct {
	CDR        string `json:"cdr"`
	TemplateID string `json:"template_id"`
	Level      string `json:"level"`
	File       string `json:"file"`
	// Stage names where the run stopped: read, parse, compile or decode.
	Stage string `json:"stage,omitempty"`
	Error string `json:"error,omitempty"`
	// Unmatched are the JSON members the SDK's decoder did not read, as
	// JSON Pointers.
	Unmatched   []string `json:"unmatched,omitempty"`
	OK          *bool    `json:"ok,omitempty"`
	Issues      []issue  `json:"issues,omitempty"`
	Unevaluated []string `json:"unevaluated,omitempty"`
}

// validateExample validates one CDR example against its OPT with the SDK.
func validateExample(cc *compiler, tplFile string, ex exampleRecord, doc []byte) validateRecord {
	rec := validateRecord{CDR: ex.CDR, TemplateID: ex.TemplateID, Level: ex.Level, File: ex.File}
	compiled, stage, err := cc.get(tplFile)
	if err != nil {
		rec.Stage, rec.Error = stage, err.Error()
		return rec
	}
	var unmatched []string
	root, err := decodeRoot(doc, canjson.WithUnmatched(func(p string) { unmatched = append(unmatched, p) }))
	rec.Unmatched = unmatched
	if err != nil {
		rec.Stage, rec.Error = "decode", err.Error()
		return rec
	}
	r := validation.Validate(root, compiled)
	rec.OK, rec.Issues = new(r.OK), issuesOf(r)
	rec.Unevaluated = r.Coverage.Unevaluated
	return rec
}

// decodeRoot decodes a canonical-JSON root by its _type: a COMPOSITION (or
// a document that names no type), an EHR_STATUS or a FOLDER, the roots a
// CDR's template example can have.
func decodeRoot(b []byte, opt canjson.DecoderOption) (any, error) {
	var probe struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, err
	}
	switch probe.Type {
	case "COMPOSITION", "":
		var c rm.Composition
		if err := canjson.Unmarshal(b, &c, opt); err != nil {
			return nil, err
		}
		return &c, nil
	case "EHR_STATUS":
		var s rm.EHRStatus
		if err := canjson.Unmarshal(b, &s, opt); err != nil {
			return nil, err
		}
		return &s, nil
	case "FOLDER":
		var f rm.Folder
		if err := canjson.Unmarshal(b, &f, opt); err != nil {
			return nil, err
		}
		return &f, nil
	}
	return nil, fmt.Errorf("root type %s is not one the run validates", probe.Type)
}
