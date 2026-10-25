// Command cdrcheck cross-checks the SDK against live openEHR CDRs: it
// generates documents from a directory of OPTs with the SDK's example
// generator, posts them to each CDR, and validates each CDR's own template
// example with the SDK's validator.
//
// For each CDR it uploads every OPT, then, for each template, policy
// (Required, Medium, Large) and value fill (the example fill and the random
// fill on two seeds), it generates a document, validates it with the
// template validator and the RM floor, and posts each COMPOSITION to every
// CDR that holds its template. It fetches each CDR's own example of each
// template (GET /definition/template/adl1.4/{id}/example, at each level the
// CDR builds), validates it with the SDK, and posts it, its uid removed, to
// every CDR. Every document goes to an EHR of its own.
//
// It is opt-in and not part of make ci. It reads:
//
//	OPENEHR_XC_TEMPLATES         a directory of OPTs (*.opt, searched recursively)
//	OPENEHR_LIVE_EHRBASE         an EHRbase REST base, ending in /openehr/v1
//	OPENEHR_LIVE_EHRBASE_BASIC   its Basic credential as user:pass (optional)
//	OPENEHR_LIVE_FERROEHR        a FerroEHR REST base, ending in /openehr/v1
//	OPENEHR_LIVE_FERROEHR_BASIC  its Basic credential as user:pass (optional)
//	OPENEHR_LIVE_ALLOW_MUTATING  any value: lets the run write to the CDRs
//	OPENEHR_XC_OUT               the output directory (or -out)
//
// With no template directory it prints that it skipped and exits 0. With
// templates but no CDR, or without the write opt-in, it generates and
// validates offline and skips the CDR stages. Either CDR may be left out.
//
// The run writes to the CDRs: templates under their own ids, EHRs and
// compositions. It never deletes and never replaces a template. A template
// a CDR already holds is compared with the OPT by a signature of its parsed
// XML; where the content differs, the OPT is uploaded again under its id
// plus ".xc1", to every CDR, and generated from under that id, so that the
// SDK and every CDR read the same content. The run names every EHR it
// creates (PUT /ehr/{ehr_id}) with an id that begins with its per-run
// identifier, which run.json records before the first request, and records
// each create as it returns.
//
// The output is a directory outside the repository that no other run has
// claimed, by default a new one under the system temp directory: one JSON
// Lines file per stage (templates, upload, gen, ehrs, post, examples,
// validate, cross), each line written as its record completes, the
// documents and the CDRs' examples, run.json, summary.json and summary.md.
// A run that stops early, on Ctrl-C or an error, keeps every record made
// until then and writes a summary marked partial. A second Ctrl-C while it
// winds down ends it at once, before the summary. A credential never reaches
// the output, a progress line or an error.
//
// -only REGEXP checks only the templates whose id matches; the summary of
// such a run says so.
//
// Usage:
//
//	make cdr-crosscheck
//	go run ./cmd/cdrcheck [-out DIR] [-workers N] [-only REGEXP]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := interruptContext(context.Background(), signal.NotifyContext)
	err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cdrcheck:", err)
		os.Exit(1)
	}
}

// interruptContext returns a context that ends on the first Ctrl-C
// (os.Interrupt), as notify, signal.NotifyContext, gives it. Once that
// context has ended, the stop notify returned runs at once and restores the
// default handling of os.Interrupt, so a second Ctrl-C while the run winds
// down ends the process instead of being swallowed.
func interruptContext(parent context.Context, notify func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)) (context.Context, context.CancelFunc) {
	ctx, stop := notify(parent, os.Interrupt)
	context.AfterFunc(ctx, stop)
	return ctx, stop
}

// run reads the configuration and runs the cross-check. getenv, stdout and
// stderr are parameters so tests drive it without the process environment.
// The error it returns carries no credential.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	cfg, skip, err := loadConfig(args, getenv, stderr)
	if err != nil {
		return err
	}
	if skip {
		_, err := fmt.Fprintln(stdout, skipNotice)
		return err
	}
	if err := execute(ctx, cfg, stdout); err != nil {
		return errors.New(newRedactor(cfg.cdrs).string(err.Error()))
	}
	return nil
}
